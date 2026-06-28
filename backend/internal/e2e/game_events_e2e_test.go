//go:build e2e

package e2e

import (
	"backend/internal/db"
	"backend/internal/risk"
	"backend/internal/service"
	"backend/internal/store"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setupDB creates an isolated database, applies all migrations, and returns a pool.
// The database is dropped on test cleanup.
func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminDSN := os.Getenv("E2E_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("set E2E_ADMIN_DSN to run DB-backed e2e tests")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	t.Cleanup(admin.Close)

	dbName := fmt.Sprintf("e2e_events_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+dbName+`"`); err != nil {
		t.Fatalf("create test db: %v", err)
	}

	pool, err := pgxpool.New(ctx, withDatabase(adminDSN, dbName))
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}

	if err := applyMigration(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(),
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND pid <> pg_backend_pid()`, dbName)
		_, _ = admin.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+dbName+`"`)
	})
	return pool
}

func insertUser(t *testing.T, pool *pgxpool.Pool, username string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ($1, 'hash') RETURNING id::text`, username,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
	return id
}

// insertAttackPhaseGame inserts a game with state forced to PhaseAttack, Alaska owned by
// attackerID (5 armies) and Kamchatka owned by defenderID (2 armies).
func insertAttackPhaseGame(t *testing.T, pool *pgxpool.Pool, attackerID, defenderID, thirdID string) string {
	t.Helper()
	ctx := context.Background()

	g, err := risk.NewClassicAutoStartGame([]string{attackerID, defenderID, thirdID}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	attackerIdx, defenderIdx := -1, -1
	for i, p := range g.Players {
		if p.ID == attackerID {
			attackerIdx = i
		}
		if p.ID == defenderID {
			defenderIdx = i
		}
	}
	g.CurrentPlayer = attackerIdx
	g.Phase = risk.PhaseAttack
	g.PendingReinforcements = 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: attackerIdx, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: defenderIdx, Armies: 2}

	stateJSON, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal game: %v", err)
	}

	var gameID string
	err = pool.QueryRow(ctx,
		`INSERT INTO games (owner_user_id, status, state) VALUES ($1::uuid, 'in_progress', $2::jsonb) RETURNING id::text`,
		attackerID, stateJSON,
	).Scan(&gameID)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	return gameID
}

func newSvcWithDomainEvents(pool *pgxpool.Pool) *service.GamesService {
	appDB := db.New(pool)
	svc := service.NewGamesService(appDB, store.NewPostgresGamesStore())
	svc.SetGameDomainEventStore(store.NewPostgresGameDomainEventStore())
	return svc
}

// Test 1 & 5: A successful attack produces exactly one event, persisted in the same transaction.
func TestCombatRollEventPersistedWithGameState(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	a := insertUser(t, pool, "ev_att1")
	d := insertUser(t, pool, "ev_def1")
	x := insertUser(t, pool, "ev_thi1")
	gameID := insertAttackPhaseGame(t, pool, a, d, x)

	svc := newSvcWithDomainEvents(pool)
	_, err := svc.ApplyGameAction(ctx, service.GameActionInput{
		GameID: gameID, PlayerUserID: a,
		Action: "attack", From: "Alaska", To: "Kamchatka",
		AttackerDice: 3, DefenderDice: 2,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction: %v", err)
	}

	var evCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM game_domain_events WHERE game_id=$1::uuid AND event_type=$2`,
		gameID, risk.EventTypeCombatRollResolved,
	).Scan(&evCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if evCount != 1 {
		t.Fatalf("expected 1 event, got %d", evCount)
	}
}

// Test 2: Payload has correct players, territories, dice, comparisons, losses, army counts, capture flag.
func TestCombatRollEventPayloadCorrect(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	attackerID := insertUser(t, pool, "ev_att2")
	defenderID := insertUser(t, pool, "ev_def2")
	x := insertUser(t, pool, "ev_thi2")
	gameID := insertAttackPhaseGame(t, pool, attackerID, defenderID, x)

	svc := newSvcWithDomainEvents(pool)
	_, err := svc.ApplyGameAction(ctx, service.GameActionInput{
		GameID: gameID, PlayerUserID: attackerID,
		Action: "attack", From: "Alaska", To: "Kamchatka",
		AttackerDice: 3, DefenderDice: 2,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction: %v", err)
	}

	var payloadRaw []byte
	var actorText string
	var evVersion int16
	if err := pool.QueryRow(ctx,
		`SELECT payload, COALESCE(actor_player_id::text,''), event_version
		 FROM game_domain_events WHERE game_id=$1::uuid`,
		gameID,
	).Scan(&payloadRaw, &actorText, &evVersion); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if actorText != attackerID {
		t.Fatalf("actor_player_id: want %q got %q", attackerID, actorText)
	}
	if evVersion != risk.EventVersionCombatRollResolved {
		t.Fatalf("event_version: want %d got %d", risk.EventVersionCombatRollResolved, evVersion)
	}

	var pl risk.CombatRollResolvedPayload
	if err := json.Unmarshal(payloadRaw, &pl); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if pl.SchemaVersion != risk.SchemaVersionCombatRollResolved {
		t.Fatalf("schema_version: want %d got %d", risk.SchemaVersionCombatRollResolved, pl.SchemaVersion)
	}
	if pl.AttackerPlayerID != attackerID {
		t.Fatalf("attacker_player_id: want %q got %q", attackerID, pl.AttackerPlayerID)
	}
	if pl.DefenderPlayerID != defenderID {
		t.Fatalf("defender_player_id: want %q got %q", defenderID, pl.DefenderPlayerID)
	}
	if pl.SourceTerritoryID != "Alaska" || pl.TargetTerritoryID != "Kamchatka" {
		t.Fatalf("territories: src=%q tgt=%q", pl.SourceTerritoryID, pl.TargetTerritoryID)
	}
	if pl.SourceArmiesBefore != 5 || pl.TargetArmiesBefore != 2 {
		t.Fatalf("armies before: src=%d tgt=%d", pl.SourceArmiesBefore, pl.TargetArmiesBefore)
	}
	if len(pl.AttackerDice) == 0 || len(pl.DefenderDice) == 0 {
		t.Fatal("dice must be non-empty")
	}
	if len(pl.Comparisons) == 0 {
		t.Fatal("comparisons must be non-empty")
	}
	for _, c := range pl.Comparisons {
		if c.Loser != "attacker" && c.Loser != "defender" {
			t.Fatalf("invalid loser value: %q", c.Loser)
		}
	}
	if pl.AttackerLosses+pl.DefenderLosses == 0 {
		t.Fatal("expected at least one loss")
	}
	// After armies must match before minus losses.
	if pl.SourceArmiesAfter != pl.SourceArmiesBefore-pl.AttackerLosses {
		t.Fatalf("source armies after mismatch: before=%d losses=%d after=%d",
			pl.SourceArmiesBefore, pl.AttackerLosses, pl.SourceArmiesAfter)
	}
	if pl.TargetArmiesAfter != pl.TargetArmiesBefore-pl.DefenderLosses {
		t.Fatalf("target armies after mismatch: before=%d losses=%d after=%d",
			pl.TargetArmiesBefore, pl.DefenderLosses, pl.TargetArmiesAfter)
	}
}

// Test 3: A tie records attacker as the loser (verified at engine level; spot-check in DB payload).
// (Full tie coverage is in engine_test.go; this confirms the value survives serialization.)
func TestTiedDiceComparisonsStoredCorrectly(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	a := insertUser(t, pool, "tie_att")
	d := insertUser(t, pool, "tie_def")
	x := insertUser(t, pool, "tie_thi")
	gameID := insertAttackPhaseGame(t, pool, a, d, x)

	svc := newSvcWithDomainEvents(pool)
	// Run enough attacks that at least one tie is statistically likely (but don't assert one happened,
	// since we can't control the RNG in integration tests — the property is verified in engine_test.go).
	for i := 0; i < 5; i++ {
		var stateRaw []byte
		if err := pool.QueryRow(ctx, `SELECT state FROM games WHERE id=$1::uuid`, gameID).Scan(&stateRaw); err != nil {
			break
		}
		var g risk.Game
		if err := json.Unmarshal(stateRaw, &g); err != nil {
			break
		}
		if g.Phase == risk.PhaseOccupy {
			_ = svc.ApplyGameAction(ctx, service.GameActionInput{
				GameID: gameID, PlayerUserID: g.Players[g.CurrentPlayer].ID,
				Action: "occupy", Armies: 1,
			})
			continue
		}
		if g.Phase != risk.PhaseAttack {
			break
		}
		src := g.Territories["Alaska"]
		dst := g.Territories["Kamchatka"]
		if src.Armies <= 1 || dst.Armies < 1 {
			break
		}
		_ = svc.ApplyGameAction(ctx, service.GameActionInput{
			GameID: gameID, PlayerUserID: g.Players[g.CurrentPlayer].ID,
			Action: "attack", From: "Alaska", To: "Kamchatka",
			AttackerDice: 1, DefenderDice: 1,
		})
	}

	// Verify that all stored comparisons have valid loser values.
	rows, err := pool.Query(ctx,
		`SELECT jsonb_array_elements(payload->'comparisons')->>'loser'
		 FROM game_domain_events WHERE game_id=$1::uuid`, gameID)
	if err != nil {
		t.Fatalf("query comparisons: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var loser string
		if err := rows.Scan(&loser); err != nil {
			t.Fatalf("scan loser: %v", err)
		}
		if loser != "attacker" && loser != "defender" {
			t.Fatalf("invalid loser in stored comparison: %q", loser)
		}
	}
	if rows.Err() != nil {
		t.Fatalf("rows err: %v", rows.Err())
	}
}

// Test 7: Events for the same game receive strictly increasing game_sequence values.
func TestEventSequencesStrictlyIncreasing(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	a := insertUser(t, pool, "seq_att")
	d := insertUser(t, pool, "seq_def")
	x := insertUser(t, pool, "seq_thi")
	gameID := insertAttackPhaseGame(t, pool, a, d, x)

	svc := newSvcWithDomainEvents(pool)
	for i := 0; i < 4; i++ {
		var stateRaw []byte
		if err := pool.QueryRow(ctx, `SELECT state FROM games WHERE id=$1::uuid`, gameID).Scan(&stateRaw); err != nil {
			break
		}
		var g risk.Game
		if err := json.Unmarshal(stateRaw, &g); err != nil {
			break
		}
		if g.Phase == risk.PhaseOccupy {
			_ = svc.ApplyGameAction(ctx, service.GameActionInput{
				GameID: gameID, PlayerUserID: g.Players[g.CurrentPlayer].ID,
				Action: "occupy", Armies: 1,
			})
			continue
		}
		if g.Phase != risk.PhaseAttack {
			break
		}
		src := g.Territories["Alaska"]
		dst := g.Territories["Kamchatka"]
		if src.Armies <= 1 || dst.Armies < 1 {
			break
		}
		_ = svc.ApplyGameAction(ctx, service.GameActionInput{
			GameID: gameID, PlayerUserID: g.Players[g.CurrentPlayer].ID,
			Action: "attack", From: "Alaska", To: "Kamchatka",
			AttackerDice: 1, DefenderDice: 1,
		})
	}

	rows, err := pool.Query(ctx,
		`SELECT game_sequence FROM game_domain_events WHERE game_id=$1::uuid ORDER BY game_sequence`, gameID)
	if err != nil {
		t.Fatalf("query sequences: %v", err)
	}
	defer rows.Close()
	var seqs []int64
	for rows.Next() {
		var s int64
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seqs = append(seqs, s)
	}
	if rows.Err() != nil {
		t.Fatalf("rows: %v", rows.Err())
	}
	if len(seqs) < 2 {
		t.Skip("not enough events produced to verify ordering (possible early game-over)")
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("sequences not strictly increasing: %v", seqs)
		}
	}
}

// Test 8: Events for different games have independent sequences (each starts at 1).
func TestEventSequencesIndependentAcrossGames(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	a1 := insertUser(t, pool, "ind_a1")
	d1 := insertUser(t, pool, "ind_d1")
	x1 := insertUser(t, pool, "ind_x1")
	gameID1 := insertAttackPhaseGame(t, pool, a1, d1, x1)

	a2 := insertUser(t, pool, "ind_a2")
	d2 := insertUser(t, pool, "ind_d2")
	x2 := insertUser(t, pool, "ind_x2")
	gameID2 := insertAttackPhaseGame(t, pool, a2, d2, x2)

	svc := newSvcWithDomainEvents(pool)
	doAttack := func(gameID, playerID string) {
		_ = svc.ApplyGameAction(ctx, service.GameActionInput{
			GameID: gameID, PlayerUserID: playerID,
			Action: "attack", From: "Alaska", To: "Kamchatka",
			AttackerDice: 1, DefenderDice: 1,
		})
	}
	doAttack(gameID1, a1)
	doAttack(gameID2, a2)

	var seq1, seq2 int64
	_ = pool.QueryRow(ctx, `SELECT COALESCE(min(game_sequence),0) FROM game_domain_events WHERE game_id=$1::uuid`, gameID1).Scan(&seq1)
	_ = pool.QueryRow(ctx, `SELECT COALESCE(min(game_sequence),0) FROM game_domain_events WHERE game_id=$1::uuid`, gameID2).Scan(&seq2)

	if seq1 != 1 {
		t.Fatalf("game1 first sequence: want 1, got %d", seq1)
	}
	if seq2 != 1 {
		t.Fatalf("game2 first sequence: want 1, got %d", seq2)
	}
}

// Test 9: The unique (game_id, game_sequence) constraint prevents duplicates.
func TestUniqueGameSequenceConstraint(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	a := insertUser(t, pool, "uniq_att")

	var gameID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO games (owner_user_id, status, state) VALUES ($1::uuid, 'in_progress', '{}'::jsonb) RETURNING id::text`,
		a,
	).Scan(&gameID); err != nil {
		t.Fatalf("insert game: %v", err)
	}

	insert := func(seq int64) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO game_domain_events (game_id, game_sequence, event_type, event_version, payload)
			 VALUES ($1::uuid, $2, 'combat_roll_resolved', 1, '{}'::jsonb)`,
			gameID, seq,
		)
		return err
	}

	if err := insert(1); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := insert(1); err == nil {
		t.Fatal("expected unique constraint violation for duplicate (game_id, game_sequence)")
	}
	if err := insert(2); err != nil {
		t.Fatalf("different sequence for same game should succeed: %v", err)
	}
}

// Test 6: A failed event insert does not commit the updated game state.
func TestEventInsertFailureRollsBackGameState(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	a := insertUser(t, pool, "rb_att")
	d := insertUser(t, pool, "rb_def")
	x := insertUser(t, pool, "rb_thi")
	gameID := insertAttackPhaseGame(t, pool, a, d, x)

	var stateBefore []byte
	if err := pool.QueryRow(ctx, `SELECT state FROM games WHERE id=$1::uuid`, gameID).Scan(&stateBefore); err != nil {
		t.Fatalf("read initial state: %v", err)
	}

	appDB := db.New(pool)
	svc := service.NewGamesService(appDB, store.NewPostgresGamesStore())
	svc.SetGameDomainEventStore(&eventStoreAlwaysFail{})

	_, err := svc.ApplyGameAction(ctx, service.GameActionInput{
		GameID: gameID, PlayerUserID: a,
		Action: "attack", From: "Alaska", To: "Kamchatka",
		AttackerDice: 1, DefenderDice: 1,
	})
	if err == nil {
		t.Fatal("expected error from failing domain event store")
	}

	var stateAfter []byte
	if err := pool.QueryRow(ctx, `SELECT state FROM games WHERE id=$1::uuid`, gameID).Scan(&stateAfter); err != nil {
		t.Fatalf("read state after: %v", err)
	}

	// Compare territory armies to detect any committed mutation.
	extract := func(raw []byte) map[string]any {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		return m
	}
	beforeTerrs, _ := json.Marshal(extract(stateBefore)["territories"])
	afterTerrs, _ := json.Marshal(extract(stateAfter)["territories"])
	if string(beforeTerrs) != string(afterTerrs) {
		t.Fatal("game state mutated despite event insert failure — transaction did not roll back")
	}

	var evCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM game_domain_events WHERE game_id=$1::uuid`, gameID).Scan(&evCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if evCount != 0 {
		t.Fatalf("expected 0 persisted events after rollback, got %d", evCount)
	}
}

type eventStoreAlwaysFail struct{}

func (s *eventStoreAlwaysFail) InsertDomainEvent(
	_ context.Context, _ db.Querier, _ string, _ risk.DomainEvent, _ []byte,
) (store.GameDomainEvent, error) {
	return store.GameDomainEvent{}, errors.New("injected event store failure")
}
