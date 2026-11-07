package main

import (
	"context"
	"log/slog"
	"os"

	"Finova/internal/config"
	"Finova/internal/database"
	"Finova/internal/datasource/tsetmc"
	"Finova/internal/httpapi"
	"Finova/internal/market"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Market data module: TSETMC source -> repository -> collector -> handler.
	src := tsetmc.New(
		tsetmc.WithBaseURL(cfg.TSETMCBaseURL),
		tsetmc.WithUserAgent(cfg.TSETMCUserAgent),
	)
	repo := market.NewRepository(db)
	collector := market.NewCollector(src, repo, slog.Default())
	marketH := market.NewHandler(collector, repo)

	// Optional background scheduler, enabled by COLLECT_INTERVAL.
	if cfg.CollectInterval > 0 {
		go collector.Run(ctx, cfg.CollectInterval)
	}

	r := httpapi.NewRouter(db, marketH)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server", "err", err)
	}
}
