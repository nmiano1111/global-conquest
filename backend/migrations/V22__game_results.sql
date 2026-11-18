-- game_results / game_result_players: a durable, admin-facing record of
-- EVERY completed game (human-only, mixed human/bot, all-bot; classic and
-- custom maps), powering the admin player-statistics pages. Unlike
-- player_game_stats (V19), which backs the public leaderboard and
-- deliberately records only human-only classic games, nothing is filtered
-- out at write time here -- the stats API filters at read time instead.
--
-- Like player_game_stats, neither table has an FK to games (or maps), so an
-- admin deleting a game does not erase its contribution to the stats. Rows
-- are written once, inside the transaction of a game's final action (see
-- store.PostgresGameResultsStore.Record), and pre-existing completed games
-- are backfilled idempotently at backend startup (BackfillCompleted)
-- rather than in this file, so the aggregation SQL lives in one place.
CREATE TABLE game_results (
    game_id       uuid        PRIMARY KEY,
    -- started_at is the first recorded action's time (game_replay_events),
    -- falling back to the game's creation time for games that predate
    -- replay events. completed_at is the final action's commit time.
    started_at    timestamptz NOT NULL,
    completed_at  timestamptz NOT NULL,
    map_id        uuid        NULL,
    map_name      text        NULL,
    player_count  smallint    NOT NULL,
    human_count   smallint    NOT NULL,
    bot_count     smallint    NOT NULL,
    turn_count    int         NOT NULL,
    is_sandboxed  boolean     NOT NULL DEFAULT false,
    winner_seat   smallint    NULL,
    -- has_replay_history / has_combat_history report whether the per-player
    -- action and combat columns below are trustworthy for this game; they
    -- are NULL on game_result_players when false.
    has_replay_history boolean NOT NULL,
    has_combat_history boolean NOT NULL
);

CREATE INDEX game_results_completed_at_idx ON game_results(completed_at);

CREATE TABLE game_result_players (
    game_id        uuid     NOT NULL REFERENCES game_results(game_id) ON DELETE CASCADE,
    seat           smallint NOT NULL,
    -- player_key identifies the stats subject: a human's user id, or
    -- 'bot:<strategy>' so every bot running the same strategy aggregates
    -- together (bot display names are cosmetic, per-game wrestler names).
    player_key     text     NOT NULL,
    user_id        uuid     NULL REFERENCES users(id) ON DELETE SET NULL,
    is_bot         boolean  NOT NULL,
    bot_strategy   text     NULL,
    -- display_name snapshots the human's username / bot's name at write
    -- time; reads prefer the live username when the user still exists.
    display_name   text     NOT NULL,
    won            boolean  NOT NULL,
    -- placement: 1 = winner, otherwise ranked by reverse elimination order
    -- (last player knocked out = 2nd). NULL when elimination order is
    -- unknown (no replay history).
    placement      smallint NULL,
    turns_taken        int  NULL,
    attacks            int  NULL,
    conquests          int  NULL,
    eliminations       int  NULL,
    card_trades        int  NULL,
    armies_destroyed   int  NULL,
    armies_lost        int  NULL,
    PRIMARY KEY (game_id, seat)
);

CREATE INDEX game_result_players_player_key_idx ON game_result_players(player_key);
