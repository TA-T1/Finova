package main

import (
	"context"
	"log/slog"
	"os"

	"Finova/internal/backtest"
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
		tsetmc.WithTimeout(cfg.TSETMCTimeout),
		tsetmc.WithMaxAttempts(cfg.TSETMCMaxAttempts),
		tsetmc.WithRateLimit(cfg.TSETMCRateLimit),
	)
	repo := market.NewRepository(db)
	collector := market.NewCollector(src, repo, slog.Default(),
		market.WithRiskFreeRate(cfg.RiskFreeRate))
	marketH := market.NewHandler(collector, repo)
	backtestH := backtest.NewHandler(backtest.NewRepository(db))

	// Optional background scheduler, enabled by COLLECT_INTERVAL.
	if cfg.CollectInterval > 0 {
		go collector.Run(ctx, cfg.CollectInterval)
	}

	r := httpapi.NewRouter(db, marketH, cfg.CORSAllowedOrigins, backtestH)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server", "err", err)
	}
}
