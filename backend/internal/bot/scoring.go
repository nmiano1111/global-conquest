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

// Candidate is one legal option considered at a decision, including its
// full feature breakdown -- the uncapped counterpart to ScoredCommand
// (which only records a score). Only ever populated when the deciding
// ScoredStrategy was built with recordCandidates=true (see
// NewExploringScoredStrategy); nil for every strategy used in real
// gameplay or tournament eval.
type Candidate struct {
	Command  Command
	Features []Feature
	// Chosen is true for the single candidate this decision actually
	// picked -- may not be index 0 among ranked candidates when
	// exploration fired (see Explanation.Explored).
	Chosen bool
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
	// Explored is true when this decision was a random exploration pick
	// (see ScoredStrategy.selectBest / NewExploringScoredStrategy) that
	// differs from what the highest-scoring candidate would have been --
	// false whenever exploration wasn't triggered, or coincidentally
	// landed on the actual best option anyway. Only ever set by a
	// strategy built with NewExploringScoredStrategy, used to generate
	// training data with real action-outcome contrast; never true for
	// real gameplay.
	Explored bool
	// AllCandidates lists every legal candidate considered at this
	// decision (including the chosen one), each with its own full
	// feature breakdown -- unlike Alternatives (capped at
	// maxAlternatives, scores only), this is uncapped. Only populated
	// when the deciding ScoredStrategy was built with
	// recordCandidates=true (see NewExploringScoredStrategy); nil
	// otherwise, including for every strategy used in real gameplay.
	// Lets cmd/traindata build one training row per legal candidate
	// instead of just the chosen one, avoiding the chosen-only selection
	// bias diagnosed in Next_Phase_Bot_ML_Roadmap.md.
	AllCandidates []Candidate
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

// explanationFor builds the (Command, Explanation) pair for choosing
// all[chosenIdx] -- every other entry, in rank order, becomes a candidate
// alternative up to maxAlternatives. Shared by selectBest (chosenIdx
// always 0) and ScoredStrategy.selectBest's exploration path (chosenIdx
// may be any index) so the two never duplicate this logic.
//
// When recordAll is true, every entry in all (not just the top
// maxAlternatives runner-ups) is also copied into Explanation.AllCandidates
// -- no extra feature computation, since all is already fully scored.
func explanationFor(all []ranked, chosenIdx, maxAlternatives int, explored, recordAll bool) (Command, Explanation) {
	chosen := all[chosenIdx]
	alternatives := make([]ScoredCommand, 0, min(maxAlternatives, len(all)-1))
	for i, r := range all {
		if i == chosenIdx {
			continue
		}
		if len(alternatives) >= maxAlternatives {
			break
		}
		alternatives = append(alternatives, ScoredCommand{Command: r.option.Command, Score: r.score})
	}

	var allCandidates []Candidate
	if recordAll {
		allCandidates = make([]Candidate, len(all))
		for i, r := range all {
			allCandidates[i] = Candidate{Command: r.option.Command, Features: r.option.Features, Chosen: i == chosenIdx}
		}
	}

	return chosen.option.Command, Explanation{
		Score:         chosen.score,
		Features:      chosen.option.Features,
		Alternatives:  alternatives,
		Explored:      explored,
		AllCandidates: allCandidates,
	}
}

// selectBest picks the max-scoring option and builds its Explanation,
// including up to maxAlternatives runner-ups (sorted by descending score).
//
// options must be non-empty; every phase that calls this always includes
// at least a "end this phase" sentinel candidate.
func selectBest(options []scoredOption, maxAlternatives int) (Command, Explanation) {
	return explanationFor(rankOptions(options), 0, maxAlternatives, false, false)
}
