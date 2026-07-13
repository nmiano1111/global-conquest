package bot

import "testing"

func TestSelectBestPicksMaxScore(t *testing.T) {
	options := []scoredOption{
		{Command: Command{Action: ActionEndAttack}, Features: []Feature{{"end_phase_bias", 0}}},
		{Command: Command{Action: ActionAttack, To: "Alaska"}, Features: []Feature{{"army_advantage", 3}}},
		{Command: Command{Action: ActionAttack, To: "Peru"}, Features: []Feature{{"army_advantage", 7}}},
	}
	cmd, expl := selectBest(options, 2)
	if cmd.Action != ActionAttack || cmd.To != "Peru" {
		t.Fatalf("expected the Peru attack (highest score), got %+v", cmd)
	}
	if expl.Score != 7 {
		t.Fatalf("expected explanation score 7, got %v", expl.Score)
	}
	if len(expl.Features) != 1 || expl.Features[0].Name != "army_advantage" {
		t.Fatalf("expected the winning features to be carried into the explanation, got %+v", expl.Features)
	}
}

func TestSelectBestBreaksTiesByInputOrder(t *testing.T) {
	options := []scoredOption{
		{Command: Command{Action: ActionAttack, To: "First"}, Features: []Feature{{"x", 5}}},
		{Command: Command{Action: ActionAttack, To: "Second"}, Features: []Feature{{"x", 5}}},
	}
	cmd, _ := selectBest(options, 0)
	if cmd.To != "First" {
		t.Fatalf("expected the first-listed option to win a tie, got %+v", cmd)
	}
}

func TestSelectBestCapsAndOrdersAlternatives(t *testing.T) {
	options := []scoredOption{
		{Command: Command{Action: ActionAttack, To: "Best"}, Features: []Feature{{"x", 10}}},
		{Command: Command{Action: ActionAttack, To: "Second"}, Features: []Feature{{"x", 8}}},
		{Command: Command{Action: ActionAttack, To: "Third"}, Features: []Feature{{"x", 6}}},
		{Command: Command{Action: ActionAttack, To: "Fourth"}, Features: []Feature{{"x", 4}}},
	}
	_, expl := selectBest(options, 2)
	if len(expl.Alternatives) != 2 {
		t.Fatalf("expected alternatives capped at 2, got %d", len(expl.Alternatives))
	}
	if expl.Alternatives[0].Command.To != "Second" || expl.Alternatives[1].Command.To != "Third" {
		t.Fatalf("expected alternatives in descending score order, got %+v", expl.Alternatives)
	}
}

func TestExplanationStringRendersFeatures(t *testing.T) {
	expl := Explanation{Score: 6.8, Features: []Feature{{"army_advantage", 8}, {"exposure_penalty", -1.2}}}
	s := expl.String()
	if s != "score=6.8 army_advantage=+8.0 exposure_penalty=-1.2" {
		t.Fatalf("unexpected explanation string: %q", s)
	}
}

func TestExplanationStringEmptyFeatures(t *testing.T) {
	if s := (Explanation{}).String(); s != "score=0.0" {
		t.Fatalf("expected bare score for an empty explanation, got %q", s)
	}
}
