-- attack_decisions: one row per attack-phase decision a player made -- either
-- an attack roll (kind 'attack') or ending the attack phase (kind 'stop') --
-- reconstructed from game_replay_events by service.DecisionExtractor. Each
-- row also summarizes the legal attacks that were available at that moment
-- (risk.LegalAttacks on the pre-decision board), so stats can compare what a
-- player chose against what they could have chosen.
--
-- Like game_results, these rows outlive the game: they hang off game_results
-- (not games), so admin-deleting a game keeps its decision history.
CREATE TABLE attack_decisions (
    game_id                     uuid     NOT NULL REFERENCES game_results(game_id) ON DELETE CASCADE,
    game_sequence               bigint   NOT NULL,
    seat                        smallint NOT NULL,
    player_key                  text     NOT NULL,
    -- Zero-based count of turns ended before this decision, game-wide.
    turn_index                  int      NOT NULL,
    kind                        text     NOT NULL CHECK (kind IN ('attack', 'stop')),

    -- The chosen attack (NULL for kind 'stop').
    from_territory              text,
    to_territory                text,
    defender_seat               smallint,
    attacker_armies             int,
    defender_armies             int,
    -- Odds of taking the target fighting to a conclusion with max dice.
    win_prob                    real,
    -- Another roll of the same attack this player was already making.
    continued                   boolean,
    attacker_dice               smallint,
    defender_dice               smallint,
    attacker_loss               smallint,
    defender_loss               smallint,
    -- Expected losses for this roll's dice counts (for dice-luck stats).
    expected_attacker_loss      real,
    expected_defender_loss      real,
    captured                    boolean,
    completes_continent         boolean,
    breaks_continent            boolean,
    targets_leader              boolean,
    targets_weakest             boolean,

    -- Every legal attack at this decision point.
    options                     int      NOT NULL,
    best_win_prob               real,
    -- Option counts per win_prob bucket: <25%, 25-50%, 50-75%, 75-90%, >=90%.
    options_by_bucket           int[]    NOT NULL,
    options_completes_continent int      NOT NULL,
    options_breaks_continent    int      NOT NULL,
    options_vs_leader           int      NOT NULL,
    options_vs_weakest          int      NOT NULL,

    PRIMARY KEY (game_id, game_sequence)
);

CREATE INDEX attack_decisions_player_key_idx ON attack_decisions(player_key);

-- attack_decision_games marks which completed games the extractor has
-- processed, and with which extractor version (a newer version re-extracts).
-- available is false when the game's replay history or board was already
-- gone, so nothing could be extracted.
CREATE TABLE attack_decision_games (
    game_id      uuid        PRIMARY KEY REFERENCES game_results(game_id) ON DELETE CASCADE,
    version      int         NOT NULL,
    available    boolean     NOT NULL,
    decisions    int         NOT NULL,
    extracted_at timestamptz NOT NULL DEFAULT now()
);
