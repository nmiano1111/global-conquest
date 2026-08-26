package store

import (
	"context"
	"encoding/json"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// BotTuningConfigStore defines persistence for the singleton (id=1)
// bot_tuning_config row -- the admin-editable source of truth
// GamesService snapshots onto Game.BotTuning at creation time (see
// risk.BotTuning's doc comment).
type BotTuningConfigStore interface {
	Get(ctx context.Context, q db.Querier) (risk.BotTuning, error)
	Update(ctx context.Context, q db.Querier, cfg risk.BotTuning, updatedByUserID string) (risk.BotTuning, error)
}

// PostgresBotTuningConfigStore is the Postgres-backed BotTuningConfigStore.
type PostgresBotTuningConfigStore struct{}

// NewPostgresBotTuningConfigStore constructs a PostgresBotTuningConfigStore.
func NewPostgresBotTuningConfigStore() *PostgresBotTuningConfigStore {
	return &PostgresBotTuningConfigStore{}
}

// Get returns the current tuning config. The row is seeded by migration
// V18 and never deleted, so this never returns pgx.ErrNoRows in practice.
func (s *PostgresBotTuningConfigStore) Get(ctx context.Context, exec db.Querier) (risk.BotTuning, error) {
	const stmt = `
		SELECT kill_advantage_ratio, kill_cluster_max_size, min_attack_win_probability,
		       hogwild_armies_per_territory, pacing
		FROM bot_tuning_config
		WHERE id = 1
	`
	var cfg risk.BotTuning
	var pacingJSON []byte
	if err := exec.QueryRow(ctx, stmt).Scan(
		&cfg.KillAdvantageRatio, &cfg.KillClusterMaxSize, &cfg.MinAttackWinProbability,
		&cfg.HogwildArmiesPerTerritory, &pacingJSON,
	); err != nil {
		return risk.BotTuning{}, err
	}
	if err := json.Unmarshal(pacingJSON, &cfg.Pacing); err != nil {
		return risk.BotTuning{}, err
	}
	return cfg, nil
}

// Update overwrites the singleton row with cfg and returns it as stored.
// updatedByUserID is recorded for audit purposes; an empty value is
// stored as NULL rather than attempting an invalid empty-string uuid cast.
func (s *PostgresBotTuningConfigStore) Update(ctx context.Context, exec db.Querier, cfg risk.BotTuning, updatedByUserID string) (risk.BotTuning, error) {
	pacingJSON, err := json.Marshal(cfg.Pacing)
	if err != nil {
		return risk.BotTuning{}, err
	}
	var updatedBy any
	if updatedByUserID != "" {
		updatedBy = updatedByUserID
	}

	const stmt = `
		UPDATE bot_tuning_config
		SET kill_advantage_ratio = $1,
		    kill_cluster_max_size = $2,
		    min_attack_win_probability = $3,
		    hogwild_armies_per_territory = $4,
		    pacing = $5::jsonb,
		    updated_at = now(),
		    updated_by_user_id = $6::uuid
		WHERE id = 1
		RETURNING kill_advantage_ratio, kill_cluster_max_size, min_attack_win_probability,
		          hogwild_armies_per_territory, pacing
	`
	var out risk.BotTuning
	var outPacingJSON []byte
	if err := exec.QueryRow(ctx, stmt,
		cfg.KillAdvantageRatio, cfg.KillClusterMaxSize, cfg.MinAttackWinProbability,
		cfg.HogwildArmiesPerTerritory, pacingJSON, updatedBy,
	).Scan(
		&out.KillAdvantageRatio, &out.KillClusterMaxSize, &out.MinAttackWinProbability,
		&out.HogwildArmiesPerTerritory, &outPacingJSON,
	); err != nil {
		return risk.BotTuning{}, err
	}
	if err := json.Unmarshal(outPacingJSON, &out.Pacing); err != nil {
		return risk.BotTuning{}, err
	}
	return out, nil
}
