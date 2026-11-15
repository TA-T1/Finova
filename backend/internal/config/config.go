package config

import (
	"fmt"
	"os"
	"strconv"
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
	// TSETMCTimeout is the per-request timeout (default 10s when zero).
	TSETMCTimeout time.Duration
	// TSETMCMaxAttempts is the total tries per request incl. the first (default 3 when zero).
	TSETMCMaxAttempts int
	// TSETMCRateLimit is the maximum requests per second (default 2; 0 = unlimited).
	TSETMCRateLimit float64
	// SnapshotTime is the daily snapshot time in Asia/Tehran, "HH:MM" (default 13:00).
	SnapshotTime string
	// SnapshotTries is the total attempts per day when a snapshot fails (default 3).
	SnapshotTries int
	// SnapshotRetryDelay is the wait between failed attempts (default 5m).
	SnapshotRetryDelay time.Duration
	// CollectInterval, when > 0, enables the background option-chain collector
	// at that cadence. Zero (the default) disables the scheduler.
	CollectInterval time.Duration
}

// Load reads the full application config. DATABASE_URL is required.
func Load() (*Config, error) {
	cfg, err := LoadSource()
	if err != nil {
		return nil, err
	}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
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

// LoadSource reads only the market-data source settings, so tools like
// cmd/fetchdemo work without a database. Unset values stay zero, and the
// tsetmc client then applies its defaults.
func LoadSource() (*Config, error) {
	_ = godotenv.Load()
	cfg := &Config{
		Port:               getenv("APP_PORT", "8080"),
		TSETMCBaseURL:      os.Getenv("TSETMC_BASE_URL"),
		TSETMCUserAgent:    os.Getenv("TSETMC_USER_AGENT"),
		TSETMCRateLimit:    2,
		SnapshotTime:       getenv("SNAPSHOT_TIME", "13:00"),
		SnapshotTries:      3,
		SnapshotRetryDelay: 5 * time.Minute,
	}
	if v := os.Getenv("TSETMC_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid TSETMC_TIMEOUT %q: want a positive duration like 10s", v)
		}
		cfg.TSETMCTimeout = d
	}
	if v := os.Getenv("TSETMC_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid TSETMC_MAX_ATTEMPTS %q: want an integer >= 1", v)
		}
		cfg.TSETMCMaxAttempts = n
	}
	if v := os.Getenv("SNAPSHOT_TRIES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid SNAPSHOT_TRIES %q: want an integer >= 1", v)
		}
		cfg.SnapshotTries = n
	}
	if v := os.Getenv("SNAPSHOT_RETRY_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid SNAPSHOT_RETRY_DELAY %q: want a positive duration like 5m", v)
		}
		cfg.SnapshotRetryDelay = d
	}
	if v := os.Getenv("TSETMC_RATE_LIMIT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			return nil, fmt.Errorf("invalid TSETMC_RATE_LIMIT %q: want requests/second >= 0 (0 = unlimited)", v)
		}
		cfg.TSETMCRateLimit = f
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
