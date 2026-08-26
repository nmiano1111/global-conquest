package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

const testPacingJSON = `{
	"turn_start_min_ms": 1200, "turn_start_max_ms": 2200,
	"card_turn_in_min_ms": 1500, "card_turn_in_max_ms": 2700,
	"reinforcement_min_ms": 600, "reinforcement_max_ms": 1200,
	"first_attack_min_ms": 1000, "first_attack_max_ms": 1800,
	"repeat_attack_min_ms": 500, "repeat_attack_max_ms": 1000,
	"capture_min_ms": 1500, "capture_max_ms": 2700,
	"occupation_min_ms": 750, "occupation_max_ms": 1500,
	"fortify_min_ms": 1000, "fortify_max_ms": 2000,
	"end_attack_min_ms": 450, "end_attack_max_ms": 900,
	"dramatic_min_ms": 2200, "dramatic_max_ms": 3500
}`

func TestPostgresBotTuningConfigStoreGet(t *testing.T) {
	q := &stubQuerier{
		row: &stubRow{values: []any{1.5, 20, 0.5, 36, []byte(testPacingJSON)}},
	}
	s := NewPostgresBotTuningConfigStore()

	cfg, err := s.Get(context.Background(), q)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM bot_tuning_config") {
		t.Fatalf("expected bot_tuning_config select SQL, got %q", q.lastSQL)
	}
	if cfg.KillAdvantageRatio != 1.5 || cfg.KillClusterMaxSize != 20 || cfg.MinAttackWinProbability != 0.5 || cfg.HogwildArmiesPerTerritory != 36 {
		t.Fatalf("unexpected scalar fields: %+v", cfg)
	}
	if cfg.Pacing.TurnStartMinMs != 1200 || cfg.Pacing.DramaticMaxMs != 3500 {
		t.Fatalf("unexpected pacing decode: %+v", cfg.Pacing)
	}
}

func TestPostgresBotTuningConfigStoreUpdate(t *testing.T) {
	q := &stubQuerier{
		row: &stubRow{values: []any{1.2, 15, 0.6, 40, []byte(testPacingJSON)}},
	}
	s := NewPostgresBotTuningConfigStore()

	cfg := risk.BotTuning{
		KillAdvantageRatio:        1.2,
		KillClusterMaxSize:        15,
		MinAttackWinProbability:   0.6,
		HogwildArmiesPerTerritory: 40,
	}
	if err := json.Unmarshal([]byte(testPacingJSON), &cfg.Pacing); err != nil {
		t.Fatalf("unmarshal fixture pacing: %v", err)
	}

	out, err := s.Update(context.Background(), q, cfg, "admin-1")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(q.lastSQL, "UPDATE bot_tuning_config") {
		t.Fatalf("expected bot_tuning_config update SQL, got %q", q.lastSQL)
	}
	if len(q.lastArgs) != 6 || q.lastArgs[5] != "admin-1" {
		t.Fatalf("expected updated_by_user_id arg to be admin-1, got %v", q.lastArgs)
	}
	if out.KillAdvantageRatio != 1.2 || out.KillClusterMaxSize != 15 {
		t.Fatalf("unexpected returned config: %+v", out)
	}
}

func TestPostgresBotTuningConfigStoreUpdateEmptyUpdatedByIsNil(t *testing.T) {
	q := &stubQuerier{
		row: &stubRow{values: []any{1.5, 20, 0.5, 36, []byte(testPacingJSON)}},
	}
	s := NewPostgresBotTuningConfigStore()

	var cfg risk.BotTuning
	if err := json.Unmarshal([]byte(testPacingJSON), &cfg.Pacing); err != nil {
		t.Fatalf("unmarshal fixture pacing: %v", err)
	}
	if _, err := s.Update(context.Background(), q, cfg, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(q.lastArgs) != 6 || q.lastArgs[5] != nil {
		t.Fatalf("expected a nil updated_by_user_id arg for an empty actor, got %v", q.lastArgs)
	}
}
