// Command snapshot stores the end-of-day option prices in quote_snapshots.
//
//	snapshot run      take one snapshot now and exit
//	snapshot daemon   stay running and snapshot every day at SNAPSHOT_TIME
//	                  (default 13:00 Asia/Tehran) until SIGINT/SIGTERM
//
// It needs DATABASE_URL and reaches TSETMC through the market adapter. See
// .env.example for the SNAPSHOT_* and TSETMC_* settings.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"Finova/internal/config"
	"Finova/internal/database"
	"Finova/internal/datasource"
	"Finova/internal/datasource/tsetmc"
	"Finova/internal/snapshot"
	"Finova/internal/source"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "run" && os.Args[1] != "daemon") {
		fmt.Fprintln(os.Stderr, "usage: snapshot run | snapshot daemon")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		slog.Error("snapshot", "err", err)
		os.Exit(1)
	}
}

func run(mode string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	at, err := snapshot.ParseTimeOfDay(cfg.SnapshotTime)
	if err != nil {
		return fmt.Errorf("config SNAPSHOT_TIME: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer db.Close()

	var src source.Source = tsetmc.New(
		tsetmc.WithBaseURL(cfg.TSETMCBaseURL),
		tsetmc.WithUserAgent(cfg.TSETMCUserAgent),
		tsetmc.WithTimeout(cfg.TSETMCTimeout),
		tsetmc.WithMaxAttempts(cfg.TSETMCMaxAttempts),
		tsetmc.WithRateLimit(cfg.TSETMCRateLimit),
	)
	task := func(ctx context.Context) error {
		_, err := snapshot.Run(ctx, src, db)
		return err
	}

	if mode == "run" {
		return task(ctx)
	}

	slog.Info("snapshot daemon started", "at", at.String(), "zone", datasource.Tehran.String(),
		"tries", cfg.SnapshotTries, "retry_delay", cfg.SnapshotRetryDelay)
	d := &snapshot.Daemon{
		At: at, Loc: datasource.Tehran,
		Tries: cfg.SnapshotTries, RetryDelay: cfg.SnapshotRetryDelay,
		Task: task,
	}
	err = d.Run(ctx)
	slog.Info("snapshot daemon stopped")
	return err
}
