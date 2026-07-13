// Package bot implements bot-controlled players for Global Conquest. A
// Strategy inspects read-only authoritative risk.Game state — using the
// risk.Legal* query helpers rather than duplicating engine rules — and
// picks one Command, the same action shape a human WebSocket client
// submits. A Runner drives one bot's turn by submitting those commands
// through the same application path human commands use, and a Manager
// ensures at most one runner is active per game at a time.
package bot

import "github.com/nmiano1111/global-conquest/backend/internal/game"

// Action values mirror the action strings service.GamesService.ApplyGameAction
// switches on. They are duplicated here (rather than imported) because the
// service package sits above game/bot in the dependency graph and must not
// be imported from here; keeping the literal strings identical is what
// makes a bot Command behave exactly like the equivalent human command.
const (
	ActionPlaceInitialArmy   = "place_initial_army"
	ActionTradeCards         = "trade_cards"
	ActionPlaceReinforcement = "place_reinforcement"
	ActionAttack             = "attack"
	ActionOccupy             = "occupy"
	ActionEndAttack          = "end_attack"
	ActionFortify            = "fortify"
	ActionEndTurn            = "end_turn"
)

// Command is the same shape of command a human player submits over
// WebSocket. A Strategy returns one Command per decision; the runner
// submits it verbatim through the normal application command path.
type Command struct {
	// Action names the action to perform, one of the Action* constants.
	Action string
	// Territory is the single territory targeted by the action, when
	// applicable.
	Territory string
	// From is the source territory for actions that move armies.
	From string
	// To is the destination territory for actions that move armies.
	To string
	// Armies is the number of armies involved in the action.
	Armies int
	// AttackerDice is the number of dice the attacker rolls.
	AttackerDice int
	// DefenderDice is the number of dice the defender rolls.
	DefenderDice int
	// CardIndices identifies the up-to-three cards selected from the
	// player's hand, for card-trading actions.
	CardIndices [3]int
}

// toGameActionInput attaches the game and actor identity the runner knows
// about but the strategy does not need to.
func (c Command) toGameActionInput(gameID, playerID string) game.GameActionInput {
	return game.GameActionInput{
		GameID:       gameID,
		PlayerUserID: playerID,
		Action:       c.Action,
		Territory:    c.Territory,
		From:         c.From,
		To:           c.To,
		Armies:       c.Armies,
		AttackerDice: c.AttackerDice,
		DefenderDice: c.DefenderDice,
		CardIndices:  c.CardIndices,
	}
}
