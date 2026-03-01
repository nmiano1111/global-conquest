package risk

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
)

type Phase string

const (
	PhaseSetupClaim     Phase = "setup_claim"
	PhaseSetupReinforce Phase = "setup_reinforce"
	PhaseReinforce      Phase = "reinforce"
	PhaseAttack         Phase = "attack"
	PhaseOccupy         Phase = "occupy"
	PhaseFortify        Phase = "fortify"
	PhaseGameOver       Phase = "game_over"
)

var (
	ErrInvalidPlayerCount = errors.New("risk: player count must be between 3 and 6")
	ErrOutOfTurn          = errors.New("risk: out of turn")
	ErrInvalidPhase       = errors.New("risk: action not allowed in current phase")
	ErrInvalidMove        = errors.New("risk: invalid move")
)

type RNG interface {
	IntN(n int) int
}

type stdRNG struct{}

func (stdRNG) IntN(n int) int { return rand.IntN(n) }

type PlayerState struct {
	ID         string `json:"id"`
	Cards      []Card `json:"cards"`
	Eliminated bool   `json:"eliminated"`
}

type TerritoryState struct {
	Owner  int `json:"owner"`
	Armies int `json:"armies"`
}

type OccupyState struct {
	From    Territory `json:"from"`
	To      Territory `json:"to"`
	MinMove int       `json:"min_move"`
	MaxMove int       `json:"max_move"`
}

type AttackResult struct {
	AttackerRolls []int  `json:"attacker_rolls"`
	DefenderRolls []int  `json:"defender_rolls"`
	AttackerLoss  int    `json:"attacker_loss"`
	DefenderLoss  int    `json:"defender_loss"`
	Conquered     bool   `json:"conquered"`
	Eliminated    string `json:"eliminated"`
}

type Game struct {
	Board       Board                        `json:"board"`
	Players     []PlayerState                `json:"players"`
	Territories map[Territory]TerritoryState `json:"territories"`

	CurrentPlayer int    `json:"current_player"`
	Phase         Phase  `json:"phase"`
	Winner        string `json:"winner"`

	SetupReserves map[int]int `json:"setup_reserves"`

	PendingReinforcements int          `json:"pending_reinforcements"`
	ConqueredThisTurn     bool         `json:"conquered_this_turn"`
	TerritoryBonusUsed    bool         `json:"territory_bonus_used"`
	HasFortified          bool         `json:"has_fortified"`
	Occupy                *OccupyState `json:"occupy"`

	Deck       []Card `json:"deck"`
	Discard    []Card `json:"discard"`
	SetsTraded int    `json:"sets_traded"`

	rng RNG `json:"-"`
}

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
	for i, id := range playerIDs {
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

func (g *Game) PlaceInitialArmy(playerID string, t Territory) error {
	if g.Phase != PhaseSetupReinforce {
		return ErrInvalidPhase
	}
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return err
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
		return nil
	}
	g.advanceTurnInSetup()
	return nil
}

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
	return value + bonus, nil
}

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
	}
	return nil
}

func (g *Game) Attack(playerID string, from, to Territory, attackerDice, defenderDice int) (AttackResult, error) {
	pi, err := g.requireCurrentPlayer(playerID)
	if err != nil {
		return AttackResult{}, err
	}
	if g.Phase != PhaseAttack {
		return AttackResult{}, ErrInvalidPhase
	}
	if !g.Board.IsAdjacent(from, to) {
		return AttackResult{}, fmt.Errorf("%w: territories not adjacent", ErrInvalidMove)
	}
	src, ok := g.Territories[from]
	if !ok {
		return AttackResult{}, fmt.Errorf("%w: unknown source territory", ErrInvalidMove)
	}
	dst, ok := g.Territories[to]
	if !ok {
		return AttackResult{}, fmt.Errorf("%w: unknown target territory", ErrInvalidMove)
	}
	if src.Owner != pi {
		return AttackResult{}, fmt.Errorf("%w: source territory not owned by player", ErrInvalidMove)
	}
	if dst.Owner == pi || dst.Owner < 0 {
		return AttackResult{}, fmt.Errorf("%w: invalid target owner", ErrInvalidMove)
	}
	if src.Armies <= 1 {
		return AttackResult{}, fmt.Errorf("%w: not enough armies to attack", ErrInvalidMove)
	}
	maxAttDice := min(3, src.Armies-1)
	if attackerDice < 1 || attackerDice > maxAttDice {
		return AttackResult{}, fmt.Errorf("%w: invalid attacker dice", ErrInvalidMove)
	}
	maxDefDice := min(2, dst.Armies)
	if defenderDice < 1 || defenderDice > maxDefDice {
		return AttackResult{}, fmt.Errorf("%w: invalid defender dice", ErrInvalidMove)
	}

	ar := AttackResult{
		AttackerRolls: rollDice(g.rng, attackerDice),
		DefenderRolls: rollDice(g.rng, defenderDice),
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ar.AttackerRolls)))
	sort.Sort(sort.Reverse(sort.IntSlice(ar.DefenderRolls)))

	for i := 0; i < min(len(ar.AttackerRolls), len(ar.DefenderRolls)); i++ {
		if ar.AttackerRolls[i] > ar.DefenderRolls[i] {
			ar.DefenderLoss++
		} else {
			ar.AttackerLoss++
		}
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
	return ar, nil
}

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
	return nil
}

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
	if !g.Board.IsAdjacent(from, to) {
		return fmt.Errorf("%w: territories not adjacent", ErrInvalidMove)
	}
	src := g.Territories[from]
	dst := g.Territories[to]
	if src.Owner != pi || dst.Owner != pi {
		return fmt.Errorf("%w: both territories must be owned by player", ErrInvalidMove)
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
	g.Phase = PhaseReinforce
	g.ConqueredThisTurn = false
	g.TerritoryBonusUsed = false
	g.HasFortified = false
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
