package market

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"Finova/internal/datasource"
)

// Collector pulls data from a datasource.Source and persists it via Repository.
type Collector struct {
	src  datasource.Source
	repo *Repository
	log  *slog.Logger
}

// NewCollector wires a source and repository together. A nil logger is replaced
// with the default slog logger.
func NewCollector(src datasource.Source, repo *Repository, log *slog.Logger) *Collector {
	if log == nil {
		log = slog.Default()
	}
	return &Collector{src: src, repo: repo, log: log}
}

// CollectResult summarises one option-chain collection pass.
type CollectResult struct {
	Source      string    `json:"source"`
	TS          time.Time `json:"ts"`
	Rows        int       `json:"rows"`
	Underlyings int       `json:"underlyings"`
	Contracts   int       `json:"contracts"`
	Quotes      int       `json:"quotes"`
	Errors      int       `json:"errors"`
	FirstError  string    `json:"firstError,omitempty"`
}

// CollectOptionChain fetches the full live option chain and upserts underlyings,
// contracts and quotes. Per-row failures are counted and skipped rather than
// aborting the whole pass; a failure to fetch the chain at all is returned.
func (c *Collector) CollectOptionChain(ctx context.Context) (CollectResult, error) {
	res := CollectResult{Source: c.src.Name(), TS: time.Now().Truncate(time.Minute)}

	rows, err := c.src.OptionMarketWatch(ctx)
	if err != nil {
		return res, fmt.Errorf("collect option chain: %w", err)
	}
	res.Rows = len(rows)

	underlyingIDs := make(map[string]int)      // insCode -> id
	quotedUnderlyings := make(map[string]bool) // insCode -> quote written this pass

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
			if err := c.repo.InsertUnderlyingQuote(ctx, uid, res.TS, row.UnderlyingLast, row.UnderlyingClose); err != nil {
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
		}
	}

	c.log.Info("option chain collected",
		"source", res.Source, "rows", res.Rows, "underlyings", res.Underlyings,
		"contracts", res.Contracts, "quotes", res.Quotes, "errors", res.Errors)
	return res, nil
}

// ensureUnderlying returns the id of the underlying with insCode, creating it if
// necessary. It tries to enrich identity (symbol/ISIN/name) via InstrumentInfo,
// falling back to the symbol seen in the option chain if that lookup fails.
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
	return c.repo.UpsertUnderlying(ctx, info)
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
