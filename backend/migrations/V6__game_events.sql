CREATE TABLE game_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    event_type text NOT NULL,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX game_events_game_created_idx ON game_events(game_id, created_at DESC);
