-- event_history_complete tracks whether game_domain_events captures the
-- entire game from the start (needed so reports never overstate coverage).
-- New rows default true: game_domain_events already existed when they were
-- created, so every roll they make is captured. Pre-existing games are
-- backfilled to false since we cannot verify whether earlier rolls (before
-- this event table existed) went uncaptured.
ALTER TABLE games
    ADD COLUMN event_history_complete BOOLEAN NOT NULL DEFAULT true;

UPDATE games SET event_history_complete = false;
