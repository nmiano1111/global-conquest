package bot

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// WrestlerNames is a curated pool of ring names from prominent professional
// wrestlers active in the 1980s and/or 1990s, used to assign display names
// to bot players. Every entry must be unique.
var WrestlerNames = []string{
	"Hulk Hogan",
	"Macho Man Randy Savage",
	"Bret Hart",
	"Shawn Michaels",
	"Ric Flair",
	"The Undertaker",
	"Sting",
	"Lex Luger",
	"Ultimate Warrior",
	"Roddy Piper",
	"Jake the Snake Roberts",
	"Ted DiBiase",
	"Mr. Perfect",
	"Dusty Rhodes",
	"Rick Rude",
	"Vader",
	"Diesel",
	"Razor Ramon",
	"Yokozuna",
	"British Bulldog",
	"Owen Hart",
	"Mick Foley",
	"Big Boss Man",
	"Bam Bam Bigelow",
	"Sid Vicious",
	"Booker T",
	"Goldberg",
	"Diamond Dallas Page",
	"Kevin Nash",
	"Scott Hall",
	"Rick Steiner",
	"Scott Steiner",
	"Animal",
	"Hawk",
	"Jim Duggan",
	"Earthquake",
	"Typhoon",
	"Tatanka",
	"Doink the Clown",
	"Sgt. Slaughter",
}

// NameRNG is the minimal randomness source AssignBotNames needs: an integer
// in [0, n). Production uses a crypto/rand-backed default; tests inject a
// deterministic fake.
type NameRNG interface {
	IntN(n int) int
}

type cryptoNameRNG struct{}

// IntN returns a cryptographically random integer in [0, n), or 0 if n is
// not positive.
func (cryptoNameRNG) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(fmt.Sprintf("bot: crypto/rand unavailable: %v", err))
	}
	return int(v.Int64())
}

// AssignBotNames picks `count` display names for bot players from
// WrestlerNames, skipping any name in exclude (case-insensitive, trimmed —
// intended to hold human player names already claimed in the same game). A
// nil rng defaults to a crypto/rand-backed source.
//
// Names are unique within the returned slice as long as the pool (after
// exclusions) has at least `count` entries. If `count` exceeds the number
// of available unique names, the pool is cycled and a stable, deterministic
// roman-numeral suffix is appended on repeat cycles (e.g. "Sting II") rather
// than failing.
func AssignBotNames(rng NameRNG, count int, exclude []string) []string {
	return AssignBotNamesFromPool(WrestlerNames, rng, count, exclude)
}

// AssignBotNamesFromPool is AssignBotNames with an explicit source pool,
// exposed so tests can exercise the exhaustion/suffix fallback path with a
// small pool without requiring 40+ bots.
func AssignBotNamesFromPool(pool []string, rng NameRNG, count int, exclude []string) []string {
	if count <= 0 {
		return nil
	}
	if rng == nil {
		rng = cryptoNameRNG{}
	}

	excluded := make(map[string]struct{}, len(exclude))
	for _, e := range exclude {
		if e = strings.TrimSpace(e); e != "" {
			excluded[strings.ToLower(e)] = struct{}{}
		}
	}

	available := make([]string, 0, len(pool))
	for _, n := range pool {
		if _, skip := excluded[strings.ToLower(n)]; !skip {
			available = append(available, n)
		}
	}

	// Fisher-Yates shuffle for an unbiased random draw order.
	for i := len(available) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		available[i], available[j] = available[j], available[i]
	}

	out := make([]string, count)
	for i := 0; i < count; i++ {
		base := "Bot"
		if len(available) > 0 {
			base = available[i%len(available)]
		}
		generation := 1
		if len(available) > 0 {
			generation = i/len(available) + 1
		} else {
			generation = i + 1
		}
		if generation <= 1 {
			out[i] = base
		} else {
			out[i] = fmt.Sprintf("%s %s", base, romanNumeral(generation))
		}
	}
	return out
}

// romanNumeral renders a small positive integer (generation counters are
// never expected to exceed a handful) as an uppercase roman numeral.
func romanNumeral(n int) string {
	if n <= 0 {
		return ""
	}
	values := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var sb strings.Builder
	for i, v := range values {
		for n >= v {
			n -= v
			sb.WriteString(symbols[i])
		}
	}
	return sb.String()
}
