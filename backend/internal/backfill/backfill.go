// Package backfill fills quote_snapshots with the price history of the
// option contracts that are listed today.
//
// TSETMC serves a daily history per instrument (GetClosingPriceDailyList), and
// that works for option contracts too: about 60 days per current contract, since
// its listing. The history has no best bid/ask and no open interest, so backfilled
// rows keep those at 0 ("unknown"), and a row that already exists is never
// overwritten, because a live snapshot is richer. See docs/api.md, "Historical
// option prices".
package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"Finova/internal/datasource"
	"Finova/internal/snapshot"
)

const dateLayout = "2006-01-02"

// flushRows is the number of rows collected before they are written. Writing in
// batches keeps the progress of a 15-minute run when it is interrupted.
const flushRows = 20000

// Source is what the backfill needs from a provider: the current option chain
// (contract and underlying instrument codes) and daily price history.
type Source interface {
	OptionMarketWatch(ctx context.Context) ([]datasource.OptionRow, error)
	DailyPrices(ctx context.Context, insCode string, adjusted bool) ([]datasource.DailyPrice, error)
}

// Writer stores rows without overwriting existing ones. It returns the number of
// rows inserted.
type Writer func(ctx context.Context, rows []snapshot.Row) (int, error)

// PoolWriter writes with snapshot.InsertMissing.
func PoolWriter(db *pgxpool.Pool) Writer {
	return func(ctx context.Context, rows []snapshot.Row) (int, error) {
		return snapshot.InsertMissing(ctx, db, rows)
	}
}

// Result is the outcome of a run.
type Result struct {
	Contracts   int // contracts in the chain
	Fetched     int // contracts whose history was fetched
	Failed      int // contracts (or underlyings) whose history request failed
	Rows        int // rows built from history
	Inserted    int // rows actually inserted (the rest already existed)
	FirstDate   string
	LastDate    string
	FirstErrMsg string
}

type config struct {
	now   func() time.Time
	log   *slog.Logger
	limit int
}

// Option customises Run.
type Option func(*config)

// WithClock replaces time.Now (for tests).
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// WithLogger replaces slog.Default().
func WithLogger(l *slog.Logger) Option { return func(c *config) { c.log = l } }

// WithLimit stops after n contracts (for a quick trial run). Zero means all.
func WithLimit(n int) Option { return func(c *config) { c.limit = n } }

type target struct {
	insCode, symbol, underlyingSymbol, underlyingIns string
	optionType                                       datasource.OptionType
	strike                                           int64
	expiry                                           time.Time
}

// Run backfills the history of every contract in the current chain. Only days
// before today (Tehran) are written: today comes from the live snapshot.
func Run(ctx context.Context, src Source, write Writer, opts ...Option) (Result, error) {
	cfg := config{now: time.Now, log: slog.Default()}
	for _, o := range opts {
		o(&cfg)
	}
	var res Result

	chain, err := src.OptionMarketWatch(ctx)
	if err != nil {
		return res, fmt.Errorf("backfill: fetch option chain: %w", err)
	}
	targets := targetsOf(chain)
	res.Contracts = len(targets)
	if cfg.limit > 0 && len(targets) > cfg.limit {
		targets = targets[:cfg.limit]
	}
	if len(targets) == 0 {
		return res, fmt.Errorf("backfill: the option chain has no contracts")
	}

	now := cfg.now()
	today := now.In(datasource.Tehran).Format(dateLayout)

	// Underlying history: one request per underlying, needed for underlying_price.
	underPrices := map[string]map[string]int64{} // underlying insCode -> date -> last price
	for _, t := range targets {
		if _, done := underPrices[t.underlyingIns]; done {
			continue
		}
		hist, err := src.DailyPrices(ctx, t.underlyingIns, false)
		if err != nil {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			res.fail(err)
			cfg.log.Warn("underlying history failed, its rows get a NULL underlying_price",
				"underlying", t.underlyingSymbol, "err", err)
			underPrices[t.underlyingIns] = nil
			continue
		}
		m := make(map[string]int64, len(hist))
		for _, d := range hist {
			if p := lastPrice(d); p > 0 {
				m[d.Date.In(datasource.Tehran).Format(dateLayout)] = p
			}
		}
		underPrices[t.underlyingIns] = m
	}

	var batch []snapshot.Row
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := write(ctx, batch)
		if err != nil {
			return fmt.Errorf("backfill: write %d rows: %w", len(batch), err)
		}
		res.Inserted += n
		batch = batch[:0]
		return nil
	}

	for i, t := range targets {
		hist, err := src.DailyPrices(ctx, t.insCode, false)
		if err != nil {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			res.fail(err)
			cfg.log.Warn("contract history failed", "contract", t.symbol, "err", err)
			continue
		}
		res.Fetched++
		for _, r := range rowsOf(t, hist, underPrices[t.underlyingIns], today, now) {
			if res.FirstDate == "" || r.TradeDate < res.FirstDate {
				res.FirstDate = r.TradeDate
			}
			if r.TradeDate > res.LastDate {
				res.LastDate = r.TradeDate
			}
			res.Rows++
			batch = append(batch, r)
		}
		if len(batch) >= flushRows {
			if err := flush(); err != nil {
				return res, err
			}
		}
		if (i+1)%100 == 0 {
			cfg.log.Info("backfill progress", "contracts", i+1, "of", len(targets), "rows", res.Rows)
		}
	}
	if err := flush(); err != nil {
		return res, err
	}
	cfg.log.Info("backfill done", "contracts", res.Fetched, "failed", res.Failed,
		"rows", res.Rows, "inserted", res.Inserted, "from", res.FirstDate, "to", res.LastDate)
	return res, nil
}

func (r *Result) fail(err error) {
	r.Failed++
	if r.FirstErrMsg == "" {
		r.FirstErrMsg = err.Error()
	}
}

// targetsOf lists every call and put in the chain once, sorted by symbol so a
// limited run is reproducible.
func targetsOf(chain []datasource.OptionRow) []target {
	seen := map[string]bool{}
	var out []target
	for _, row := range chain {
		for _, leg := range []*datasource.OptionLeg{row.Call, row.Put} {
			if leg == nil || leg.InsCode == "" || leg.Symbol == "" || seen[leg.InsCode] {
				continue
			}
			seen[leg.InsCode] = true
			out = append(out, target{
				insCode: leg.InsCode, symbol: leg.Symbol,
				underlyingSymbol: row.UnderlyingSymbol, underlyingIns: row.UnderlyingInsCode,
				optionType: leg.Type, strike: row.StrikePrice, expiry: row.ExpiryDate,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].symbol < out[j].symbol })
	return out
}

// lastPrice is the day's last traded price, falling back to the settlement price.
func lastPrice(d datasource.DailyPrice) int64 {
	if d.Close > 0 {
		return d.Close
	}
	return d.Final
}

// rowsOf turns one contract's history into snapshot rows. Days from today on,
// days after the expiry and days on which the contract did not trade are dropped.
func rowsOf(t target, hist []datasource.DailyPrice, under map[string]int64, today string, capturedAt time.Time) []snapshot.Row {
	expiry := t.expiry.In(datasource.Tehran).Format(dateLayout)
	rows := make([]snapshot.Row, 0, len(hist))
	for _, d := range hist {
		date := d.Date.In(datasource.Tehran).Format(dateLayout)
		if date >= today || date > expiry {
			continue
		}
		if d.Volume == 0 && d.Close == 0 && d.Final == 0 {
			continue
		}
		r := snapshot.Row{
			TradeDate:        date,
			ContractSymbol:   t.symbol,
			UnderlyingSymbol: t.underlyingSymbol,
			OptionType:       string(t.optionType),
			Strike:           t.strike,
			Expiry:           expiry,
			ClosePrice:       d.Final, // settlement price, like the live snapshot's ClosePrice
			LastPrice:        d.Close,
			Volume:           d.Volume,
			Value:            d.Value,
			TradeCount:       d.TradeCount,
			CapturedAt:       capturedAt,
		}
		if p, ok := under[date]; ok {
			r.UnderlyingPrice = &p
		}
		rows = append(rows, r)
	}
	return rows
}
