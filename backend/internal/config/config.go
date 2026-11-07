package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	DatabaseURL string

	// TSETMCBaseURL overrides the TSETMC API base (no trailing slash).
	TSETMCBaseURL string
	// TSETMCUserAgent overrides the User-Agent sent to TSETMC.
	TSETMCUserAgent string
	// CollectInterval, when > 0, enables the background option-chain collector
	// at that cadence. Zero (the default) disables the scheduler.
	CollectInterval time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()
	cfg := &Config{
		Port:            getenv("APP_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		TSETMCBaseURL:   os.Getenv("TSETMC_BASE_URL"),
		TSETMCUserAgent: os.Getenv("TSETMC_USER_AGENT"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if v := os.Getenv("COLLECT_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid COLLECT_INTERVAL %q: %w", v, err)
		}
		cfg.CollectInterval = d
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
