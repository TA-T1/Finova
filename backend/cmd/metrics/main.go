// Command metrics recalculates option metrics from quotes already stored in Postgres.
//
//	metrics recompute [-at RFC3339] [-dry-run]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Finova/internal/config"
	"Finova/internal/database"
	"Finova/internal/market"
)

type commandOptions struct {
	at     *time.Time
	dryRun bool
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "recompute" {
		fmt.Fprintln(os.Stderr, "usage: metrics recompute [-at RFC3339] [-dry-run]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		slog.Error("metrics recompute", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer db.Close()

	repo := market.NewRepository(db)
	quotes, err := repo.LatestQuotesForMetrics(ctx)
	if err != nil {
		return fmt.Errorf("load latest quotes: %w", err)
	}
	inputs, inputSkips := market.BuildMetricInputs(quotes, opts.at)
	for _, skipped := range inputSkips {
		slog.Warn("metrics contract skipped", "contractId", skipped.ContractID, "reason", skipped.Reason)
	}

	result, err := market.ComputeAndStoreMetrics(ctx, repo, inputs, cfg.RiskFreeRate, opts.dryRun, slog.Default())
	if err != nil {
		return fmt.Errorf("calculate or store metrics: %w", err)
	}
	skipsByReason := make(map[string]int)
	for _, skipped := range inputSkips {
		skipsByReason[skipped.Reason]++
	}
	for _, skipped := range result.Skipped {
		skipsByReason[skipped.Reason]++
	}
	slog.Info("metrics recompute complete",
		"read", len(quotes),
		"calculated", result.Calculated,
		"skipped", len(inputSkips)+len(result.Skipped),
		"skipReasons", skipsByReason,
		"stored", result.Stored,
		"dryRun", opts.dryRun)
	return nil
}

func parseOptions(args []string) (commandOptions, error) {
	fs := flag.NewFlagSet("recompute", flag.ContinueOnError)
	atValue := fs.String("at", "", "override metric timestamp for every contract (RFC3339)")
	dryRun := fs.Bool("dry-run", false, "calculate metrics without writing to the database")
	if err := fs.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if fs.NArg() != 0 {
		return commandOptions{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	opts := commandOptions{dryRun: *dryRun}
	if *atValue != "" {
		at, err := time.Parse(time.RFC3339, *atValue)
		if err != nil {
			return commandOptions{}, fmt.Errorf("invalid -at %q: want RFC3339 timestamp: %w", *atValue, err)
		}
		opts.at = &at
	}
	return opts, nil
}
