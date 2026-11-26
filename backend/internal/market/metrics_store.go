package market

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// metricColumns lists the option_metrics columns in the order metricArgs
// returns their values. The first two form the primary key.
var metricColumns = []string{
	"contract_id", "ts",
	"underlying_price", "days_to_expiry", "risk_free_rate", "market_price", "price_source",
	"moneyness", "intrinsic_value", "time_value", "break_even", "break_even_percent",
	"leverage", "effective_leverage",
	"implied_vol", "historical_vol", "greeks_vol",
	"delta", "gamma", "vega_per_pct", "theta_per_day", "rho_per_pct",
	"theoretical_price", "price_gap", "price_gap_percent",
}

// upsertMetricsSQL is built from metricColumns so the column list, the
// placeholders and the ON CONFLICT update cannot drift apart.
var upsertMetricsSQL = func() string {
	placeholders := make([]string, len(metricColumns))
	for i := range metricColumns {
		placeholders[i] = "$" + strconv.Itoa(i+1)
	}
	updates := make([]string, 0, len(metricColumns)-2)
	for _, c := range metricColumns[2:] {
		updates = append(updates, c+" = EXCLUDED."+c)
	}
	return "INSERT INTO option_metrics (" + strings.Join(metricColumns, ", ") + ")\n" +
		"VALUES (" + strings.Join(placeholders, ", ") + ")\n" +
		"ON CONFLICT (contract_id, ts) DO UPDATE SET " + strings.Join(updates, ", ")
}()

// metricArgs returns the statement arguments of m, in metricColumns order. A
// nil pointer is sent as NULL.
func metricArgs(m *OptionMetrics) []any {
	return []any{
		m.ContractID, m.TS,
		m.UnderlyingPrice, m.DaysToExpiry, m.RiskFreeRate, m.MarketPrice, m.PriceSource,
		m.Moneyness, m.IntrinsicValue, m.TimeValue, m.BreakEven, m.BreakEvenPercent,
		m.Leverage, m.EffectiveLeverage,
		m.ImpliedVol, m.HistoricalVol, m.GreeksVol,
		m.Delta, m.Gamma, m.VegaPerPct, m.ThetaPerDay, m.RhoPerPct,
		m.TheoreticalPrice, m.PriceGap, m.PriceGapPercent,
	}
}

// metricsBatchSize bounds how many statements go to Postgres in one round trip.
const metricsBatchSize = 500

// UpsertOptionMetrics writes calculated metrics. Re-running a pass with the
// same timestamp overwrites its rows. Rows are sent in batches, which is far
// fewer round trips than one Exec per contract (a pass has about 1,800).
func (r *Repository) UpsertOptionMetrics(ctx context.Context, rows []OptionMetrics) error {
	for start := 0; start < len(rows); start += metricsBatchSize {
		end := min(start+metricsBatchSize, len(rows))

		batch := &pgx.Batch{}
		for i := start; i < end; i++ {
			batch.Queue(upsertMetricsSQL, metricArgs(&rows[i])...)
		}

		results := r.db.SendBatch(ctx, batch)
		for i := start; i < end; i++ {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return fmt.Errorf("upsert metrics for contract %d: %w", rows[i].ContractID, err)
			}
		}
		if err := results.Close(); err != nil {
			return fmt.Errorf("upsert metrics batch: %w", err)
		}
	}
	return nil
}

// RecentCloses returns, for each underlying, its n most recent closing prices
// from underlying_daily_prices, oldest first. Underlyings with fewer than n
// stored days are returned with what they have; the caller decides whether
// that is enough.
//
// The price is final_price (the settlement price the data team treats as the
// canonical close), falling back to close when it is missing. Only completed
// days are used (before today in Tehran), so the series, and the volatility
// computed from it, do not change during the session.
func (r *Repository) RecentCloses(ctx context.Context, n int) (map[int][]float64, error) {
	const q = `
		SELECT underlying_id, price
		FROM (
			SELECT p.underlying_id, p.trade_date,
			       COALESCE(NULLIF(p.final_price, 0), p.close)::float8 AS price,
			       ROW_NUMBER() OVER (
			           PARTITION BY p.underlying_id ORDER BY p.trade_date DESC
			       ) AS rn
			FROM underlying_daily_prices p
			WHERE COALESCE(NULLIF(p.final_price, 0), p.close) > 0
			  AND p.trade_date < (now() AT TIME ZONE 'Asia/Tehran')::date
		) recent
		WHERE rn <= $1
		ORDER BY underlying_id, trade_date`
	rows, err := r.db.Query(ctx, q, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int][]float64)
	for rows.Next() {
		var id int
		var price float64
		if err := rows.Scan(&id, &price); err != nil {
			return nil, err
		}
		out[id] = append(out[id], price)
	}
	return out, rows.Err()
}
