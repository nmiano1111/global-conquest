-- is_hidden on games: an admin-only flag excluding the game from ListGames
-- for everyone but admins and its creator (see internal/service.GamesService's
-- gameVisible predicate). Set at creation time, alongside admin-only
-- all-bots games (see GamesService.CreateClassicGame's allBots parameter).
ALTER TABLE games
  ADD COLUMN IF NOT EXISTS is_hidden boolean NOT NULL DEFAULT false;
