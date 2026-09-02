-- player_game_stats: a durable, per-player win/loss record for completed
-- human-only games, written once at game-completion time (see
-- GamesService.applyGameAction / SetPlayerGameStatsStore) and never touched
-- again. Unlike game_players, this table has NO foreign key on game_id --
-- deliberately, so that admin-deleting a game (DELETE /api/games/:id, which
-- cascades game_players away via its games(id) FK) does not erase that
-- game's contribution to the leaderboard. player_count is captured at write
-- time from the live risk.Game.Players length so historical stats can be
-- split by player count without needing the game's JSONB state to still
-- exist.
CREATE TABLE player_game_stats (
    game_id      uuid        NOT NULL,
    user_id      uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    won          boolean     NOT NULL,
    player_count smallint    NOT NULL,
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (game_id, user_id)
);

CREATE INDEX player_game_stats_user_id_idx ON player_game_stats(user_id);
