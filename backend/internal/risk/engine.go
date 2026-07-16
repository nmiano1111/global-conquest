// Package risk implements Global Conquest's core Risk game engine: the
// phase state machine, board and card rules, combat resolution, and turn
// sequencing that together are the sole authority for game legality. It
// owns the Game type, which is serialized as JSONB game state, and exposes
// read-only legal-action queries (see legal_actions.go) that bots and other
// callers use to enumerate legal moves without duplicating these rules.
// Nothing outside this package may mutate game state directly or
// re-implement its rules.
package risk

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sort"
)

// Phase identifies the current stage of a game's turn state machine (see
// the Phase* constants below).
type Phase string

const (
	// PhaseSetupClaim is the engine-only initial phase in which players
	// claim unowned territories one at a time. It is unused in practice:
	// NewClassicAutoStartGame and NewClassicRandomTerritoryGame both skip
	// past it directly to PhaseReinforce or PhaseSetupReinforce.
	PhaseSetupClaim Phase = "setup_claim"
	// PhaseSetupReinforce follows random territory distribution; players
	// place their remaining starting armies one at a time via
	// PlaceInitialArmy before the game begins.
	PhaseSetupReinforce Phase = "setup_reinforce"
	// PhaseReinforce is the start of a normal turn: the current player may
	// trade in card sets and must place all pending reinforcement armies
	// before attacking.
	PhaseReinforce Phase = "reinforce"
	// PhaseAttack is the phase in which the current player may launch any
	// number of attacks via Attack, or end the phase via EndAttackPhase.
	PhaseAttack Phase = "attack"
	// PhaseOccupy follows a successful conquest; the current player must
	// move armies into the captured territory via OccupyTerritory before
	// attacking again.
	PhaseOccupy Phase = "occupy"
	// PhaseFortify is the final phase of a turn, allowing at most one troop
	// movement between connected owned territories via Fortify before
	// EndTurn.
	PhaseFortify Phase = "fortify"
	// PhaseGameOver marks a finished game; no further actions are legal
	// once a single player controls every territory.
	PhaseGameOver Phase = "game_over"
)

var (
	// ErrInvalidPlayerCount is returned by NewClassicGame when the number
	// of players is outside the supported range of 3 to 6.
	ErrInvalidPlayerCount = errors.New("risk: player count must be between 3 and 6")
	// ErrOutOfTurn is returned when an action is submitted by a player
	// other than the game's CurrentPlayer.
	ErrOutOfTurn = errors.New("risk: out of turn")
	// ErrInvalidPhase is returned when an action is submitted during a
	// phase that does not permit it.
	ErrInvalidPhase = errors.New("risk: action not allowed in current phase")
	// ErrInvalidMove wraps a more specific error describing why an
	// otherwise phase-legal action violates game rules.
	ErrInvalidMove = errors.New("risk: invalid move")
)

// RNG is the source of randomness the engine uses for shuffling and dice
// rolls, allowing tests to inject deterministic sequences in place of
// stdRNG's crypto/rand-backed implementation.
type RNG interface {
	IntN(n int) int
}

type stdRNG struct{}

// IntN returns a cryptographically random integer in [0, n). crypto/rand.Int
// only fails if the OS entropy source is unavailable, which would already be
// catastrophic for the rest of the service (e.g. session token generation in
// auth/session_token.go), so we panic rather than silently degrading to a
// predictable fallback.
func (stdRNG) IntN(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(fmt.Sprintf("risk: crypto/rand unavailable: %v", err))
	}
	return int(v.Int64())
}

// PlayerState is one player's state within a Game: identity, hand, and
// elimination/controller status.
type PlayerState struct {
	// ID is the player's unique identifier, matching the ID passed to
	// NewClassicGame.
	ID string `json:"id"`
	// Cards holds the player's current hand of Risk cards, earned by
	// conquering at least one territory during a turn (see EndTurn) and
	// spent via TradeCards.
	Cards []Card `json:"cards"`
	// Eliminated reports whether the player has lost all territories and
	// is out of the game.
	Eliminated bool `json:"eliminated"`

	// Controller and Strategy are omitempty so existing serialized games
	// (all-human, no such fields) continue to decode as human players with
	// no strategy assigned. See ControllerType and PlayerState.IsBot.
	Controller ControllerType `json:"controller,omitempty"`
	// Strategy names which bot.Strategy implementation controls this
	// player when Controller is ControllerBot; empty and unused for human
	// players.
	Strategy string `json:"strategy,omitempty"`

	// Name is a bot's assigned display name. Humans have no row here — an
	// empty Name always means "look up the human's username instead."
	Name string `json:"name,omitempty"`
}

// TerritoryState is a single territory's ownership and army count.
type TerritoryState struct {
	// Owner is the index into Game.Players of the territory's controlling
	// player, or -1 if unclaimed.
	Owner int `json:"owner"`
	// Armies is the number of armies stationed on the territory.
	Armies int `json:"armies"`
}

// OccupyState is the pending post-conquest army move recorded on Game.Occupy
// while Phase is PhaseOccupy.
type OccupyState struct {
	// From is the attacking territory that must supply the occupying armies.
	From Territory `json:"from"`
	// To is the just-conquered territory being occupied.
	To Territory `json:"to"`
	// MinMove is the minimum number of armies that must move into To,
	// equal to the number of attacker dice rolled in the conquering attack.
	MinMove int `json:"min_move"`
	// MaxMove is the maximum number of armies that may move into To,
	// leaving at least one army behind in From.
	MaxMove int `json:"max_move"`
}

// AttackResult reports the outcome of a single call to Attack: dice rolled,
// losses on each side, and whether the target territory was conquered.
type AttackResult struct {
	// AttackerRolls holds the attacker's dice results, sorted highest to lowest.
	AttackerRolls []int `json:"attacker_rolls"`
	// DefenderRolls holds the defender's dice results, sorted highest to lowest.
	DefenderRolls []int `json:"defender_rolls"`
	// AttackerLoss is the number of armies the attacker lost in this attack.
	AttackerLoss int `json:"attacker_loss"`
	// DefenderLoss is the number of armies the defender lost in this attack.
	DefenderLoss int `json:"defender_loss"`
	// Conquered reports whether the target territory's armies were reduced
	// to zero, transferring ownership to the attacker.
	Conquered bool `json:"conquered"`
	// Eliminated holds the defeated player's ID if this attack eliminated
	// them by capturing their last territory, or empty otherwise.
	Eliminated string `json:"eliminated"`
}

// Game is the full state of a single match: board, players, territories,
// phase, and turn bookkeeping. It is serialized as JSONB in the
// games.state column and is the sole in-memory representation of game
// state mutated by every exported method on *Game.
type Game struct {
	// Board is the static map data (continents, adjacency, iteration
	// order) the game was created with.
	Board Board `json:"board"`
	// Players holds every player's state, indexed consistently with
	// CurrentPlayer, TerritoryState.Owner, and SetupReserves.
	Players []PlayerState `json:"players"`
	// Territories maps every territory on the board to its current owner
	// and army count.
	Territories map[Territory]TerritoryState `json:"territories"`

	// CurrentPlayer is the index into Players of the player whose turn it
	// currently is.
	CurrentPlayer int `json:"current_player"`
	// Phase is the current stage of the turn state machine, gating which
	// actions are legal.
	Phase Phase `json:"phase"`
	// Winner is the ID of the winning player, set once Phase reaches
	// PhaseGameOver.
	Winner string `json:"winner"`
	// TurnNumber counts completed turns, incremented at the start of each
	// player's turn by startTurn.
	TurnNumber int `json:"turn_number"`

	// SetupReserves maps player index to the number of starting armies
	// they have yet to place during setup; consumed by ClaimTerritory and
	// PlaceInitialArmy.
	SetupReserves map[int]int `json:"setup_reserves"`

	// PendingReinforcements is the number of reinforcement armies the
	// current player still has to place before PhaseReinforce can advance
	// to PhaseAttack.
	PendingReinforcements int `json:"pending_reinforcements"`
	// ConqueredThisTurn reports whether the current player has conquered
	// at least one territory this turn, entitling them to draw a card in
	// EndTurn.
	ConqueredThisTurn bool `json:"conquered_this_turn"`
	// TerritoryBonusUsed reports whether the current player has already
	// received the +2 territory bonus from trading in a card matching an
	// owned territory this turn.
	TerritoryBonusUsed bool `json:"territory_bonus_used"`
	// HasFortified reports whether the current player has already made
	// their one allowed Fortify move this turn.
	HasFortified bool `json:"has_fortified"`
	// ForcedCardTrade reports whether the current player was pushed back
	// into PhaseReinforce because they held five or more cards after
	// occupying a conquered territory, and must trade down before
	// continuing.
	ForcedCardTrade bool `json:"forced_card_trade"`
	// Occupy holds the pending post-conquest army move, non-nil only while
	// Phase is PhaseOccupy.
	Occupy *OccupyState `json:"occupy"`

	// Deck holds the face-down draw pile of cards not yet dealt to any player.
	Deck []Card `json:"deck"`
	// Discard holds cards that have been traded in and are eligible to be
	// reshuffled into Deck once it's empty.
	Discard []Card `json:"discard"`
	// SetsTraded counts how many card sets have been traded in across the
	// whole game, determining the next set's reinforcement value via
	// nextTradeValue.
	SetsTraded int `json:"sets_traded"`

	rng RNG `json:"-"`
}

// NewClassicGame creates a new classic-rules game for the given player IDs,
// validating that there are between 3 and 6 players, building the standard
// board and deck, randomizing player turn order, and allocating starting
// army reserves. The returned game starts in PhaseSetupClaim with no
// territories claimed; callers normally use NewClassicAutoStartGame or
// NewClassicRandomTerritoryGame instead, since PhaseSetupClaim is not
// driven by any UI. If rng is nil, a crypto/rand-backed RNG is used.
func NewClassicGame(playerIDs []string, rng RNG) (*Game, error) {
	if len(playerIDs) < 3 || len(playerIDs) > 6 {
		return nil, ErrInvalidPlayerCount
	}
	if rng == nil {
		rng = stdRNG{}
	}
	b := ClassicBoard()
	if err := b.Validate(); err != nil {
		return nil, err
	}

	g := &Game{
		Board:         b,
		Players:       make([]PlayerState, len(playerIDs)),
		Territories:   make(map[Territory]TerritoryState, len(b.Order)),
		CurrentPlayer: 0,
		Phase:         PhaseSetupClaim,
		SetupReserves: map[int]int{},
		Deck:          ClassicDeck(b.Order),
		rng:           rng,
	}
	shuffled := make([]string, len(playerIDs))
	copy(shuffled, playerIDs)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	for i, id := range shuffled {
		g.Players[i] = PlayerState{ID: id}
	}
	starting := map[int]int{3: 35, 4: 30, 5: 25, 6: 20}[len(playerIDs)]
	for i := range g.Players {
		g.SetupReserves[i] = starting
	}
	for _, t := range b.Order {
		g.Territories[t] = TerritoryState{Owner: -1, Armies: 0}
	}
	shuffleCards(g.rng, g.Deck)
	return g, nil
}

// NewClassicAutoStartGame creates a classic game and automatically completes setup.
// Territories are distributed across players and remaining starting armies are placed
// on owned territories, then the game begins at reinforce phase.
func NewClassicAutoStartGame(playerIDs []string, rng RNG) (*Game, error) {
	g, err := NewClassicGame(playerIDs, rng)
	if err != nil {
		return nil, err
	}

	order := append([]Territory(nil), g.Board.Order...)
	shuffleTerritories(g.rng, order)

	owned := make(map[int][]Territory, len(g.Players))
	for i, t := range order {
		owner := i % len(g.Players)
		g.Territories[t] = TerritoryState{Owner: owner, Armies: 1}
		g.SetupReserves[owner]--
		owned[owner] = append(owned[owner], t)
	}

	for pi := range g.Players {
		terr := owned[pi]
		if len(terr) == 0 {
			continue
		}
		for g.SetupReserves[pi] > 0 {
			pick := terr[g.rng.IntN(len(terr))]
			ts := g.Territories[pick]
			ts.Armies++
			g.Territories[pick] = ts
			g.SetupReserves[pi]--
		}
	}

	g.CurrentPlayer = 0
	g.Phase = PhaseReinforce
	g.startTurn()
	return g, nil
}

// NewClassicRandomTerritoryGame creates a classic game with territories randomly
// pre-assigned to players (1 army each), then starts in PhaseSetupReinforce so
// players can manually place their remaining starting armies before the game begins.
//
// Naming note: "RandomTerritory" describes the initial territory deal, but
// NewClassicAutoStartGame deals territories the exact same random way --
// it's not what actually distinguishes the two constructors. The real
// difference is that this one leaves the *remaining* starting armies for
// players to place manually, which is also how internal/service's own
// lobbyState.SetupMode == "manual" already refers to this same mode. Worth
// renaming this constructor (and the callers below) to something like
// NewClassicManualSetupGame to match that terminology and stop the two
// layers naming the identical concept two different ways -- not done yet,
// deliberately deferred rather than renamed alongside internal/simulation's
// GameModeManual (see that type's doc comment).
func NewClassicRandomTerritoryGame(playerIDs []string, rng RNG) (*Game, error) {
	g, err := NewClassicGame(playerIDs, rng)
	if err != nil {
		return nil, err
	}

	order := append([]Territory(nil), g.Board.Order...)
	shuffleTerritories(g.rng, order)

	for i, t := range order {
		owner := i % len(g.Players)
		g.Territories[t] = TerritoryState{Owner: owner, Armies: 1}
		g.SetupReserves[owner]--
	}

	g.CurrentPlayer = 0
	g.Phase = PhaseSetupReinforce
	return g, nil
}

// ClaimTerritory assigns an unclaimed territory to the current player
// during PhaseSetupClaim, placing one army on it and consuming one setup
// reserve. It returns ErrInvalidPhase outside PhaseSetupClaim, ErrOutOfTurn
// if playerID is not the current player, and a wrapped ErrInvalidMove for
// an unknown or already-claimed territory. Once every territory is
// claimed, Phase advances to PhaseSetupReinforce; turn order always
// advances to the next player with reserves left.
func (g *Game) ClaimTerritory(playerID string, t Territory) error {
	if g.Phase != PhaseSetupClaim {
		return ErrInvalidPhase
	}
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return err
	}
	ts, ok := g.Territories[t]
	if !ok {
		return fmt.Errorf("%w: unknown territory %q", ErrInvalidMove, t)
	}
	if ts.Owner != -1 {
		return fmt.Errorf("%w: territory already claimed", ErrInvalidMove)
	}
	ts.Owner = pi
	ts.Armies = 1
	g.Territories[t] = ts
	g.SetupReserves[pi]--

	if g.allClaimed() {
		g.Phase = PhaseSetupReinforce
	}
	g.advanceTurnInSetup()
	return nil
}

// PlaceInitialArmy places one reserve army on a territory the calling
// player owns during PhaseSetupReinforce. Unlike most actions, any player
// with reserves left may call this out of strict turn order — it returns
// ErrOutOfTurn only if playerID does not match a player, and a wrapped
// ErrInvalidMove if the player has no reserves left or does not own the
// territory. Once every player has exhausted their reserves, Phase
// advances to PhaseReinforce, play starts with player 0, and startTurn
// computes their initial reinforcements.
func (g *Game) PlaceInitialArmy(playerID string, t Territory) error {
	if g.Phase != PhaseSetupReinforce {
		return ErrInvalidPhase
	}
	pi := -1
	for i, p := range g.Players {
		if p.ID == playerID {
			pi = i
			break
		}
	}
	if pi == -1 {
		return ErrOutOfTurn
	}
	if g.SetupReserves[pi] <= 0 {
		return fmt.Errorf("%w: no reserves left", ErrInvalidMove)
	}
	ts := g.Territories[t]
	if ts.Owner != pi {
		return fmt.Errorf("%w: territory not owned by player", ErrInvalidMove)
	}
	ts.Armies++
	g.Territories[t] = ts
	g.SetupReserves[pi]--

	if g.setupDone() {
		g.Phase = PhaseReinforce
		g.CurrentPlayer = 0
		g.startTurn()
	}
	return nil
}

// TradeCards trades in three of the current player's cards, identified by
// hand indices idx, for reinforcement armies. It is only legal during
// PhaseReinforce and requires the current player, three unique indices,
// and a valid set (three matching symbols, one of each symbol, or
// including a Wild — see isValidSet). The reinforcement value follows
// nextTradeValue based on the running SetsTraded count, plus a one-time +2
// territory bonus if the player owns a territory depicted on one of the
// traded cards. Traded cards move to Discard. If this call satisfies an
// outstanding ForcedCardTrade (hand drops below five cards and no
// reinforcements remain pending), Phase advances directly to PhaseAttack.
// It returns the total reinforcement value granted, or an error
// (ErrOutOfTurn, ErrInvalidPhase, or a wrapped ErrInvalidMove) if the
// request is illegal.
func (g *Game) TradeCards(playerID string, idx [3]int) (int, error) {
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return 0, err
	}
	if g.Phase != PhaseReinforce {
		return 0, ErrInvalidPhase
	}
	p := &g.Players[pi]
	if len(p.Cards) < 3 {
		return 0, fmt.Errorf("%w: not enough cards", ErrInvalidMove)
	}
	if idx[0] == idx[1] || idx[0] == idx[2] || idx[1] == idx[2] {
		return 0, fmt.Errorf("%w: card indexes must be unique", ErrInvalidMove)
	}

	cards := []Card{
		p.Cards[idx[0]],
		p.Cards[idx[1]],
		p.Cards[idx[2]],
	}
	if !isValidSet(cards) {
		return 0, fmt.Errorf("%w: invalid card set", ErrInvalidMove)
	}

	value := nextTradeValue(g.SetsTraded + 1)
	bonus := 0
	if !g.TerritoryBonusUsed {
		for _, c := range cards {
			if c.Symbol == Wild {
				continue
			}
			ts := g.Territories[c.Territory]
			if ts.Owner == pi {
				bonus = 2
				g.TerritoryBonusUsed = true
				break
			}
		}
	}
	g.SetsTraded++
	g.PendingReinforcements += value + bonus

	remove := []int{idx[0], idx[1], idx[2]}
	slices.Sort(remove)
	for i := len(remove) - 1; i >= 0; i-- {
		j := remove[i]
		g.Discard = append(g.Discard, p.Cards[j])
		p.Cards = append(p.Cards[:j], p.Cards[j+1:]...)
	}
	if g.ForcedCardTrade && len(p.Cards) < 5 && g.PendingReinforcements == 0 {
		g.ForcedCardTrade = false
		g.Phase = PhaseAttack
	}
	return value + bonus, nil
}

// PlaceReinforcement places armies from the current player's pending
// reinforcement pool onto a territory they own. It is only legal during
// PhaseReinforce, requires the current player, and rejects a non-positive
// or over-budget army count and a hand of five or more cards (which must
// be traded down first via TradeCards). Once PendingReinforcements reaches
// zero, Phase advances to PhaseAttack.
func (g *Game) PlaceReinforcement(playerID string, t Territory, armies int) error {
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return err
	}
	if g.Phase != PhaseReinforce {
		return ErrInvalidPhase
	}
	if armies <= 0 || armies > g.PendingReinforcements {
		return fmt.Errorf("%w: invalid reinforcement count", ErrInvalidMove)
	}
	if len(g.Players[pi].Cards) >= 5 {
		return fmt.Errorf("%w: must trade cards first", ErrInvalidMove)
	}
	ts := g.Territories[t]
	if ts.Owner != pi {
		return fmt.Errorf("%w: territory not owned by player", ErrInvalidMove)
	}
	ts.Armies += armies
	g.Territories[t] = ts
	g.PendingReinforcements -= armies
	if g.PendingReinforcements == 0 {
		g.Phase = PhaseAttack
		g.ForcedCardTrade = false
	}
	return nil
}

// Attack resolves one round of combat from an owned territory against an
// adjacent enemy territory, enforcing the standard Risk dice rules: up to 3
// attacker dice (never more than one less than the source's army count)
// and up to 2 defender dice (never more than the target's army count),
// both at least 1. Dice are rolled via the game's RNG, sorted descending,
// and paired highest-to-highest; each pairing where the defender's die is
// greater than or equal to the attacker's die costs the attacker an army
// (ties favor the defender), otherwise the defender loses an army. If the
// defending territory's armies reach zero, ownership transfers to the
// attacker, Phase becomes PhaseOccupy with an OccupyState bounding the
// required troop movement, ConqueredThisTurn is set, and if the defender
// has no territories left they are eliminated and their cards transferred
// to the attacker. It returns the AttackResult, a DomainEvent carrying a
// CombatRollResolvedPayload describing the roll for persistence, and an
// error (ErrOutOfTurn, ErrInvalidPhase, or a wrapped ErrInvalidMove) if the
// request is illegal.
func (g *Game) Attack(playerID string, from, to Territory, attackerDice, defenderDice int) (AttackResult, *DomainEvent, error) {
	g.ensureRNG()
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return AttackResult{}, nil, err
	}
	if g.Phase != PhaseAttack {
		return AttackResult{}, nil, ErrInvalidPhase
	}
	if !g.Board.IsAdjacent(from, to) {
		return AttackResult{}, nil, fmt.Errorf("%w: territories not adjacent", ErrInvalidMove)
	}
	src, ok := g.Territories[from]
	if !ok {
		return AttackResult{}, nil, fmt.Errorf("%w: unknown source territory", ErrInvalidMove)
	}
	dst, ok := g.Territories[to]
	if !ok {
		return AttackResult{}, nil, fmt.Errorf("%w: unknown target territory", ErrInvalidMove)
	}
	if src.Owner != pi {
		return AttackResult{}, nil, fmt.Errorf("%w: source territory not owned by player", ErrInvalidMove)
	}
	if dst.Owner == pi || dst.Owner < 0 {
		return AttackResult{}, nil, fmt.Errorf("%w: invalid target owner", ErrInvalidMove)
	}
	if src.Armies <= 1 {
		return AttackResult{}, nil, fmt.Errorf("%w: not enough armies to attack", ErrInvalidMove)
	}
	maxAttDice := min(3, src.Armies-1)
	if attackerDice < 1 || attackerDice > maxAttDice {
		return AttackResult{}, nil, fmt.Errorf("%w: invalid attacker dice", ErrInvalidMove)
	}
	maxDefDice := min(2, dst.Armies)
	if defenderDice < 1 || defenderDice > maxDefDice {
		return AttackResult{}, nil, fmt.Errorf("%w: invalid defender dice", ErrInvalidMove)
	}

	defenderPlayerID := g.Players[dst.Owner].ID
	srcArmiesBefore := src.Armies
	dstArmiesBefore := dst.Armies

	ar := AttackResult{
		AttackerRolls: rollDice(g.rng, attackerDice),
		DefenderRolls: rollDice(g.rng, defenderDice),
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ar.AttackerRolls)))
	sort.Sort(sort.Reverse(sort.IntSlice(ar.DefenderRolls)))

	comparisons := make([]DieComparison, 0, min(len(ar.AttackerRolls), len(ar.DefenderRolls)))
	for i := 0; i < min(len(ar.AttackerRolls), len(ar.DefenderRolls)); i++ {
		loser := "defender"
		if ar.AttackerRolls[i] > ar.DefenderRolls[i] {
			ar.DefenderLoss++
		} else {
			ar.AttackerLoss++
			loser = "attacker"
		}
		comparisons = append(comparisons, DieComparison{
			AttackerDie: ar.AttackerRolls[i],
			DefenderDie: ar.DefenderRolls[i],
			Loser:       loser,
		})
	}
	src.Armies -= ar.AttackerLoss
	dst.Armies -= ar.DefenderLoss
	g.Territories[from] = src
	g.Territories[to] = dst

	if dst.Armies == 0 {
		eliminated := g.Players[dst.Owner].ID
		dstOwner := dst.Owner
		dst.Owner = pi
		g.Territories[to] = dst
		g.Occupy = &OccupyState{
			From:    from,
			To:      to,
			MinMove: attackerDice,
			MaxMove: src.Armies - 1,
		}
		g.Phase = PhaseOccupy
		g.ConqueredThisTurn = true
		ar.Conquered = true
		if g.playerTerritoryCount(dstOwner) == 0 {
			g.Players[pi].Cards = append(g.Players[pi].Cards, g.Players[dstOwner].Cards...)
			g.Players[dstOwner].Cards = nil
			g.Players[dstOwner].Eliminated = true
			ar.Eliminated = eliminated
		}
	}

	event := &DomainEvent{
		Type:          EventTypeCombatRollResolved,
		Version:       EventVersionCombatRollResolved,
		ActorPlayerID: playerID,
		Payload: CombatRollResolvedPayload{
			SchemaVersion:      SchemaVersionCombatRollResolved,
			TurnNumber:         g.TurnNumber,
			Phase:              string(PhaseAttack),
			AttackerPlayerID:   playerID,
			DefenderPlayerID:   defenderPlayerID,
			SourceTerritoryID:  string(from),
			TargetTerritoryID:  string(to),
			SourceArmiesBefore: srcArmiesBefore,
			TargetArmiesBefore: dstArmiesBefore,
			AttackerDice:       ar.AttackerRolls,
			DefenderDice:       ar.DefenderRolls,
			Comparisons:        comparisons,
			AttackerLosses:     ar.AttackerLoss,
			DefenderLosses:     ar.DefenderLoss,
			SourceArmiesAfter:  g.Territories[from].Armies,
			TargetArmiesAfter:  g.Territories[to].Armies,
			TerritoryCaptured:  ar.Conquered,
		},
	}
	return ar, event, nil
}

// OccupyTerritory moves armies from the attacking territory into a
// just-conquered one, completing the post-Attack occupation required by an
// active OccupyState. It is only legal during PhaseOccupy, requires the
// current player to own both territories in Occupy, and enforces armies
// within [Occupy.MinMove, Occupy.MaxMove]. On success it clears Occupy,
// returns Phase to PhaseAttack, checks for a winner, and — if the player's
// hand has grown to five or more cards — sets ForcedCardTrade and routes
// back to PhaseReinforce until they trade down.
func (g *Game) OccupyTerritory(playerID string, armies int) error {
	if g.Phase != PhaseOccupy || g.Occupy == nil {
		return ErrInvalidPhase
	}
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return err
	}
	from := g.Territories[g.Occupy.From]
	to := g.Territories[g.Occupy.To]
	if from.Owner != pi || to.Owner != pi {
		return fmt.Errorf("%w: occupy state ownership mismatch", ErrInvalidMove)
	}
	if armies < g.Occupy.MinMove || armies > g.Occupy.MaxMove {
		return fmt.Errorf("%w: invalid occupy armies", ErrInvalidMove)
	}
	from.Armies -= armies
	to.Armies += armies
	g.Territories[g.Occupy.From] = from
	g.Territories[g.Occupy.To] = to
	g.Occupy = nil
	g.Phase = PhaseAttack
	g.checkWinner()
	if g.Phase != PhaseGameOver && len(g.Players[pi].Cards) >= 5 {
		g.ForcedCardTrade = true
		g.Phase = PhaseReinforce
	}
	return nil
}

// EndAttackPhase lets the current player voluntarily leave PhaseAttack and
// move on to PhaseFortify without launching any further attacks. It
// returns ErrInvalidPhase outside PhaseAttack and ErrOutOfTurn if playerID
// is not the current player.
func (g *Game) EndAttackPhase(playerID string) error {
	if g.Phase != PhaseAttack {
		return ErrInvalidPhase
	}
	if _, err := g.requireCurrentPlayer(playerID); err != nil {
		return err
	}
	g.Phase = PhaseFortify
	return nil
}

// Fortify moves armies between two territories the current player owns,
// connected through a chain of the player's own territories (see
// isContiguous). It is only legal during PhaseFortify, permits at most one
// such move per turn (HasFortified), and requires a positive army count
// that leaves at least one army behind in the source territory.
func (g *Game) Fortify(playerID string, from, to Territory, armies int) error {
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return err
	}
	if g.Phase != PhaseFortify {
		return ErrInvalidPhase
	}
	if g.HasFortified {
		return fmt.Errorf("%w: already fortified this turn", ErrInvalidMove)
	}
	src := g.Territories[from]
	dst := g.Territories[to]
	if src.Owner != pi || dst.Owner != pi {
		return fmt.Errorf("%w: both territories must be owned by player", ErrInvalidMove)
	}
	if !g.isContiguous(from, to, pi) {
		return fmt.Errorf("%w: territories not connected through owned territories", ErrInvalidMove)
	}
	if armies <= 0 || armies >= src.Armies {
		return fmt.Errorf("%w: invalid fortify armies", ErrInvalidMove)
	}
	src.Armies -= armies
	dst.Armies += armies
	g.Territories[from] = src
	g.Territories[to] = dst
	g.HasFortified = true
	return nil
}

// EndTurn ends the current player's turn from PhaseAttack or PhaseFortify.
// If the player conquered at least one territory this turn, they draw a
// card first (reshuffling Discard into Deck if needed). It then checks for
// a winner — ending the game and setting Winner if only one player still
// holds territory — or otherwise advances CurrentPlayer to the next
// player with territory remaining and starts their turn via startTurn.
func (g *Game) EndTurn(playerID string) error {
	_, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return err
	}
	if g.Phase != PhaseFortify && g.Phase != PhaseAttack {
		return ErrInvalidPhase
	}
	if g.ConqueredThisTurn {
		g.drawCard(g.CurrentPlayer)
	}
	g.checkWinner()
	if g.Phase == PhaseGameOver {
		return nil
	}
	g.advanceToNextPlayer()
	g.startTurn()
	return nil
}

func (g *Game) startTurn() {
	g.TurnNumber++
	g.Phase = PhaseReinforce
	g.ConqueredThisTurn = false
	g.TerritoryBonusUsed = false
	g.HasFortified = false
	g.ForcedCardTrade = false
	g.Occupy = nil
	g.PendingReinforcements = g.reinforcementsFor(g.CurrentPlayer)
}

func (g *Game) reinforcementsFor(pi int) int {
	tc := g.playerTerritoryCount(pi)
	base := max(3, tc/3)
	for _, c := range g.Board.Continents {
		ok := true
		for _, t := range c.Territories {
			if g.Territories[t].Owner != pi {
				ok = false
				break
			}
		}
		if ok {
			base += c.Bonus
		}
	}
	return base
}

func (g *Game) drawCard(pi int) {
	g.ensureRNG()
	if len(g.Deck) == 0 {
		if len(g.Discard) == 0 {
			return
		}
		g.Deck = append(g.Deck, g.Discard...)
		g.Discard = nil
		shuffleCards(g.rng, g.Deck)
	}
	card := g.Deck[0]
	g.Deck = g.Deck[1:]
	g.Players[pi].Cards = append(g.Players[pi].Cards, card)
}

func (g *Game) checkWinner() {
	active := 0
	last := -1
	for i := range g.Players {
		if g.playerTerritoryCount(i) > 0 {
			active++
			last = i
		}
	}
	if active == 1 {
		g.Phase = PhaseGameOver
		g.Winner = g.Players[last].ID
	}
}

func (g *Game) playerTerritoryCount(pi int) int {
	count := 0
	for _, ts := range g.Territories {
		if ts.Owner == pi {
			count++
		}
	}
	return count
}

func (g *Game) isContiguous(from, to Territory, pi int) bool {
	visited := map[Territory]bool{from: true}
	queue := []Territory{from}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for neighbor := range g.Board.Adjacent[curr] {
			if neighbor == to {
				return true
			}
			if !visited[neighbor] && g.Territories[neighbor].Owner == pi {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return false
}

func (g *Game) requireCurrentPlayer(playerID string) (int, error) {
	if g.Phase == PhaseGameOver {
		return -1, ErrInvalidPhase
	}
	if g.Players[g.CurrentPlayer].ID != playerID {
		return -1, ErrOutOfTurn
	}
	return g.CurrentPlayer, nil
}

func (g *Game) allClaimed() bool {
	for _, ts := range g.Territories {
		if ts.Owner == -1 {
			return false
		}
	}
	return true
}

func (g *Game) setupDone() bool {
	for _, r := range g.SetupReserves {
		if r > 0 {
			return false
		}
	}
	return true
}

func (g *Game) advanceTurnInSetup() {
	n := len(g.Players)
	for i := 1; i <= n; i++ {
		ni := (g.CurrentPlayer + i) % n
		if g.Phase == PhaseSetupClaim || g.SetupReserves[ni] > 0 {
			g.CurrentPlayer = ni
			return
		}
	}
}

func (g *Game) advanceToNextPlayer() {
	n := len(g.Players)
	for i := 1; i <= n; i++ {
		ni := (g.CurrentPlayer + i) % n
		if g.playerTerritoryCount(ni) > 0 {
			g.CurrentPlayer = ni
			return
		}
	}
}

func isValidSet(cards []Card) bool {
	if len(cards) != 3 {
		return false
	}
	wilds := 0
	count := map[Symbol]int{}
	for _, c := range cards {
		if c.Symbol == Wild {
			wilds++
		} else {
			count[c.Symbol]++
		}
	}
	if wilds >= 1 {
		return true
	}
	if len(count) == 1 {
		return true
	}
	return len(count) == 3
}

func nextTradeValue(setNumber int) int {
	if setNumber <= 5 {
		return 2*setNumber + 2
	}
	if setNumber == 6 {
		return 15
	}
	return 15 + (setNumber-6)*5
}

func (g *Game) ensureRNG() {
	if g.rng == nil {
		g.rng = stdRNG{}
	}
}

func rollDice(rng RNG, n int) []int {
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = rng.IntN(6) + 1
	}
	return out
}

func shuffleCards(rng RNG, cards []Card) {
	for i := len(cards) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		cards[i], cards[j] = cards[j], cards[i]
	}
}

func shuffleTerritories(rng RNG, territories []Territory) {
	for i := len(territories) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		territories[i], territories[j] = territories[j], territories[i]
	}
}
