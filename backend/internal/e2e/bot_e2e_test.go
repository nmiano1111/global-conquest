//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"

	"backend/internal/bot"
	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/risk"
	"backend/internal/service"
	"backend/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

// directSubmitter adapts service.GameActionService (the same adapter
// production wiring uses) to bot.ActionSubmitter, so the bot runner's
// commands flow through the real transactional ApplyGameAction path against
// a live Postgres instance, without needing a running WebSocket hub.
type directSubmitter struct {
	svc *service.GameActionService
}

func (d *directSubmitter) SubmitGameAction(ctx context.Context, in game.GameActionInput) (game.GameActionUpdate, error) {
	return d.svc.ApplyGameAction(ctx, in)
}

// insertLopsidedAttackGame inserts a 3-player game where botID owns every
// territory except one, which targetID owns with a single army; the third
// player owns nothing (already eliminated). It is botID's turn in the
// attack phase, so a bot that keeps attacking will conquer the last
// territory, eliminate targetID, and win outright.
func insertLopsidedAttackGame(t *testing.T, pool *pgxpool.Pool, botID, targetID, eliminatedID string) string {
	t.Helper()
	ctx := context.Background()

	g, err := risk.NewClassicGame([]string{botID, targetID, eliminatedID}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	botIdx, targetIdx, elimIdx := -1, -1, -1
	for i, p := range g.Players {
		switch p.ID {
		case botID:
			botIdx = i
		case targetID:
			targetIdx = i
		case eliminatedID:
			elimIdx = i
		}
	}

	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: botIdx, Armies: 3}
	}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: targetIdx, Armies: 1}
	g.SetupReserves = map[int]int{botIdx: 0, targetIdx: 0, elimIdx: 0}
	g.Players[elimIdx].Eliminated = true
	g.Players[botIdx].Controller = risk.ControllerBot
	g.Players[botIdx].Strategy = bot.StrategyBasicV1

	g.Phase = risk.PhaseAttack
	g.CurrentPlayer = botIdx
	g.PendingReinforcements = 0
	g.TurnNumber = 1

	stateJSON, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal game: %v", err)
	}

	var gameID string
	err = pool.QueryRow(ctx,
		`INSERT INTO games (owner_user_id, status, state) VALUES ($1::uuid, 'in_progress', $2::jsonb) RETURNING id::text`,
		botID, stateJSON,
	).Scan(&gameID)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	return gameID
}

// TestBotDrivesLopsidedGameToGameOver covers the end-to-end acceptance
// criteria: a game containing a bot player progresses through the attack,
// occupy, and fortify phases and reaches game_over through the same
// transactional command path humans use, without any territory-claim logic
// or advanced strategy being involved.
func TestBotDrivesLopsidedGameToGameOver(t *testing.T) {
	poolConn := setupDB(t)
	ctx := context.Background()

	botID := insertUser(t, poolConn, "bot_player")
	targetID := insertUser(t, poolConn, "target_player")
	eliminatedID := insertUser(t, poolConn, "eliminated_player")

	gameID := insertLopsidedAttackGame(t, poolConn, botID, targetID, eliminatedID)

	appDB := db.New(poolConn)
	gamesSvc := service.NewGamesService(appDB, store.NewPostgresGamesStore())
	gamesSvc.SetGameDomainEventStore(store.NewPostgresGameDomainEventStore())

	loader := service.NewBotGameLoader(gamesSvc)
	submitter := &directSubmitter{svc: service.NewGameActionService(gamesSvc)}
	strategies := bot.StrategyRegistry{bot.StrategyBasicV1: bot.NewBasicStrategy()}
	runner := bot.NewRunner(loader, submitter, strategies, bot.RealSleeper{}, 0)

	reason, err := runner.RunTurn(ctx, gameID, bot.ExecutionSimulation)
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if reason != bot.StopGameOver {
		t.Fatalf("expected the lopsided attack to end the game, got reason=%s", reason)
	}

	final, err := gamesSvc.GetGame(ctx, gameID)
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if final.Status != "completed" {
		t.Fatalf("expected status=completed, got %q", final.Status)
	}
	var finalState risk.Game
	if err := json.Unmarshal(final.State, &finalState); err != nil {
		t.Fatalf("unmarshal final state: %v", err)
	}
	if finalState.Phase != risk.PhaseGameOver {
		t.Fatalf("expected phase=game_over, got %s", finalState.Phase)
	}
	if finalState.Winner != botID {
		t.Fatalf("expected bot %q to win, got winner=%q", botID, finalState.Winner)
	}
}
