package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string

	MaxConns int32
}

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

func (c Config) ConnString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode,
	)
}

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
