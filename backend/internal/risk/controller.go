package risk

// ControllerType distinguishes human- from bot-controlled players. It is
// descriptive metadata only: the engine never branches on it, and it does
// not affect legality or state transitions.
type ControllerType string

const (
	ControllerHuman ControllerType = "human"
	ControllerBot   ControllerType = "bot"
)

// IsBot reports whether the player is bot-controlled. Games serialized
// before this field existed decode with a zero-value Controller, which
// correctly reports false here (defaults to human).
func (p PlayerState) IsBot() bool {
	return p.Controller == ControllerBot
}
