-- is_sandboxed on users: an admin-only flag marking a player fully isolated
-- from everyone else (see internal/service.GamesService's visibility
-- predicate). is_sandboxed on games is a denormalized copy of the creator's
-- flag at the moment the game was created, so a game's isolation status is
-- fixed for its lifetime and every read path (list/join/bootstrap/chat-join)
-- can check it directly off the game row without joining users.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS is_sandboxed boolean NOT NULL DEFAULT false;

ALTER TABLE games
  ADD COLUMN IF NOT EXISTS is_sandboxed boolean NOT NULL DEFAULT false;
