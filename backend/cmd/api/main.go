package main

import (
	"context"
	"log/slog"
	"os"
	"Finova/internal/config"
	"Finova/internal/database"
	"Finova/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	r := httpapi.NewRouter(db)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server", "err", err)
	}
}