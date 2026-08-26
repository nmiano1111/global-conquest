-- bot_tuning_config: a singleton (id=1) row holding the admin-tunable
-- killbot-v2 AI parameters (see internal/risk.BotTuning and
-- internal/service.BotTuningConfig). Snapshotted onto a new game's
-- risk.Game.BotTuning field at creation time -- fixed for that game's
-- lifetime regardless of later admin edits here, the same
-- creation-time-snapshot pattern games.is_sandboxed/is_hidden already use.
-- Defaults match DefaultBotTuning() (internal/bot/tuning.go) -- what
-- killbot-v2 already does today, out of the box.
CREATE TABLE IF NOT EXISTS bot_tuning_config (
  id integer PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  kill_advantage_ratio double precision NOT NULL DEFAULT 1.5,
  kill_cluster_max_size integer NOT NULL DEFAULT 20,
  min_attack_win_probability double precision NOT NULL DEFAULT 0.5,
  hogwild_armies_per_territory integer NOT NULL DEFAULT 36,
  pacing jsonb NOT NULL DEFAULT '{
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
  }'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO bot_tuning_config (id) VALUES (1)
  ON CONFLICT (id) DO NOTHING;
