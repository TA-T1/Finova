# Option calculation layer

After every collection pass, `market.Collector` calculates a set of metrics for
each contract it just stored a quote for and writes them to `option_metrics`
(migration `000009`). The maths lives in `internal/options` (pure, no database);
`internal/market/metrics.go` turns a stored quote into its inputs, and
`internal/market/metrics_store.go` writes the rows.

```
OptionMarketWatch -> upsert contract + quote -> metricInput
                                                   |  + HV20 (underlying_daily_prices)
                                                   v
                              options.Evaluate -> OptionMetrics -> option_metrics
```

The pass result of `POST /market/collect/options` has a new `metrics` field:
the number of metric rows stored. A metrics failure is counted in `errors` and
`firstError`; it never fails the pass, because the quotes are already written.

## What is stored

One row per `(contract_id, ts)`, where `ts` is the same timestamp as the
contract's quote in `option_quotes`. `option_metrics_latest` gives the newest row
per contract, like `option_quotes_latest`. Money values are per share, not per
contract (multiply by `contract_size`).

| Column | Meaning |
|---|---|
| `underlying_price` | spot used (S) |
| `days_to_expiry` | calendar days to expiry, Tehran dates; `0` on expiry day |
| `risk_free_rate` | rate used (r) |
| `market_price`, `price_source` | option price used, and where it came from |
| `moneyness` | `ITM`, `ATM` (strike within 0.5% of spot) or `OTM` |
| `intrinsic_value`, `time_value` | `max(S-K, 0)` for a call, `max(K-S, 0)` for a put; premium minus intrinsic |
| `break_even`, `break_even_percent` | `K + premium` (call) or `K - premium` (put); move needed from spot, in percent |
| `leverage`, `effective_leverage` | `S / premium`; `delta * leverage` |
| `implied_vol` | volatility that reproduces `market_price` |
| `historical_vol` | HV20 of the underlying |
| `greeks_vol` | volatility the Greeks were evaluated at |
| `delta`, `gamma` | raw model units |
| `vega_per_pct`, `theta_per_day`, `rho_per_pct` | per +1 volatility point, per calendar day, per +1 rate point |
| `theoretical_price` | Black-Scholes at HV20 |
| `price_gap`, `price_gap_percent` | `market_price - theoretical_price`, and that over the theoretical price in percent |

Volatilities and rates are decimal fractions (`0.30` = 30%). Columns ending in
`_percent` are percentages (`2.5` = 2.5%).

## Rules

**Option price (`market_price`).** The last traded price if it is above zero;
otherwise the middle of the best bid and ask, if both exist and the book is not
crossed; otherwise the closing price. `price_source` is `last`, `mid` or `close`.
With none of them, every price-based column is `NULL`.

**Spot.** The underlying's last price, otherwise its closing price. The previous
day's close is not used: a stale spot would make moneyness and the Greeks look
valid when they are not. A contract with no spot is skipped.

**Time to expiry.** Whole Tehran calendar days divided by 365 (`options.DaysPerYear`).

**Implied volatility.** Newton with a bisection fallback (`options.ImpliedVolatility`).
It is `NULL` when the price is outside the no-arbitrage bounds (stale or crossed
quote), when the price is at or below intrinsic value (the solver returns 0), or
when the contract expires today.

**Greeks.** Evaluated at the implied volatility. If there is none, at HV20. If
there is neither, they are `NULL`: the engine would return step-function values
that look real. On expiry day delta is the step function (0, 1 or ±1) and the
others are 0.

**HV20.** Sample standard deviation of the last 20 daily log returns, times
`sqrt(240)` (`options.HV20`; 240 is the approximate number of Tehran trading
days a year). It uses 21 closes: `final_price` from `underlying_daily_prices`
(`close` when it is missing), completed days only. Fewer than 21 stored days
means no HV20, so no theoretical price and no fallback for the Greeks. It is
computed once per pass for all underlyings with one query.

**Theoretical price.** Black-Scholes at HV20. `price_gap` and `price_gap_percent`
need both it and a market price.

## Configuration

| Var | Default | Meaning |
|---|---|---|
| `RISK_FREE_RATE` | `0.30` | annual risk-free rate as a decimal fraction. Values outside `[0, 1)` are rejected at startup, which catches `30` typed instead of `0.30` |

## Things to be aware of

- **Adjusted prices.** `underlying_daily_prices` has no raw/adjusted flag; what is
  in it is whatever `POST /market/collect/daily/:insCode` last stored. A raw
  series contains one-day jumps at capital increases and dividends, and HV20
  would read them as volatility for the next 20 trading days. Backfill with
  `?adjusted=true`.
- **Volume.** About 1,800 contracts per pass add 1,800 rows to `option_metrics`
  per pass, the same as `option_quotes`. Plan retention for both together.
- **`last_price == 0`.** The code treats a zero last price as "no trade today"
  and falls back to the book. This has not been checked against live data on a
  day with an untraded contract.

## Checking it

```sql
SELECT c.symbol, m.price_source, m.days_to_expiry, m.moneyness,
       m.implied_vol, m.historical_vol, m.delta, m.theoretical_price
FROM option_metrics_latest m
JOIN option_contracts c ON c.id = m.contract_id
ORDER BY c.symbol
LIMIT 20;
```
