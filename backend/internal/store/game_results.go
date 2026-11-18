package store

import (
	"context"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
)

// GameResultsStore defines persistence for game_results /
// game_result_players (migrations/V22__game_results.sql): the durable,
// admin-facing record of every completed game backing the admin player
// statistics pages.
type GameResultsStore interface {
	// Record writes the result rows for one completed game. It is a no-op
	// if the game is not completed or its rows already exist.
	Record(ctx context.Context, q db.Querier, gameID string) error
	// BackfillCompleted records every completed game that has no
	// game_results row yet, returning how many games were recorded.
	BackfillCompleted(ctx context.Context, q db.Querier) (int, error)
	// ListPlayerResults returns one row per (game, seat) matching filter,
	// ordered by completion time ascending.
	ListPlayerResults(ctx context.Context, q db.Querier, filter GameResultsFilter) ([]PlayerResultRow, error)
}

// Game composition filters for GameResultsFilter.Composition.
const (
	CompositionAll       = "all"
	CompositionHumanOnly = "human_only"
	CompositionMixed     = "mixed"
	CompositionBotOnly   = "bot_only"
)

// Map filters for GameResultsFilter.Map.
const (
	MapFilterAll     = "all"
	MapFilterClassic = "classic"
	MapFilterCustom  = "custom"
)

// GameResultsFilter narrows which completed games ListPlayerResults reads.
// Zero values mean "no restriction" (except IncludeSandboxed, whose zero
// value excludes sandboxed games).
type GameResultsFilter struct {
	// Composition is one of the Composition* constants ("" = all).
	Composition string
	// Map is one of the MapFilter* constants ("" = all).
	Map string
	// PlayerCount restricts to games with exactly this many seats (0 = any).
	PlayerCount int
	// IncludeSandboxed includes games created by sandboxed users.
	IncludeSandboxed bool
	// From/To bound completed_at (inclusive From, exclusive To); nil = open.
	From *time.Time
	To   *time.Time
}

// PlayerResultRow is one seat's outcome in one completed game, joined with
// that game's metadata. Pointer fields are NULL when the game predates the
// event history needed to compute them.
type PlayerResultRow struct {
	GameID      string
	StartedAt   time.Time
	CompletedAt time.Time
	MapName     string // "" = classic board
	PlayerCount int
	HumanCount  int
	BotCount    int
	TurnCount   int
	IsSandboxed bool

	Seat        int
	PlayerKey   string
	UserID      string // "" for bots or deleted users
	IsBot       bool
	BotStrategy string
	DisplayName string
	Won         bool
	Placement   *int

	TurnsTaken      *int
	Attacks         *int
	Conquests       *int
	Eliminations    *int
	CardTrades      *int
	ArmiesDestroyed *int
	ArmiesLost      *int
}

// PostgresGameResultsStore is the Postgres-backed GameResultsStore.
type PostgresGameResultsStore struct{}

// NewPostgresGameResultsStore constructs a PostgresGameResultsStore.
func NewPostgresGameResultsStore() *PostgresGameResultsStore {
	return &PostgresGameResultsStore{}
}

// recordGamesSQL inserts game_results rows for completed games lacking
// one. $1 is a game id, or NULL for every such game (backfill). It returns
// the ids it inserted so recordPlayersSQL can be scoped to exactly those.
const recordGamesSQL = `
	INSERT INTO game_results (
		game_id, started_at, completed_at, map_id, map_name,
		player_count, human_count, bot_count, turn_count, is_sandboxed,
		winner_seat, has_replay_history, has_combat_history
	)
	SELECT
		g.id,
		COALESCE((SELECT min(r.occurred_at) FROM game_replay_events r WHERE r.game_id = g.id), g.created_at),
		g.updated_at,
		g.map_id,
		m.name,
		jsonb_array_length(g.state -> 'players'),
		(SELECT count(*) FROM jsonb_array_elements(g.state -> 'players') p WHERE COALESCE(p ->> 'controller', '') <> 'bot'),
		(SELECT count(*) FROM jsonb_array_elements(g.state -> 'players') p WHERE p ->> 'controller' = 'bot'),
		COALESCE((g.state ->> 'turn_number')::int, 0),
		g.is_sandboxed,
		(SELECT (p.ord - 1)::smallint
		   FROM jsonb_array_elements(g.state -> 'players') WITH ORDINALITY p(val, ord)
		  WHERE p.val ->> 'id' = g.state ->> 'winner'),
		EXISTS (SELECT 1 FROM game_replay_events r WHERE r.game_id = g.id),
		g.event_history_complete
	FROM games g
	LEFT JOIN maps m ON m.id = g.map_id
	WHERE g.status = 'completed'
	  AND COALESCE(g.state ->> 'winner', '') <> ''
	  AND ($1::uuid IS NULL OR g.id = $1::uuid)
	  AND NOT EXISTS (SELECT 1 FROM game_results gr WHERE gr.game_id = g.id)
	ON CONFLICT (game_id) DO NOTHING
	RETURNING game_id::text
`

// recordPlayersSQL inserts one game_result_players row per seat for the
// game ids in $1, aggregating per-player activity from the game's replay
// log (turns, attacks, conquests, eliminations, trades, elimination order)
// and combat log (armies destroyed/lost, as attacker and defender).
const recordPlayersSQL = `
	WITH target AS (
		SELECT g.id, g.state, gr.has_replay_history, gr.has_combat_history
		FROM games g
		JOIN game_results gr ON gr.game_id = g.id
		WHERE g.id = ANY($1::uuid[])
	),
	seats AS (
		SELECT t.id AS game_id, (p.ord - 1)::smallint AS seat, p.val,
		       t.has_replay_history, t.has_combat_history,
		       (p.val ->> 'id') = (t.state ->> 'winner') AS won,
		       -- humans omit "controller" (omitempty), so NULL means human
		       COALESCE(p.val ->> 'controller', '') = 'bot' AS is_bot
		FROM target t, jsonb_array_elements(t.state -> 'players') WITH ORDINALITY p(val, ord)
	),
	actions AS (
		SELECT r.game_id, r.actor_player_id::text AS pid,
		       count(*) FILTER (WHERE r.action_type = 'end_turn')    AS turns_taken,
		       count(*) FILTER (WHERE r.action_type = 'attack')      AS attacks,
		       count(*) FILTER (WHERE r.action_type = 'attack'
		                          AND (r.payload -> 'result' ->> 'conquered')::boolean) AS conquests,
		       count(*) FILTER (WHERE r.action_type = 'attack'
		                          AND COALESCE(r.payload -> 'result' ->> 'eliminated', '') <> '') AS eliminations,
		       count(*) FILTER (WHERE r.action_type = 'trade_cards') AS card_trades
		FROM game_replay_events r
		WHERE r.game_id IN (SELECT id FROM target)
		GROUP BY r.game_id, r.actor_player_id
	),
	elim_order AS (
		SELECT r.game_id, r.payload -> 'result' ->> 'eliminated' AS pid,
		       -- players knocked out AFTER this one; ranking from the end
		       -- stays correct even if early eliminations predate history
		       row_number() OVER (PARTITION BY r.game_id ORDER BY r.game_sequence DESC) - 1 AS later
		FROM game_replay_events r
		WHERE r.game_id IN (SELECT id FROM target)
		  AND r.action_type = 'attack'
		  AND COALESCE(r.payload -> 'result' ->> 'eliminated', '') <> ''
	),
	combat AS (
		SELECT game_id, pid, sum(destroyed) AS destroyed, sum(lost) AS lost
		FROM (
			SELECT e.game_id, e.payload ->> 'attacker_player_id' AS pid,
			       (e.payload ->> 'defender_losses')::int AS destroyed,
			       (e.payload ->> 'attacker_losses')::int AS lost
			FROM game_domain_events e
			WHERE e.game_id IN (SELECT id FROM target) AND e.event_type = 'combat_roll_resolved'
			UNION ALL
			SELECT e.game_id, e.payload ->> 'defender_player_id',
			       (e.payload ->> 'attacker_losses')::int,
			       (e.payload ->> 'defender_losses')::int
			FROM game_domain_events e
			WHERE e.game_id IN (SELECT id FROM target) AND e.event_type = 'combat_roll_resolved'
		) x
		GROUP BY game_id, pid
	)
	INSERT INTO game_result_players (
		game_id, seat, player_key, user_id, is_bot, bot_strategy, display_name, won, placement,
		turns_taken, attacks, conquests, eliminations, card_trades, armies_destroyed, armies_lost
	)
	SELECT
		s.game_id,
		s.seat,
		CASE WHEN s.is_bot
		     THEN 'bot:' || COALESCE(NULLIF(s.val ->> 'strategy', ''), 'unknown')
		     ELSE s.val ->> 'id' END,
		CASE WHEN s.is_bot THEN NULL ELSE u.id END,
		s.is_bot,
		CASE WHEN s.is_bot THEN COALESCE(NULLIF(s.val ->> 'strategy', ''), 'unknown') END,
		COALESCE(NULLIF(s.val ->> 'name', ''), u.username, 'unknown'),
		s.won,
		CASE WHEN s.won THEN 1
		     WHEN eo.later IS NOT NULL THEN (eo.later + 2)::smallint
		END,
		CASE WHEN s.has_replay_history THEN COALESCE(a.turns_taken, 0) END,
		CASE WHEN s.has_replay_history THEN COALESCE(a.attacks, 0) END,
		CASE WHEN s.has_replay_history THEN COALESCE(a.conquests, 0) END,
		CASE WHEN s.has_replay_history THEN COALESCE(a.eliminations, 0) END,
		CASE WHEN s.has_replay_history THEN COALESCE(a.card_trades, 0) END,
		CASE WHEN s.has_combat_history THEN COALESCE(c.destroyed, 0) END,
		CASE WHEN s.has_combat_history THEN COALESCE(c.lost, 0) END
	FROM seats s
	LEFT JOIN users u ON u.id::text = s.val ->> 'id'
	LEFT JOIN actions a ON a.game_id = s.game_id AND a.pid = s.val ->> 'id'
	LEFT JOIN elim_order eo ON eo.game_id = s.game_id AND eo.pid = s.val ->> 'id'
	LEFT JOIN combat c ON c.game_id = s.game_id AND c.pid = s.val ->> 'id'
	ON CONFLICT (game_id, seat) DO NOTHING
`

func (s *PostgresGameResultsStore) record(ctx context.Context, exec db.Querier, gameID *string) (int, error) {
	rows, err := exec.Query(ctx, recordGamesSQL, gameID)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	prow, err := exec.Query(ctx, recordPlayersSQL, ids)
	if err != nil {
		return 0, err
	}
	prow.Close()
	return len(ids), prow.Err()
}

// Record writes the result rows for one completed game; see
// GameResultsStore.Record. It must run after the game's final action's
// replay and combat events are inserted (same transaction), so the
// winning attack's elimination is counted.
func (s *PostgresGameResultsStore) Record(ctx context.Context, exec db.Querier, gameID string) error {
	_, err := s.record(ctx, exec, &gameID)
	return err
}

// BackfillCompleted records every completed game lacking result rows.
func (s *PostgresGameResultsStore) BackfillCompleted(ctx context.Context, exec db.Querier) (int, error) {
	return s.record(ctx, exec, nil)
}

const listPlayerResultsSQL = `
	SELECT
		gr.game_id::text, gr.started_at, gr.completed_at, COALESCE(gr.map_name, ''),
		gr.player_count, gr.human_count, gr.bot_count, gr.turn_count, gr.is_sandboxed,
		p.seat, p.player_key, COALESCE(p.user_id::text, ''), p.is_bot, COALESCE(p.bot_strategy, ''),
		COALESCE(u.username, p.display_name), p.won, p.placement,
		p.turns_taken, p.attacks, p.conquests, p.eliminations, p.card_trades,
		p.armies_destroyed, p.armies_lost
	FROM game_results gr
	JOIN game_result_players p ON p.game_id = gr.game_id
	LEFT JOIN users u ON u.id = p.user_id
	WHERE ($1 = 'all'
	       OR ($1 = 'human_only' AND gr.bot_count = 0)
	       OR ($1 = 'mixed'      AND gr.bot_count > 0 AND gr.human_count > 0)
	       OR ($1 = 'bot_only'   AND gr.human_count = 0))
	  AND ($2 = 'all'
	       OR ($2 = 'classic' AND gr.map_id IS NULL)
	       OR ($2 = 'custom'  AND gr.map_id IS NOT NULL))
	  AND ($3::int = 0 OR gr.player_count = $3::int)
	  AND ($4::boolean OR NOT gr.is_sandboxed)
	  AND ($5::timestamptz IS NULL OR gr.completed_at >= $5::timestamptz)
	  AND ($6::timestamptz IS NULL OR gr.completed_at <  $6::timestamptz)
	ORDER BY gr.completed_at, gr.game_id, p.seat
`

// ListPlayerResults returns every (game, seat) row matching filter, oldest
// game first.
func (s *PostgresGameResultsStore) ListPlayerResults(ctx context.Context, exec db.Querier, f GameResultsFilter) ([]PlayerResultRow, error) {
	composition := f.Composition
	if composition == "" {
		composition = CompositionAll
	}
	mapFilter := f.Map
	if mapFilter == "" {
		mapFilter = MapFilterAll
	}
	rows, err := exec.Query(ctx, listPlayerResultsSQL, composition, mapFilter, f.PlayerCount, f.IncludeSandboxed, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayerResultRow
	for rows.Next() {
		var r PlayerResultRow
		var placement *int16
		if err := rows.Scan(
			&r.GameID, &r.StartedAt, &r.CompletedAt, &r.MapName,
			&r.PlayerCount, &r.HumanCount, &r.BotCount, &r.TurnCount, &r.IsSandboxed,
			&r.Seat, &r.PlayerKey, &r.UserID, &r.IsBot, &r.BotStrategy,
			&r.DisplayName, &r.Won, &placement,
			&r.TurnsTaken, &r.Attacks, &r.Conquests, &r.Eliminations, &r.CardTrades,
			&r.ArmiesDestroyed, &r.ArmiesLost,
		); err != nil {
			return nil, err
		}
		if placement != nil {
			v := int(*placement)
			r.Placement = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
