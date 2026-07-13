// Package db provides Postgres connection configuration and pool setup (via
// pgxpool), along with a thin transaction helper used throughout the store
// layer.
package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds the parameters needed to connect to a Postgres database.
type Config struct {
	// Host is the database server hostname.
	Host string
	// Port is the database server port.
	Port int
	// User is the database login username.
	User string
	// Password is the database login password.
	Password string
	// Database is the name of the database to connect to.
	Database string
	// SSLMode is the Postgres sslmode connection parameter (e.g. "disable").
	SSLMode string

	// MaxConns is the maximum number of connections the pool will open. A
	// value <= 0 leaves the pgxpool default in place.
	MaxConns int32
}

// ConfigFromEnv builds a Config from environment variables (DB_HOST,
// DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE), falling back to
// local-development defaults for any variable that is unset. MaxConns
// defaults to 10.
func ConfigFromEnv() (Config, error) {
	port := 5432
	if v := os.Getenv("DB_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("DB_PORT: %w", err)
		}
		port = p
	}

	cfg := Config{
		Host:     getenv("DB_HOST", "localhost"),
		Port:     port,
		User:     getenv("DB_USER", "globalconq"),
		Password: getenv("DB_PASSWORD", "globalconq"),
		Database: getenv("DB_NAME", "globalconq"),
		SSLMode:  getenv("DB_SSL_MODE", "disable"),
		MaxConns: 10,
	}
	return cfg, nil
}

// ConnString returns c formatted as a Postgres connection URL
// ("postgres://user:password@host:port/database?sslmode=...").
func (c Config) ConnString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode,
	)
}

// NewPool creates a pgxpool.Pool for cfg and verifies connectivity with a
// bounded (3 second) ping, closing the pool and returning an error if the
// config is invalid, the pool cannot be created, or the ping fails.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.ConnString())
	if err != nil {
		return nil, err
	}
	if cfg.MaxConns > 0 {
		pcfg.MaxConns = cfg.MaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, err
	}

	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
