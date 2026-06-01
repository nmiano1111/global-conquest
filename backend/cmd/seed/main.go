package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"backend/internal/auth"
	"backend/internal/db"
	"backend/internal/risk"

	"github.com/jackc/pgx/v5/pgxpool"
)

type seededUser struct {
	ID       string
	Username string
}

type userSeedDef struct {
	Username string
	Role     string
}

type lobbyState struct {
	PlayerCount int      `json:"player_count"`
	PlayerIDs   []string `json:"player_ids"`
}

func main() {
	ctx := context.Background()

	cfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	users, err := seedUsers(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}

	if err := seedGames(ctx, pool, users); err != nil {
		log.Fatal(err)
	}

	if err := seedCompletedGames(ctx, pool, users); err != nil {
		log.Fatal(err)
	}

	log.Printf("seed complete: users=%d", len(users))
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool) ([]seededUser, error) {
	userDefs := []userSeedDef{
		{Username: "test_admin", Role: "admin"},
		{Username: "test_alice", Role: "player"},
		{Username: "test_bob", Role: "player"},
		{Username: "test_cara", Role: "player"},
		{Username: "test_dan", Role: "player"},
		{Username: "test_erin", Role: "player"},
		{Username: "test_frank", Role: "player"},
	}

	out := make([]seededUser, 0, len(userDefs))
	for _, def := range userDefs {
		hash, err := auth.HashPassword("password", auth.DefaultPasswordParams())
		if err != nil {
			return nil, fmt.Errorf("hash password for %s: %w", def.Username, err)
		}

		const q = `
			INSERT INTO users (username, password_hash, role)
			VALUES ($1, $2, $3)
			ON CONFLICT (username)
			DO UPDATE SET
				password_hash = EXCLUDED.password_hash,
				role = EXCLUDED.role,
				updated_at = now()
			RETURNING id::text, username
		`
		var u seededUser
		if err := pool.QueryRow(ctx, q, def.Username, hash, def.Role).Scan(&u.ID, &u.Username); err != nil {
			return nil, fmt.Errorf("upsert user %s: %w", def.Username, err)
		}
		out = append(out, u)
	}
	return out, nil
}

func seedGames(ctx context.Context, pool *pgxpool.Pool, users []seededUser) error {
	byName := map[string]seededUser{}
	for _, u := range users {
		byName[u.Username] = u
	}

	gameDefs := []struct {
		owner       string
		status      string
		playerCount int
		players     []string
	}{
		{
			owner:       "test_alice",
			status:      "lobby",
			playerCount: 4,
			players:     []string{"test_alice"},
		},
		{
			owner:       "test_bob",
			status:      "lobby",
			playerCount: 3,
			players:     []string{"test_bob", "test_cara"},
		},
		{
			owner:       "test_dan",
			status:      "in_progress",
			playerCount: 3,
			players:     []string{"test_dan", "test_erin", "test_frank"},
		},
	}

	const insertGame = `
		INSERT INTO games (owner_user_id, status, state)
		VALUES ($1::uuid, $2, $3::jsonb)
	`

	for _, def := range gameDefs {
		playerIDs := make([]string, 0, len(def.players))
		for _, p := range def.players {
			u, ok := byName[p]
			if !ok {
				return fmt.Errorf("missing seeded user %q", p)
			}
			playerIDs = append(playerIDs, u.ID)
		}

		var state []byte
		switch def.status {
		case "lobby":
			if len(playerIDs) == 0 {
				return fmt.Errorf("lobby seed for owner %s must include at least owner in players", def.owner)
			}
			lobby := lobbyState{
				PlayerCount: def.playerCount,
				PlayerIDs:   playerIDs,
			}
			var err error
			state, err = json.Marshal(lobby)
			if err != nil {
				return fmt.Errorf("marshal lobby state: %w", err)
			}
		case "in_progress":
			engine, err := risk.NewClassicGame(playerIDs, nil)
			if err != nil {
				return fmt.Errorf("build game state for owner %s: %w", def.owner, err)
			}

			// Nudge one game into a more realistic "in_progress" phase.
			for engine.Phase == risk.PhaseSetupClaim {
				pid := engine.Players[engine.CurrentPlayer].ID
				for _, terr := range engine.Board.Order {
					if engine.Territories[terr].Owner == -1 {
						if err := engine.ClaimTerritory(pid, terr); err != nil {
							return fmt.Errorf("claim setup territory: %w", err)
						}
						break
					}
				}
			}
			for engine.Phase == risk.PhaseSetupReinforce {
				placed := false
				for pi, p := range engine.Players {
					if engine.SetupReserves[pi] <= 0 {
						continue
					}
					var owned risk.Territory
					for t, ts := range engine.Territories {
						if ts.Owner == pi {
							owned = t
							break
						}
					}
					if owned == "" {
						continue
					}
					if err := engine.PlaceInitialArmy(p.ID, owned); err != nil {
						return fmt.Errorf("place initial army: %w", err)
					}
					placed = true
					break
				}
				if !placed {
					break
				}
			}
			state, err = json.Marshal(engine)
			if err != nil {
				return fmt.Errorf("marshal game state: %w", err)
			}
		default:
			return fmt.Errorf("unsupported game status %q", def.status)
		}

		owner := byName[def.owner]
		if _, err := pool.Exec(ctx, insertGame, owner.ID, def.status, state); err != nil {
			return fmt.Errorf("insert game for %s: %w", def.owner, err)
		}
	}

	return cleanupOldSeedGames(ctx, pool)
}

func seedCompletedGames(ctx context.Context, pool *pgxpool.Pool, users []seededUser) error {
	byName := map[string]seededUser{}
	for _, u := range users {
		byName[u.Username] = u
	}

	type completedGameDef struct {
		owner   string
		players []string
		winner  string
	}

	// bob wins 2, alice wins 1, cara wins 1, erin wins 1; dan and frank have losses only
	defs := []completedGameDef{
		{owner: "test_alice", players: []string{"test_alice", "test_bob", "test_cara"}, winner: "test_bob"},
		{owner: "test_bob", players: []string{"test_bob", "test_dan", "test_erin"}, winner: "test_bob"},
		{owner: "test_cara", players: []string{"test_cara", "test_dan", "test_frank"}, winner: "test_cara"},
		{owner: "test_alice", players: []string{"test_alice", "test_erin", "test_frank"}, winner: "test_alice"},
		{owner: "test_erin", players: []string{"test_erin", "test_bob", "test_alice"}, winner: "test_erin"},
	}

	const insertGame = `
		INSERT INTO games (owner_user_id, status, state)
		VALUES ($1::uuid, $2, $3::jsonb)
		RETURNING id::text
	`
	const insertPlayer = `
		INSERT INTO game_players (game_id, user_id, player_index, won)
		VALUES ($1::uuid, $2::uuid, $3, $4)
		ON CONFLICT (game_id, user_id) DO NOTHING
	`

	for _, def := range defs {
		playerIDs := make([]string, 0, len(def.players))
		for _, p := range def.players {
			u, ok := byName[p]
			if !ok {
				return fmt.Errorf("missing seeded user %q", p)
			}
			playerIDs = append(playerIDs, u.ID)
		}

		engine, err := risk.NewClassicGame(playerIDs, nil)
		if err != nil {
			return fmt.Errorf("build completed game state: %w", err)
		}
		state, err := json.Marshal(engine)
		if err != nil {
			return fmt.Errorf("marshal completed game state: %w", err)
		}

		owner := byName[def.owner]
		var gameID string
		if err := pool.QueryRow(ctx, insertGame, owner.ID, "completed", state).Scan(&gameID); err != nil {
			return fmt.Errorf("insert completed game for %s: %w", def.owner, err)
		}

		winnerID := byName[def.winner].ID
		for i, uid := range playerIDs {
			if _, err := pool.Exec(ctx, insertPlayer, gameID, uid, i, uid == winnerID); err != nil {
				return fmt.Errorf("insert game_player for game %s user %s: %w", gameID, uid, err)
			}
		}
	}

	return nil
}

func cleanupOldSeedGames(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
		DELETE FROM games
		WHERE id IN (
			SELECT g.id
			FROM games g
			JOIN users u ON u.id = g.owner_user_id
			WHERE u.username LIKE 'test_%'
			ORDER BY g.created_at DESC
			OFFSET 10
		)
	`
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, q)
	return err
}
