ALTER TABLE games
    ADD COLUMN event_sequence BIGINT NOT NULL DEFAULT 0;

CREATE TABLE game_domain_events (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id         UUID        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    game_sequence   BIGINT      NOT NULL,
    event_type      TEXT        NOT NULL,
    event_version   SMALLINT    NOT NULL,
    actor_player_id UUID        REFERENCES users(id) ON DELETE SET NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload         JSONB       NOT NULL,

    UNIQUE (game_id, game_sequence)
);

CREATE INDEX game_domain_events_event_type_idx ON game_domain_events(event_type);
