-- game_replay_events is an append-only, per-game-ordered log of every
-- committed game action (not just attacks, unlike game_domain_events),
-- storing the same JSON shape already broadcast to clients as
-- game_state_updated. It exists to power player-facing replay: a client
-- can fetch a game's full action history and step through it using the
-- exact same state-application logic the live view already uses.
--
-- game_sequence is its own atomically-incremented counter, sharing the
-- games.event_sequence column with game_domain_events but ticking
-- independently — the two tables' sequence numbers are not expected to
-- line up for the same action, since only game_replay_events writes a row
-- for every action. Nothing joins across them.
CREATE TABLE game_replay_events (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id         uuid        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    game_sequence   bigint      NOT NULL,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    actor_player_id uuid,
    action_type     text        NOT NULL,
    payload         jsonb       NOT NULL,

    UNIQUE (game_id, game_sequence)
);

CREATE INDEX game_replay_events_game_seq_idx ON game_replay_events(game_id, game_sequence);
