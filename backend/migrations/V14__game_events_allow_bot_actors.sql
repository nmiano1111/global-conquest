-- Bot players are not rows in `users` (no account, no session — see
-- risk.PlayerState.Controller), but every game action, bot or human,
-- writes a game_events row via GamesService.ApplyGameAction. The FK below
-- rejected any event whose actor was a bot, which surfaced as every bot
-- command failing and being retried until the runner gave up.
--
-- No code joins game_events.actor_user_id against users; it is only
-- ever stored/displayed as an opaque id, so dropping the FK is safe.
ALTER TABLE game_events DROP CONSTRAINT game_events_actor_user_id_fkey;

-- Same problem, same fix: game_domain_events.actor_player_id is set from
-- risk.DomainEvent.ActorPlayerID (e.g. combat_roll_resolved), which is
-- whichever player attacked — bot or human. No code joins this column
-- against users either.
ALTER TABLE game_domain_events DROP CONSTRAINT game_domain_events_actor_player_id_fkey;
