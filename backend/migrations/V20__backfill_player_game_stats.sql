-- Backfill player_game_stats from every completed, human-only game that
-- still exists at migration time (games already deleted before this
-- migration ran cannot be recovered -- their game_players rows were
-- cascade-deleted along with them). Mirrors GetLeaderboard's existing
-- bot-exclusion filter (internal/store/game_players.go) and derives
-- player_count from the same JSONB players array that filter inspects.
-- g.updated_at is used as completed_at: UpdateState sets status='completed'
-- in the same write as the game's final action, so it's the closest
-- existing timestamp to actual completion.
INSERT INTO player_game_stats (game_id, user_id, won, player_count, completed_at)
SELECT
    gp.game_id,
    gp.user_id,
    gp.won,
    jsonb_array_length(g.state -> 'players'),
    g.updated_at
FROM game_players gp
JOIN games g ON g.id = gp.game_id
WHERE g.status = 'completed'
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements(g.state -> 'players') AS p
      WHERE p ->> 'controller' = 'bot'
  )
ON CONFLICT (game_id, user_id) DO NOTHING;
