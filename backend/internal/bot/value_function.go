package bot

// ValueFunction scores an already-encoded, flat whole-board feature
// vector (tdstate.Encode(g, pi).Flatten()) and reports the margin
// ValueStrategy's attack/fortify phases require a candidate's afterstate
// score to exceed the current state's score by before acting on it,
// instead of ending the phase -- implemented by both BoardValue (linear)
// and gcnmodel.Model (GCN), so ValueStrategy's phase logic (afterstate
// computation, margin-gated attack/fortify, unconditional
// reinforce/occupy) works identically regardless of which model class
// actually produced the score.
type ValueFunction interface {
	Score(features []float64) float64
	AttackMargin() float64
	FortifyMargin() float64
}
