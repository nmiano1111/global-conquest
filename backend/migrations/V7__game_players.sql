CREATE TABLE game_players (
    game_id      uuid        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    user_id      uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    player_index int         NOT NULL,
    won          boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (game_id, user_id)
);

CREATE INDEX game_players_user_id_idx ON game_players(user_id);
