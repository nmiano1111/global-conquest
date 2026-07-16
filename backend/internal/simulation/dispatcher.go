package simulation

import (
	"errors"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// ErrUnknownCommand is returned by Dispatch when a Command's Action isn't
// one of bot's 8 known constants -- unreachable from any strategy shipped
// today, but the dispatcher fails loudly rather than silently no-op'ing
// if a future or buggy strategy ever returns something else.
var ErrUnknownCommand = errors.New("simulation: unknown command action")

// DispatchResult carries whatever a dispatched command's engine call
// returned beyond a plain error, for the caller to fold into Result
// counters and trace records. Only attack and trade_cards produce
// anything beyond success/failure -- every other field is the zero value
// for every other action.
type DispatchResult struct {
	// AttackResult and DomainEvent are populated only for bot.ActionAttack.
	AttackResult risk.AttackResult
	DomainEvent  *risk.DomainEvent

	// ReinforcementsGranted is populated only for bot.ActionTradeCards --
	// TradeCards' own return value (the armies granted by that trade).
	ReinforcementsGranted int
}

// Dispatch translates cmd into the matching risk.Game method call and
// applies it, per the 8-row mapping in the simulation framework design
// doc. It never retries: this simulator is single-threaded and always
// re-reads authoritative state before every strategy decision, so an
// engine rejection can only mean the strategy (or this dispatcher) itself
// produced an illegal command against the exact state it just observed --
// callers should treat any returned error as a hard failure
// (FailureEngineRejectedCommand), never something worth retrying.
func Dispatch(g *risk.Game, playerID string, cmd bot.Command) (DispatchResult, error) {
	switch cmd.Action {
	case bot.ActionPlaceInitialArmy:
		err := g.PlaceInitialArmy(playerID, risk.Territory(cmd.Territory))
		return DispatchResult{}, err

	case bot.ActionTradeCards:
		armies, err := g.TradeCards(playerID, cmd.CardIndices)
		return DispatchResult{ReinforcementsGranted: armies}, err

	case bot.ActionPlaceReinforcement:
		err := g.PlaceReinforcement(playerID, risk.Territory(cmd.Territory), cmd.Armies)
		return DispatchResult{}, err

	case bot.ActionAttack:
		// bot.Command.DefenderDice is always the zero value -- no
		// strategy sets it. Compute it independently, the same way
		// production's ApplyGameAction does, rather than trusting the
		// command: min(2, the target's current army count).
		defenderDice := min(2, g.Territories[risk.Territory(cmd.To)].Armies)
		ar, ev, err := g.Attack(playerID, risk.Territory(cmd.From), risk.Territory(cmd.To), cmd.AttackerDice, defenderDice)
		return DispatchResult{AttackResult: ar, DomainEvent: ev}, err

	case bot.ActionEndAttack:
		err := g.EndAttackPhase(playerID)
		return DispatchResult{}, err

	case bot.ActionOccupy:
		// From/To are not read from cmd for this action -- they come
		// from g.Occupy, set by the preceding Attack call. The bot
		// command only ever carries Armies here.
		err := g.OccupyTerritory(playerID, cmd.Armies)
		return DispatchResult{}, err

	case bot.ActionFortify:
		err := g.Fortify(playerID, risk.Territory(cmd.From), risk.Territory(cmd.To), cmd.Armies)
		return DispatchResult{}, err

	case bot.ActionEndTurn:
		err := g.EndTurn(playerID)
		return DispatchResult{}, err

	default:
		return DispatchResult{}, fmt.Errorf("%w: %q", ErrUnknownCommand, cmd.Action)
	}
}
