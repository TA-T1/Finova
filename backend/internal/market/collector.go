package market

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"Finova/internal/datasource"
	"Finova/internal/options"
)

// Collector pulls data from a datasource.Source and persists it via Repository.
type Collector struct {
	src  datasource.Source
	repo *Repository
	log  *slog.Logger
	rate float64 // annual risk-free rate for the calculation layer
}

// CollectorOption customises a Collector.
type CollectorOption func(*Collector)

// WithRiskFreeRate sets the annual risk-free rate used by the calculation layer,
// as a decimal fraction (0.30 = 30%).
func WithRiskFreeRate(rate float64) CollectorOption {
	return func(c *Collector) { c.rate = rate }
}

// NewCollector wires a source and repository together. A nil logger is replaced
// with the default slog logger.
func NewCollector(src datasource.Source, repo *Repository, log *slog.Logger, opts ...CollectorOption) *Collector {
	if log == nil {
		log = slog.Default()
	}
	c := &Collector{src: src, repo: repo, log: log, rate: DefaultRiskFreeRate}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CollectResult summarises one option-chain collection pass.
type CollectResult struct {
	Source      string    `json:"source"`
	TS          time.Time `json:"ts"`
	Rows        int       `json:"rows"`
	Underlyings int       `json:"underlyings"`
	Contracts   int       `json:"contracts"`
	Quotes      int       `json:"quotes"`
	Metrics     int       `json:"metrics"` // calculated metric rows stored
	Errors      int       `json:"errors"`
	FirstError  string    `json:"firstError,omitempty"`
}

// CollectOptionChain fetches the full live option chain and upserts underlyings,
// contracts and quotes, then calculates and stores the option metrics for the
// quotes it wrote (see docs/calculation-layer.md). Per-row failures are counted
// and skipped rather than aborting the whole pass; a failure to fetch the chain
// at all is returned.
func (c *Collector) CollectOptionChain(ctx context.Context) (CollectResult, error) {
	res := CollectResult{Source: c.src.Name(), TS: time.Now()}
	if !MarketOpenAt(res.TS) {
		return res, ErrMarketClosed
	}

	rows, err := c.src.OptionMarketWatch(ctx)
	if err != nil {
		return res, fmt.Errorf("collect option chain: %w", err)
	}
	res.Rows = len(rows)

	underlyingIDs := make(map[string]int)      // insCode -> id
	quotedUnderlyings := make(map[string]bool) // insCode -> quote written this pass
	var metricJobs []metricInput               // contracts whose quote was stored this pass

	note := func(err error) {
		if err == nil {
			return
		}
		res.Errors++
		if res.FirstError == "" {
			res.FirstError = err.Error()
		}
	}

	for _, row := range rows {
		if row.UnderlyingInsCode == "" {
			continue
		}

		uid, ok := underlyingIDs[row.UnderlyingInsCode]
		if !ok {
			uid, err = c.ensureUnderlying(ctx, row.UnderlyingInsCode, row.UnderlyingSymbol)
			if err != nil {
				note(fmt.Errorf("underlying %s: %w", row.UnderlyingInsCode, err))
				continue
			}
			underlyingIDs[row.UnderlyingInsCode] = uid
			res.Underlyings++
		}

		if !quotedUnderlyings[row.UnderlyingInsCode] {
			if err := c.repo.InsertUnderlyingQuote(ctx, uid, res.TS, row.UnderlyingLast, row.UnderlyingClose, row.UnderlyingYesterday); err != nil {
				note(fmt.Errorf("underlying quote %s: %w", row.UnderlyingInsCode, err))
			} else {
				quotedUnderlyings[row.UnderlyingInsCode] = true
			}
		}

		for _, leg := range []*datasource.OptionLeg{row.Call, row.Put} {
			if leg == nil {
				continue
			}
			cid, err := c.repo.UpsertContract(ctx, ContractParams{
				InsCode:      leg.InsCode,
				Symbol:       leg.Symbol,
				UnderlyingID: uid,
				Type:         leg.Type,
				StrikePrice:  row.StrikePrice,
				ExpiryDate:   row.ExpiryDate,
				ContractSize: row.ContractSize,
			})
			if err != nil {
				note(fmt.Errorf("contract %s: %w", leg.Symbol, err))
				continue
			}
			res.Contracts++

			if err := c.repo.InsertOptionQuote(ctx, cid, res.TS, *leg); err != nil {
				note(fmt.Errorf("quote %s: %w", leg.Symbol, err))
				continue
			}
			res.Quotes++

			if in, ok := newMetricInput(cid, uid, row, *leg); ok {
				metricJobs = append(metricJobs, in)
			}
		}
	}

	res.Metrics = c.storeMetrics(ctx, res.TS, metricJobs, note)

	c.log.Info("option chain collected",
		"source", res.Source, "rows", res.Rows, "underlyings", res.Underlyings,
		"contracts", res.Contracts, "quotes", res.Quotes, "metrics", res.Metrics,
		"errors", res.Errors)
	return res, nil
}

// newMetricInput builds the calculation input for one stored quote. It reports
// false when the contract cannot be calculated at all: an unknown option type,
// or no usable underlying price or strike.
func newMetricInput(contractID, underlyingID int, row datasource.OptionRow, leg datasource.OptionLeg) (metricInput, bool) {
	kind, err := options.ParseKind(string(leg.Type))
	if err != nil {
		return metricInput{}, false
	}
	spot := pickSpot(float64(row.UnderlyingLast), float64(row.UnderlyingClose))
	strike := float64(row.StrikePrice)
	if spot <= 0 || strike <= 0 {
		return metricInput{}, false
	}
	return metricInput{
		ContractID:   contractID,
		UnderlyingID: underlyingID,
		Kind:         kind,
		Strike:       strike,
		Expiry:       row.ExpiryDate,
		Spot:         spot,
		Last:         float64(leg.Last),
		Bid:          float64(leg.BestBid),
		Ask:          float64(leg.BestAsk),
		Close:        float64(leg.Close),
	}, true
}

// storeMetrics calculates the metrics of every job and stores them. It never
// fails the pass: problems are reported through note, and the number of rows
// actually stored is returned. The quotes were already written by then.
func (c *Collector) storeMetrics(ctx context.Context, ts time.Time, jobs []metricInput, note func(error)) int {
	if len(jobs) == 0 {
		return 0
	}
	hv := c.historicalVolatilities(ctx)

	rows := make([]OptionMetrics, 0, len(jobs))
	for _, job := range jobs {
		if v, ok := hv[job.UnderlyingID]; ok {
			job.HV = &v
		}
		m, err := computeMetrics(job, ts, c.rate)
		if err != nil {
			note(fmt.Errorf("metrics contract %d: %w", job.ContractID, err))
			continue
		}
		rows = append(rows, m)
	}

	if err := c.repo.UpsertOptionMetrics(ctx, rows); err != nil {
		note(fmt.Errorf("store metrics: %w", err))
		return 0
	}
	return len(rows)
}

// historicalVolatilities returns the HV20 of each underlying that has a full
// window of daily prices, keyed by underlying id. Underlyings without enough
// history are left out, so their contracts get no theoretical price. A failure
// to load prices is logged and treated the same way: the rest of the metrics
// do not depend on volatility history.
func (c *Collector) historicalVolatilities(ctx context.Context) map[int]float64 {
	closes, err := c.repo.RecentCloses(ctx, options.HVWindow+1)
	if err != nil {
		c.log.Warn("historical volatility unavailable: loading daily prices failed", "err", err)
		return nil
	}
	out := make(map[int]float64, len(closes))
	for id, series := range closes {
		if len(series) != options.HVWindow+1 {
			continue
		}
		v, err := options.HV20(series)
		if err != nil {
			c.log.Warn("historical volatility skipped", "underlyingId", id, "err", err)
			continue
		}
		out[id] = v
	}
	return out
}

// ensureUnderlying returns the id of the underlying with insCode, creating it if
// necessary. It tries to enrich identity (symbol/ISIN/name/sector) via
// InstrumentInfo, falling back to the symbol seen in the option chain if that
// lookup fails. The sector drives the stock/fund/index/other classification.
func (c *Collector) ensureUnderlying(ctx context.Context, insCode, fallbackSymbol string) (int, error) {
	if id, ok, err := c.repo.GetUnderlyingIDByInsCode(ctx, insCode); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	info := datasource.InstrumentInfo{InsCode: insCode, Symbol: fallbackSymbol}
	if full, err := c.src.InstrumentInfo(ctx, insCode); err == nil && full != nil {
		info = *full
		if info.Symbol == "" {
			info.Symbol = fallbackSymbol
		}
	} else if err != nil && err != datasource.ErrNotSupported {
		c.log.Warn("instrument info lookup failed; using chain symbol",
			"insCode", insCode, "err", err)
	}
	// Classification needs sector info; without it classifyUnderlying yields
	// KindOther, which UpsertUnderlying will not use to overwrite a known kind.
	return c.repo.UpsertUnderlying(ctx, info, classifyUnderlying(info))
}

// BackfillDaily fetches the end-of-day history for an underlying instrument and
// upserts it. The instrument is created if it is not already known.
func (c *Collector) BackfillDaily(ctx context.Context, insCode string, adjusted bool) (int, error) {
	uid, err := c.ensureUnderlying(ctx, insCode, "")
	if err != nil {
		return 0, err
	}
	prices, err := c.src.DailyPrices(ctx, insCode, adjusted)
	if err != nil {
		return 0, fmt.Errorf("daily prices %s: %w", insCode, err)
	}
	n := 0
	for _, p := range prices {
		if err := c.repo.UpsertDailyPrice(ctx, uid, p); err != nil {
			return n, fmt.Errorf("upsert daily %s %s: %w", insCode, p.Date.Format("2006-01-02"), err)
		}
		n++
	}
	c.log.Info("daily history backfilled", "insCode", insCode, "days", n, "adjusted", adjusted)
	return n, nil
}

// Run collects the option chain immediately and then on every tick of interval,
// until ctx is cancelled. Intended to be launched in its own goroutine.
func (c *Collector) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	c.log.Info("market collector started", "source", c.src.Name(), "interval", interval.String())

	tick := func() {
		if !MarketOpenAt(time.Now()) {
			return
		}
		cctx, cancel := context.WithTimeout(ctx, interval)
		defer cancel()
		if _, err := c.CollectOptionChain(cctx); err != nil {
			c.log.Error("scheduled collection failed", "err", err)
		}
	}

	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			c.log.Info("market collector stopped")
			return
		case <-t.C:
			tick()
		}
	}
}
