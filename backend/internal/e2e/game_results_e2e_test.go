//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/service"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

// TestGameResultsRecordedOnCompletionAndSurviveDeletion drives a mixed
// human/bot game to game over through the real ApplyGameAction path and
// checks the admin-stats rows written in the final transaction: game
// metadata, the bot's strategy-keyed row with its winning attack counted,
// placements from elimination order, and that the rows outlive the game.
func TestGameResultsRecordedOnCompletionAndSurviveDeletion(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	botID := insertUser(t, pool, "bot_player")
	targetID := insertUser(t, pool, "target_player")
	eliminatedID := insertUser(t, pool, "eliminated_player")
	gameID := insertLopsidedAttackGame(t, pool, botID, targetID, eliminatedID)

	appDB := db.New(pool)
	resultsStore := store.NewPostgresGameResultsStore()
	gamesSvc := service.NewGamesService(appDB, store.NewPostgresGamesStore())
	gamesSvc.SetGameDomainEventStore(store.NewPostgresGameDomainEventStore())
	gamesSvc.SetGameReplayEventStore(store.NewPostgresGameReplayEventStore())
	gamesSvc.SetGameResultsStore(resultsStore)

	runner := bot.NewRunner(service.NewBotGameLoader(gamesSvc),
		&directSubmitter{svc: service.NewGameActionService(gamesSvc)},
		bot.StrategyRegistry{bot.StrategyBasicV1: bot.NewBasicStrategy()},
		bot.RealSleeper{}, bot.DefaultPacingConfig())
	if reason, err := runner.RunTurn(ctx, gameID, bot.ExecutionSimulation); err != nil || reason != bot.StopGameOver {
		t.Fatalf("RunTurn: reason=%s err=%v", reason, err)
	}

	var playerCount, humanCount, botCount int
	if err := pool.QueryRow(ctx,
		`SELECT player_count, human_count, bot_count FROM game_results WHERE game_id = $1::uuid`, gameID,
	).Scan(&playerCount, &humanCount, &botCount); err != nil {
		t.Fatalf("game_results row: %v", err)
	}
	if playerCount != 3 || humanCount != 2 || botCount != 1 {
		t.Fatalf("counts = %d/%d/%d, want 3/2/1", playerCount, humanCount, botCount)
	}

	// Deleting the game must not erase its stats (no FK to games).
	if _, err := pool.Exec(ctx, `DELETE FROM games WHERE id = $1::uuid`, gameID); err != nil {
		t.Fatalf("delete game: %v", err)
	}

	stats := service.NewStatsService(appDB, resultsStore)
	botDetail, err := stats.PlayerDetail(ctx, "bot:"+bot.StrategyBasicV1, store.GameResultsFilter{})
	if err != nil {
		t.Fatalf("bot detail: %v", err)
	}
	if botDetail.Summary.Wins != 1 || botDetail.Summary.Games != 1 {
		t.Fatalf("bot record = %+v", botDetail.Summary)
	}
	g := botDetail.Games[0]
	if g.Placement == nil || *g.Placement != 1 {
		t.Fatalf("bot placement = %v, want 1", g.Placement)
	}
	if g.Eliminations == nil || *g.Eliminations != 1 || g.Conquests == nil || *g.Conquests < 1 {
		t.Fatalf("bot eliminations/conquests = %v/%v, want 1/>=1", g.Eliminations, g.Conquests)
	}

	target, err := stats.PlayerDetail(ctx, targetID, store.GameResultsFilter{})
	if err != nil {
		t.Fatalf("target detail: %v", err)
	}
	if tg := target.Games[0]; tg.Won || tg.Placement == nil || *tg.Placement != 2 {
		t.Fatalf("target game = %+v, want loss in 2nd place", tg)
	}
	// Eliminated before history began: no elimination event, so unknown.
	pre, err := stats.PlayerDetail(ctx, eliminatedID, store.GameResultsFilter{})
	if err != nil {
		t.Fatalf("pre-eliminated detail: %v", err)
	}
	if pre.Games[0].Placement != nil {
		t.Fatalf("pre-eliminated placement = %d, want nil", *pre.Games[0].Placement)
	}

	// Filters: a mixed game is excluded from human-only results.
	if _, err := stats.PlayerDetail(ctx, targetID, store.GameResultsFilter{Composition: store.CompositionHumanOnly}); err != service.ErrStatsPlayerNotFound {
		t.Fatalf("human_only filter: err=%v, want ErrStatsPlayerNotFound", err)
	}
}

// TestGameResultsBackfillIsIdempotent checks that startup backfill records
// a completed game finished without the results store wired, exactly once.
func TestGameResultsBackfillIsIdempotent(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	botID := insertUser(t, pool, "bot_player")
	targetID := insertUser(t, pool, "target_player")
	eliminatedID := insertUser(t, pool, "eliminated_player")
	gameID := insertLopsidedAttackGame(t, pool, botID, targetID, eliminatedID)

	appDB := db.New(pool)
	gamesSvc := service.NewGamesService(appDB, store.NewPostgresGamesStore())
	gamesSvc.SetGameReplayEventStore(store.NewPostgresGameReplayEventStore())
	runner := bot.NewRunner(service.NewBotGameLoader(gamesSvc),
		&directSubmitter{svc: service.NewGameActionService(gamesSvc)},
		bot.StrategyRegistry{bot.StrategyBasicV1: bot.NewBasicStrategy()},
		bot.RealSleeper{}, bot.DefaultPacingConfig())
	if _, err := runner.RunTurn(ctx, gameID, bot.ExecutionSimulation); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	rs := store.NewPostgresGameResultsStore()
	for i, want := range []int{1, 0} {
		n, err := rs.BackfillCompleted(ctx, appDB.Queryer())
		if err != nil {
			t.Fatalf("backfill #%d: %v", i+1, err)
		}
		if n != want {
			t.Fatalf("backfill #%d recorded %d games, want %d", i+1, n, want)
		}
	}
	var seats int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM game_result_players WHERE game_id = $1::uuid`, gameID).Scan(&seats); err != nil || seats != 3 {
		t.Fatalf("seat rows = %d (err %v), want 3", seats, err)
	}
}
