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

	log.Printf("seed complete: users=%d", len(users))
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool) ([]seededUser, error) {
	usernames := []string{
		"test_alice",
		"test_bob",
		"test_cara",
		"test_dan",
		"test_erin",
		"test_frank",
	}

	out := make([]seededUser, 0, len(usernames))
	for _, uname := range usernames {
		hash, err := auth.HashPassword("password", auth.DefaultPasswordParams())
		if err != nil {
			return nil, fmt.Errorf("hash password for %s: %w", uname, err)
		}

		const q = `
			INSERT INTO users (username, password_hash, role)
			VALUES ($1, $2, 'player')
			ON CONFLICT (username)
			DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = now()
			RETURNING id::text, username
		`
		var u seededUser
		if err := pool.QueryRow(ctx, q, uname, hash).Scan(&u.ID, &u.Username); err != nil {
			return nil, fmt.Errorf("upsert user %s: %w", uname, err)
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
				pid := engine.Players[engine.CurrentPlayer].ID
				var owned risk.Territory
				for t, ts := range engine.Territories {
					if ts.Owner == engine.CurrentPlayer {
						owned = t
						break
					}
				}
				if owned == "" {
					return fmt.Errorf("no owned territory for initial reinforce")
				}
				if err := engine.PlaceInitialArmy(pid, owned); err != nil {
					return fmt.Errorf("place initial army: %w", err)
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
