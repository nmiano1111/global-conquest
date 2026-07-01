CREATE TABLE discord_outbox (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id          UUID        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    game_sequence    BIGINT,
    notification_type TEXT       NOT NULL,
    deduplication_key TEXT       NOT NULL UNIQUE,
    payload          JSONB       NOT NULL,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    available_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at       TIMESTAMPTZ,
    delivered_at     TIMESTAMPTZ,
    attempt_count    INTEGER     NOT NULL DEFAULT 0,
    last_error       TEXT
);

CREATE INDEX discord_outbox_pending_idx
ON discord_outbox (available_at, created_at)
WHERE delivered_at IS NULL;
