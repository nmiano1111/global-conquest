// Command reports is a local CLI for generating analytics reports directly
// from Postgres, without going through Discord. It reuses the same
// backend/internal/reporting package as the Discord bot.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"backend/internal/db"
	"backend/internal/reporting"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Println("warning: .env:", err)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "roll-streaks":
		if err := runRollStreaks(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: reports <command> [flags]

Commands:
  roll-streaks   Report attacking win/loss streaks and droughts

Run 'reports roll-streaks -h' for command-specific flags.`)
}

func newReportingService(ctx context.Context) (*reporting.Service, func(), error) {
	dbCfg, err := db.ConfigFromEnv()
	if err != nil {
		return nil, nil, fmt.Errorf("db config: %w", err)
	}
	pool, err := db.NewPool(ctx, dbCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("database connect: %w", err)
	}
	d := db.New(pool)
	svc := reporting.NewService(reporting.NewRepository(d.Queryer()))
	return svc, pool.Close, nil
}
