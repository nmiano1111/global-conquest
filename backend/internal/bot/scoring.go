package bot

import (
	"fmt"
	"sort"
	"strings"
)

// Feature is one named, weighted contribution to a candidate's total score
// — e.g. {"army_advantage", 8.0}. Summing a candidate's Features equals its
// score. Naming every contribution (rather than folding straight into a
// single float) is what makes a decision explainable after the fact.
type Feature struct {
	// Name identifies the contribution (e.g. "army_advantage").
	Name string
	// Value is the weighted contribution to the candidate's total score.
	Value float64
}

// ScoredCommand pairs a legal command with the score it received, used to
// record runner-up candidates in an Explanation.
type ScoredCommand struct {
	// Command is the candidate command that was scored.
	Command Command
	// Score is the total score the command received.
	Score float64
}

// Explanation records why a strategy chose the command it did: the winning
// score broken into its named contributions, plus the top runner-up
// candidates. A strategy with no scoring model (basic-v1) returns a
// zero-value Explanation — Score 0, no Features, no Alternatives.
type Explanation struct {
	// Score is the winning command's total score.
	Score float64
	// Features breaks the winning score down into its named contributions.
	Features []Feature
	// Alternatives lists the top runner-up candidates that were not chosen.
	Alternatives []ScoredCommand
}

// String renders a compact single-line form suitable for a log line, e.g.
// "score=6.8 army_advantage=+8.0 capture_probability=+2.1 end_phase_bias=+0.0".
func (e Explanation) String() string {
	if len(e.Features) == 0 {
		return fmt.Sprintf("score=%.1f", e.Score)
	}
	parts := make([]string, 0, len(e.Features)+1)
	parts = append(parts, fmt.Sprintf("score=%.1f", e.Score))
	for _, f := range e.Features {
		parts = append(parts, fmt.Sprintf("%s=%+.1f", f.Name, f.Value))
	}
	return strings.Join(parts, " ")
}

// scoredOption pairs one legal command — or a phase's "end this phase"
// sentinel command — with the features computed for it. Every phase that
// adopts the candidate-scoring pipeline builds a []scoredOption (including
// the "end" sentinel as just another entry, never special-cased) and calls
// selectBest.
type scoredOption struct {
	Command  Command
	Features []Feature
}

func (o scoredOption) score() float64 {
	var total float64
	for _, f := range o.Features {
		total += f.Value
	}
	return total
}

// ranked pairs one scoredOption with its computed score, in the sorted
// order rankOptions produces.
type ranked struct {
	option scoredOption
	score  float64
}

// rankOptions scores every option and sorts best-first. Ties break by
// input order, matching how every existing legal-action helper
// (risk.LegalAttacks, LegalReinforcements, etc.) already emits
// board-canonical order — callers that want a specific tie-break should
// order their candidate slice accordingly before calling this.
func rankOptions(options []scoredOption) []ranked {
	all := make([]ranked, len(options))
	for i, o := range options {
		all[i] = ranked{option: o, score: o.score()}
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].score > all[j].score
	})
	return all
}

// explanationFor builds the (Command, Explanation) pair for choosing the
// highest-scoring entry in all (index 0, since rankOptions already sorted
// best-first) -- every other entry, in rank order, becomes a candidate
// alternative up to maxAlternatives.
func explanationFor(all []ranked, maxAlternatives int) (Command, Explanation) {
	chosen := all[0]
	alternatives := make([]ScoredCommand, 0, min(maxAlternatives, len(all)-1))
	for _, r := range all[1:] {
		if len(alternatives) >= maxAlternatives {
			break
		}
		alternatives = append(alternatives, ScoredCommand{Command: r.option.Command, Score: r.score})
	}

	return chosen.option.Command, Explanation{
		Score:        chosen.score,
		Features:     chosen.option.Features,
		Alternatives: alternatives,
	}
}

// selectBest picks the max-scoring option and builds its Explanation,
// including up to maxAlternatives runner-ups (sorted by descending score).
//
// options must be non-empty; every phase that calls this always includes
// at least a "end this phase" sentinel candidate.
func selectBest(options []scoredOption, maxAlternatives int) (Command, Explanation) {
	return explanationFor(rankOptions(options), maxAlternatives)
}
