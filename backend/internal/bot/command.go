package bot

import "backend/internal/game"

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
	Action       string
	Territory    string
	From         string
	To           string
	Armies       int
	AttackerDice int
	DefenderDice int
	CardIndices  [3]int
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
