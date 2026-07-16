package simulation

import (
	"encoding/json"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// drawSequence pulls n draws from IntN(bound), used to compare two RNG
// instances' output without depending on any particular internal state
// representation.
func drawSequence(t *testing.T, seed int64, bound, n int) []int {
	t.Helper()
	r := NewDeterministicRNG(seed)
	out := make([]int, n)
	for i := range out {
		out[i] = r.IntN(bound)
	}
	return out
}

func TestDeterministicRNGSameSeedProducesSameSequence(t *testing.T) {
	a := drawSequence(t, 12345, 100, 200)
	b := drawSequence(t, 12345, 100, 200)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("draw %d diverged: %d vs %d (same seed should reproduce identically)", i, a[i], b[i])
		}
	}
}

func TestDeterministicRNGDifferentSeedsDiverge(t *testing.T) {
	a := drawSequence(t, 1, 1_000_000, 50)
	b := drawSequence(t, 2, 1_000_000, 50)
	for i := range a {
		if a[i] != b[i] {
			return // found a divergence, as expected
		}
	}
	t.Fatalf("50 draws from a 1,000,000-wide range were identical across two different seeds -- RNG is not actually seed-dependent")
}

func TestDeterministicRNGRespectsBounds(t *testing.T) {
	r := NewDeterministicRNG(7)
	for range 1000 {
		v := r.IntN(6)
		if v < 0 || v >= 6 {
			t.Fatalf("IntN(6) returned %d, expected [0,6)", v)
		}
	}
}

// TestDeterministicRNGProducesIdenticalGameConstruction is the actual
// point of this file: the same seed fed to risk.NewClassicAutoStartGame
// via NewDeterministicRNG must produce byte-identical initial state --
// same shuffled player order, same territory distribution, same deck --
// across two entirely independent constructions.
func TestDeterministicRNGProducesIdenticalGameConstruction(t *testing.T) {
	ids := []string{"p0", "p1", "p2", "p3"}

	g1, err := risk.NewClassicAutoStartGame(ids, NewDeterministicRNG(42))
	if err != nil {
		t.Fatalf("first construction: %v", err)
	}
	g2, err := risk.NewClassicAutoStartGame(ids, NewDeterministicRNG(42))
	if err != nil {
		t.Fatalf("second construction: %v", err)
	}

	j1, err := json.Marshal(g1)
	if err != nil {
		t.Fatalf("marshal first game: %v", err)
	}
	j2, err := json.Marshal(g2)
	if err != nil {
		t.Fatalf("marshal second game: %v", err)
	}
	if string(j1) != string(j2) {
		t.Fatalf("two games constructed with the same seed diverged:\n%s\nvs\n%s", j1, j2)
	}
}

func TestDeterministicRNGSatisfiesRiskRNG(t *testing.T) {
	// Compile-time-adjacent check: NewDeterministicRNG's return type is
	// risk.RNG, so this only needs to build to pass -- but call it too,
	// so a future signature change that still compiles but breaks
	// behavior doesn't slip through silently.
	r := NewDeterministicRNG(1)
	if n := r.IntN(1); n != 0 {
		t.Fatalf("IntN(1) must always return 0, got %d", n)
	}
}
