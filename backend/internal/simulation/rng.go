package simulation

import (
	"math/rand/v2"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// NewDeterministicRNG returns a risk.RNG seeded from seed: the same seed
// always produces the same draw sequence, and thus -- since every
// nondeterministic path in internal/risk (player-order shuffle, deck
// shuffle, territory-order shuffle, extra starting-army placement, combat
// dice) is funneled through this single-method interface -- the same
// entire game.
//
// math/rand/v2's *rand.Rand already has an IntN(n int) int method
// matching risk.RNG's one required method exactly, so no wrapper type is
// needed; PCG (rand/v2's general-purpose, non-cryptographic source) is
// the right choice here since nothing about dice-rolling or shuffling a
// deck needs cryptographic guarantees, only reproducibility.
//
// The *risk.Game constructed with this RNG must stay alive in-process for
// the entire simulation. risk.Game's own rng field is unexported and
// tagged json:"-": marshaling or unmarshaling a live *risk.Game silently
// drops it, and the next call that needs randomness (Attack, drawCard)
// lazily falls back to crypto/rand -- non-deterministic, no error, no
// warning. Only marshal a simulated game for final, read-only output,
// never mid-run.
func NewDeterministicRNG(seed int64) risk.RNG {
	return rand.New(rand.NewPCG(uint64(seed), uint64(seed)+1))
}
