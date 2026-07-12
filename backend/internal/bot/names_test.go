package bot

import (
	"strings"
	"testing"
)

// seqNameRNG deterministically cycles through a fixed sequence of values,
// letting tests control the exact shuffle result without real randomness.
type seqNameRNG struct {
	v []int
	i int
}

func (s *seqNameRNG) IntN(n int) int {
	if n <= 0 || len(s.v) == 0 {
		return 0
	}
	x := s.v[s.i%len(s.v)]
	s.i++
	if x < 0 {
		x = -x
	}
	return x % n
}

func TestWrestlerNamePoolNonEmpty(t *testing.T) {
	if len(WrestlerNames) == 0 {
		t.Fatalf("expected a non-empty wrestler name pool")
	}
}

func TestWrestlerNamePoolUnique(t *testing.T) {
	seen := make(map[string]struct{}, len(WrestlerNames))
	for _, n := range WrestlerNames {
		if strings.TrimSpace(n) == "" {
			t.Fatalf("pool contains an empty or blank name")
		}
		key := strings.ToLower(n)
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate name in pool: %q", n)
		}
		seen[key] = struct{}{}
	}
}

func TestAssignBotNamesUniqueWithinGame(t *testing.T) {
	names := AssignBotNames(&seqNameRNG{v: []int{0, 1, 2, 3, 4}}, 5, nil)
	if len(names) != 5 {
		t.Fatalf("expected 5 names, got %d", len(names))
	}
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			t.Fatalf("assigned an empty name")
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("duplicate assigned name: %q", n)
		}
		seen[n] = struct{}{}
	}
}

func TestAssignBotNamesDeterministicWithSeededSelector(t *testing.T) {
	rngA := &seqNameRNG{v: []int{3, 1, 4, 1, 5, 9, 2, 6}}
	rngB := &seqNameRNG{v: []int{3, 1, 4, 1, 5, 9, 2, 6}}
	a := AssignBotNames(rngA, 4, nil)
	b := AssignBotNames(rngB, 4, nil)
	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("expected identical output for identical seeds, got %v vs %v", a, b)
		}
	}
}

func TestAssignBotNamesExcludesHumanNames(t *testing.T) {
	pool := []string{"Sting", "Vader"}
	names := AssignBotNamesFromPool(pool, &seqNameRNG{v: []int{0}}, 1, []string{"sting"})
	if len(names) != 1 || names[0] != "Vader" {
		t.Fatalf("expected the excluded name to be skipped, got %v", names)
	}
}

func TestAssignBotNamesFallbackSuffixWhenExhausted(t *testing.T) {
	pool := []string{"Sting", "Vader"}
	names := AssignBotNamesFromPool(pool, &seqNameRNG{v: []int{0, 1}}, 5, nil)
	if len(names) != 5 {
		t.Fatalf("expected 5 names, got %d", len(names))
	}
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			t.Fatalf("assigned an empty name in fallback path")
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("fallback suffix behavior produced a duplicate: %q in %v", n, names)
		}
		seen[n] = struct{}{}
	}
	// First cycle through the 2-name pool must be unsuffixed; the second
	// cycle must carry a stable "II" suffix.
	base := map[string]bool{}
	suffixed := map[string]bool{}
	for _, n := range names {
		if strings.HasSuffix(n, " II") {
			suffixed[strings.TrimSuffix(n, " II")] = true
		} else {
			base[n] = true
		}
	}
	if !base["Sting"] || !base["Vader"] {
		t.Fatalf("expected both pool names to appear unsuffixed at least once, got %v", names)
	}
	if !suffixed["Sting"] && !suffixed["Vader"] {
		t.Fatalf("expected a roman-numeral suffix once the pool is exhausted, got %v", names)
	}
}

func TestAssignBotNamesZeroCount(t *testing.T) {
	if got := AssignBotNames(nil, 0, nil); got != nil {
		t.Fatalf("expected nil for zero count, got %v", got)
	}
}

func TestRomanNumeral(t *testing.T) {
	cases := map[int]string{1: "I", 2: "II", 3: "III", 4: "IV", 5: "V", 9: "IX", 10: "X"}
	for n, want := range cases {
		if got := romanNumeral(n); got != want {
			t.Fatalf("romanNumeral(%d) = %q, want %q", n, got, want)
		}
	}
}
