package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
)

// AttackDecision is one attack_decisions row (see migration V23). Pointer
// fields are NULL for kind "stop". JSON tags match the column names so rows
// can be bulk-inserted via jsonb_to_recordset.
type AttackDecision struct {
	GameSequence int64  `json:"game_sequence"`
	Seat         int    `json:"seat"`
	PlayerKey    string `json:"player_key"`
	TurnIndex    int    `json:"turn_index"`
	Kind         string `json:"kind"`

	FromTerritory        *string  `json:"from_territory"`
	ToTerritory          *string  `json:"to_territory"`
	DefenderSeat         *int     `json:"defender_seat"`
	AttackerArmies       *int     `json:"attacker_armies"`
	DefenderArmies       *int     `json:"defender_armies"`
	WinProb              *float64 `json:"win_prob"`
	Continued            *bool    `json:"continued"`
	AttackerDice         *int     `json:"attacker_dice"`
	DefenderDice         *int     `json:"defender_dice"`
	AttackerLoss         *int     `json:"attacker_loss"`
	DefenderLoss         *int     `json:"defender_loss"`
	ExpectedAttackerLoss *float64 `json:"expected_attacker_loss"`
	ExpectedDefenderLoss *float64 `json:"expected_defender_loss"`
	Captured             *bool    `json:"captured"`
	CompletesContinent   *bool    `json:"completes_continent"`
	BreaksContinent      *bool    `json:"breaks_continent"`
	TargetsLeader        *bool    `json:"targets_leader"`
	TargetsWeakest       *bool    `json:"targets_weakest"`

	Options                   int      `json:"options"`
	BestWinProb               *float64 `json:"best_win_prob"`
	OptionsByBucket           []int    `json:"options_by_bucket"`
	OptionsCompletesContinent int      `json:"options_completes_continent"`
	OptionsBreaksContinent    int      `json:"options_breaks_continent"`
	OptionsVsLeader           int      `json:"options_vs_leader"`
	OptionsVsWeakest          int      `json:"options_vs_weakest"`

	// Per distinct attackable territory (V24); nil on version-1 rows.
	ToContinent             *string   `json:"to_continent"`
	OptionsTargets          []string  `json:"options_targets"`
	OptionsTargetContinents []string  `json:"options_target_continents"`
	OptionsWinProbs         []float64 `json:"options_win_probs"`
}

// AttackDecisionRow is an AttackDecision plus its game's id and completion
// time, as read back for stats.
type AttackDecisionRow struct {
	AttackDecision
	GameID      string
	CompletedAt time.Time
	// MapID and MapName identify the game's custom map; both "" for the
	// classic map.
	MapID   string
	MapName string
}

// DecisionSource is what the extractor needs to process one game.
type DecisionSource struct {
	// Board is games.state->'board'; empty when the game row is gone or
	// predates boards being stored in state.
	Board json.RawMessage
	// GameExists is false when the games row was deleted.
	GameExists bool
	// PlayerKeys maps seat to game_result_players.player_key.
	PlayerKeys map[int]string
}

// PostgresAttackDecisionsStore persists extracted attack decisions.
type PostgresAttackDecisionsStore struct{}

// NewPostgresAttackDecisionsStore constructs a PostgresAttackDecisionsStore.
func NewPostgresAttackDecisionsStore() *PostgresAttackDecisionsStore {
	return &PostgresAttackDecisionsStore{}
}

// PendingGames returns up to limit recorded games with replay history that
// have not been extracted at version (newest first).
func (s *PostgresAttackDecisionsStore) PendingGames(ctx context.Context, q db.Querier, version, limit int) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT gr.game_id::text
		FROM game_results gr
		LEFT JOIN attack_decision_games d ON d.game_id = gr.game_id
		WHERE gr.has_replay_history
		  AND (d.game_id IS NULL OR d.version < $1)
		ORDER BY gr.completed_at DESC
		LIMIT $2`, version, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LoadSource reads a game's board and seat-to-player-key mapping.
func (s *PostgresAttackDecisionsStore) LoadSource(ctx context.Context, q db.Querier, gameID string) (DecisionSource, error) {
	src := DecisionSource{PlayerKeys: map[int]string{}}
	var board []byte
	err := q.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM games WHERE id = $1::uuid),
		       (SELECT state -> 'board' FROM games WHERE id = $1::uuid)`, gameID).Scan(&src.GameExists, &board)
	if err != nil {
		return src, err
	}
	src.Board = board
	rows, err := q.Query(ctx, `SELECT seat, player_key FROM game_result_players WHERE game_id = $1::uuid`, gameID)
	if err != nil {
		return src, err
	}
	defer rows.Close()
	for rows.Next() {
		var seat int16
		var key string
		if err := rows.Scan(&seat, &key); err != nil {
			return src, err
		}
		src.PlayerKeys[int(seat)] = key
	}
	return src, rows.Err()
}

// Replace swaps in a game's extracted decisions and marks it processed at
// version. When available is false the game's existing rows (from an older
// extraction, made while its history still existed) are kept.
func (s *PostgresAttackDecisionsStore) Replace(ctx context.Context, q db.Querier, gameID string, version int, available bool, decisions []AttackDecision) error {
	if available {
		if err := execQ(ctx, q, `DELETE FROM attack_decisions WHERE game_id = $1::uuid`, gameID); err != nil {
			return err
		}
		if len(decisions) > 0 {
			payload, err := json.Marshal(decisions)
			if err != nil {
				return err
			}
			if err := execQ(ctx, q, insertAttackDecisionsSQL, gameID, payload); err != nil {
				return err
			}
		}
	}
	return execQ(ctx, q, `
		INSERT INTO attack_decision_games (game_id, version, available, decisions)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (game_id) DO UPDATE
		SET version = EXCLUDED.version, available = EXCLUDED.available,
		    decisions = CASE WHEN EXCLUDED.available THEN EXCLUDED.decisions ELSE attack_decision_games.decisions END,
		    extracted_at = now()`, gameID, version, available, len(decisions))
}

// execQ runs a statement through db.Querier, which only exposes Query.
func execQ(ctx context.Context, q db.Querier, stmt string, args ...any) error {
	rows, err := q.Query(ctx, stmt, args...)
	if err != nil {
		return err
	}
	rows.Close()
	return rows.Err()
}

const attackDecisionColumns = `game_sequence, seat, player_key, turn_index, kind,
	from_territory, to_territory, defender_seat, attacker_armies, defender_armies, win_prob, continued,
	attacker_dice, defender_dice, attacker_loss, defender_loss, expected_attacker_loss, expected_defender_loss,
	captured, completes_continent, breaks_continent, targets_leader, targets_weakest,
	options, best_win_prob, options_by_bucket, options_completes_continent, options_breaks_continent,
	options_vs_leader, options_vs_weakest,
	to_continent, options_targets, options_target_continents, options_win_probs`

const insertAttackDecisionsSQL = `
	INSERT INTO attack_decisions (game_id, ` + attackDecisionColumns + `)
	SELECT $1::uuid, ` + attackDecisionColumns + `
	FROM jsonb_to_recordset($2::jsonb) AS r(
		game_sequence bigint, seat smallint, player_key text, turn_index int, kind text,
		from_territory text, to_territory text, defender_seat smallint, attacker_armies int, defender_armies int,
		win_prob real, continued boolean,
		attacker_dice smallint, defender_dice smallint, attacker_loss smallint, defender_loss smallint,
		expected_attacker_loss real, expected_defender_loss real,
		captured boolean, completes_continent boolean, breaks_continent boolean, targets_leader boolean, targets_weakest boolean,
		options int, best_win_prob real, options_by_bucket int[], options_completes_continent int, options_breaks_continent int,
		options_vs_leader int, options_vs_weakest int,
		to_continent text, options_targets text[], options_target_continents text[], options_win_probs real[])
`

// ListForPlayer returns a player's decisions in games matching f, in game
// order (oldest game first, then sequence).
func (s *PostgresAttackDecisionsStore) ListForPlayer(ctx context.Context, q db.Querier, playerKey string, f GameResultsFilter) ([]AttackDecisionRow, error) {
	composition := f.Composition
	if composition == "" {
		composition = CompositionAll
	}
	mapFilter := f.Map
	if mapFilter == "" {
		mapFilter = MapFilterAll
	}
	rows, err := q.Query(ctx, `
		SELECT gr.game_id::text, gr.completed_at, COALESCE(gr.map_id::text, ''), COALESCE(gr.map_name, ''), `+attackDecisionColumns+`
		FROM attack_decisions d
		JOIN game_results gr ON gr.game_id = d.game_id
		WHERE d.player_key = $7
		  AND `+gameResultsFilterSQL+`
		ORDER BY gr.completed_at, gr.game_id, d.game_sequence`,
		composition, mapFilter, f.PlayerCount, f.IncludeSandboxed, f.From, f.To, playerKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttackDecisionRow
	for rows.Next() {
		var r AttackDecisionRow
		var (
			seat                                int16
			defSeat, aDice, dDice, aLoss, dLoss *int16
			winProb, expA, expD, best           *float32
		)
		var buckets []int32
		var targetProbs []float32
		if err := rows.Scan(&r.GameID, &r.CompletedAt, &r.MapID, &r.MapName,
			&r.GameSequence, &seat, &r.PlayerKey, &r.TurnIndex, &r.Kind,
			&r.FromTerritory, &r.ToTerritory, &defSeat, &r.AttackerArmies, &r.DefenderArmies, &winProb, &r.Continued,
			&aDice, &dDice, &aLoss, &dLoss, &expA, &expD,
			&r.Captured, &r.CompletesContinent, &r.BreaksContinent, &r.TargetsLeader, &r.TargetsWeakest,
			&r.Options, &best, &buckets, &r.OptionsCompletesContinent, &r.OptionsBreaksContinent,
			&r.OptionsVsLeader, &r.OptionsVsWeakest,
			&r.ToContinent, &r.OptionsTargets, &r.OptionsTargetContinents, &targetProbs,
		); err != nil {
			return nil, err
		}
		r.Seat = int(seat)
		r.DefenderSeat, r.AttackerDice, r.DefenderDice = intPtr16(defSeat), intPtr16(aDice), intPtr16(dDice)
		r.AttackerLoss, r.DefenderLoss = intPtr16(aLoss), intPtr16(dLoss)
		r.WinProb, r.ExpectedAttackerLoss, r.ExpectedDefenderLoss, r.BestWinProb = f64Ptr(winProb), f64Ptr(expA), f64Ptr(expD), f64Ptr(best)
		if targetProbs != nil {
			r.OptionsWinProbs = make([]float64, len(targetProbs))
			for i, p := range targetProbs {
				r.OptionsWinProbs[i] = float64(p)
			}
		}
		r.OptionsByBucket = make([]int, len(buckets))
		for i, b := range buckets {
			r.OptionsByBucket[i] = int(b)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func intPtr16(v *int16) *int {
	if v == nil {
		return nil
	}
	x := int(*v)
	return &x
}

func f64Ptr(v *float32) *float64 {
	if v == nil {
		return nil
	}
	x := float64(*v)
	return &x
}
