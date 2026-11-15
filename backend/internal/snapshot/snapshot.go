// Package snapshot stores one end-of-day row per option contract per trading
// day in quote_snapshots. It calls the market adapter directly and never reads
// the live quote tables. See Run for the logic and Daemon for the scheduler.
package snapshot

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"Finova/internal/datasource"
)

const dateLayout = "2006-01-02"

// Row is one quote_snapshots row.
type Row struct {
	TradeDate        string // Tehran calendar date, YYYY-MM-DD
	ContractSymbol   string
	UnderlyingSymbol string
	OptionType       string
	Strike           int64
	Expiry           string // YYYY-MM-DD
	ClosePrice       int64
	LastPrice        int64
	BidPrice         int64
	AskPrice         int64
	Volume           int64
	Value            int64
	TradeCount       int64
	OpenInterest     int64
	UnderlyingPrice  *int64 // nil when the underlying has no usable last price
	CapturedAt       time.Time
}

type config struct {
	now func() time.Time
	log *slog.Logger
}

// Option customises Run.
type Option func(*config)

// WithClock replaces time.Now (for tests).
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// WithLogger replaces slog.Default().
func WithLogger(l *slog.Logger) Option { return func(c *config) { c.log = l } }

// Run takes one snapshot: it fetches the market once, skips the day if nothing
// traded, joins each contract with its underlying's price and upserts every row
// in one transaction. It returns the number of rows written (0 on a
// non-trading day, which is not an error). Running twice on the same day
// overwrites instead of duplicating.
func Run(ctx context.Context, src datasource.MarketSource, db *pgxpool.Pool, opts ...Option) (int, error) {
	cfg := config{now: time.Now, log: slog.Default()}
	for _, o := range opts {
		o(&cfg)
	}
	start := cfg.now()

	snap, err := src.FetchMarket(ctx)
	if err != nil {
		return 0, fmt.Errorf("snapshot: fetch market: %w", err)
	}

	now := start.In(datasource.Tehran)
	tradeDate := now.Format(dateLayout)

	traded, reason, err := hasTradedToday(ctx, db, snap, now)
	if err != nil {
		return 0, err
	}
	if !traded {
		cfg.log.Info("no trading today, nothing written", "trade_date", tradeDate, "reason", reason)
		return 0, nil
	}

	capturedAt := snap.FetchedAt
	if capturedAt.IsZero() {
		capturedAt = now
	}
	rows, stats := buildRows(snap, tradeDate, capturedAt)
	if stats.duplicates > 0 {
		cfg.log.Warn("duplicate contract symbols in snapshot, kept the last", "count", stats.duplicates)
	}
	if stats.noUnderlying > 0 {
		cfg.log.Warn("contracts without an underlying price, stored underlying_price as NULL",
			"count", stats.noUnderlying, "examples", stats.examples)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("snapshot: market returned no contracts")
	}

	n, err := upsert(ctx, db, rows)
	if err != nil {
		return 0, err
	}
	cfg.log.Info("snapshot written", "trade_date", tradeDate, "rows", n,
		"reason", reason, "duration", cfg.now().Sub(start).Round(time.Millisecond))
	return n, nil
}

type buildStats struct {
	duplicates   int
	noUnderlying int
	examples     []string // up to 3 underlying symbols that had no price
}

// buildRows joins every contract with its underlying's last price by
// underlying symbol. Duplicated contract symbols collapse to the last one,
// because the upsert cannot touch the same row twice in one statement.
func buildRows(snap *datasource.MarketSnapshot, tradeDate string, capturedAt time.Time) ([]Row, buildStats) {
	var st buildStats
	under := make(map[string]datasource.UnderlyingQuote, len(snap.Underlyings))
	for _, u := range snap.Underlyings {
		under[u.Symbol] = u
	}

	rows := make([]Row, 0, len(snap.Contracts))
	index := make(map[string]int, len(snap.Contracts))
	seenMissing := map[string]bool{}
	for _, c := range snap.Contracts {
		r := Row{
			TradeDate:        tradeDate,
			ContractSymbol:   c.Symbol,
			UnderlyingSymbol: c.UnderlyingSymbol,
			OptionType:       c.Type,
			Strike:           toInt(c.Strike),
			Expiry:           c.Expiry.In(datasource.Tehran).Format(dateLayout),
			ClosePrice:       toInt(c.ClosePrice),
			LastPrice:        toInt(c.LastPrice),
			BidPrice:         toInt(c.BidPrice),
			AskPrice:         toInt(c.AskPrice),
			Volume:           c.Volume,
			Value:            toInt(c.Value),
			TradeCount:       c.TradeCount,
			OpenInterest:     c.OpenInterest,
			CapturedAt:       capturedAt,
		}
		if u, ok := under[c.UnderlyingSymbol]; ok && u.LastPrice > 0 {
			p := toInt(u.LastPrice)
			r.UnderlyingPrice = &p
		} else {
			st.noUnderlying++
			if !seenMissing[c.UnderlyingSymbol] && len(st.examples) < 3 {
				seenMissing[c.UnderlyingSymbol] = true
				st.examples = append(st.examples, c.UnderlyingSymbol)
			}
		}
		if i, dup := index[r.ContractSymbol]; dup {
			st.duplicates++
			rows[i] = r
			continue
		}
		index[r.ContractSymbol] = len(rows)
		rows = append(rows, r)
	}
	return rows, st
}

func toInt(f float64) int64 { return int64(math.Round(f)) }
