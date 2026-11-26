# Task 18: Backtest results page

Owner role: Front (done by the person who owns task 8, because the page shows their results). PR: #10 (`feat/front-mvp`).
Depends on: task 11 (backtest results API; not built). The page runs on mock data now, which is **real output of the backtest engine** (PR #9).

## What the task was
Pick an underlying and a template, then show metrics, the equity curve and per-expiry results.

## What the page does (`/backtests?underlying=&template=`)
- Selectors for underlying and template (the template's legs are shown as text).
- Calls `GET /backtests?underlying=&template=` and shows the first run (a selector appears if there are several, for example different entry days). Then `GET /backtests/:id`.
- **Notice:** if `metrics.modelPricedShare > 0` a yellow note says that X% of trades are priced with a Black-Scholes model at 20-day historical volatility, because past option prices are unavailable, so the result shows how the strategy behaves on model prices, not real market profit. Fees are approximate.
- **Metrics cards:** number of trades (and skipped expiries), win rate, average P&L per trade (rial, after fees), total return, max drawdown, profit factor, total fees.
- **Equity curve:** step chart starting at 1.0, Jalali dates, with a note that each trade risks the run's `allocation` of the equity, without compounding.
- **Per-expiry table:** expiry, entry, exit (with reason when it is a target or stop), legs (for example `+C700 -C725`), underlying price entry to exit, entry cost, P&L, return on risk, price source (market price, model, mixed). Newest first.
- **Skipped expiries** in a collapsible list with the reason (no listed chain, price below the minimum premium, price jump, and so on).
- States: empty prompt until both are chosen, loading skeleton, empty ("no backtest for this combination"), error with retry.

## Files
| File | |
|---|---|
| `frontend/src/pages/Backtest/BacktestPage.tsx` | the page |
| `frontend/src/types/backtestTypes.ts` | `BacktestSummary`, `BacktestDetail`, `BacktestTrade`, `BacktestMetrics`, `EquityPoint`, `SkippedExpiry` |
| `frontend/src/services/marketService.ts` | `getBacktests`, `getBacktest` (mock/real switch; `/api/v1/backtests`) |
| `frontend/src/hooks/useMarketQueries.ts` | `useBacktests`, `useBacktest` |
| `frontend/src/mocks/mockBacktest.ts` | mock list and detail |
| `frontend/src/mocks/backtestRuns.json` | 36 runs (4 underlyings x 9 templates), the last 26 weekly expiries each, produced by `backtest/scripts/export_api_runs.py`; lazy-loaded chunk of about 440 kB (47 kB gzipped), only used in mock mode |
| `frontend/src/components/Custom/charts/LineChart.tsx` | the equity chart (step mode) |

## The API shape the page expects
This is **not in `docs/api-contract.md`** (its section 3 says strategy backtests do not exist). It follows `docs/strategy-backtest.md` (from the engine PR #9):
- `GET /backtests?underlying=&template=&limit=` returns `{count, items[]}`; an item has `id, modelName ("strategy:<template>"), template, underlying, startDate, endDate, winRate, maxDrawdown, totalReturn, tradeCount, avgPnl, modelPricedShare, createdAt`.
- `GET /backtests/:id` adds `params`, `metrics`, `equityCurve [{date, equity}]`, `trades[]`, `skipped[]`.
Units: `winRate` and `totalReturn` are fractions, `maxDrawdown` a positive fraction, money in rial per position, `equity` starts at 1.0.

## For Back (task 11) and Data (task 9)
- Serve exactly those shapes (camelCase, nulls never omitted); the JSON in `backtestRuns.json` is a complete example of every field.
- `backtest_runs` needs columns for `metrics` and `trades` (suggested migration `000010`, see `docs/strategy-backtest.md`).
- Until real past expiries exist in `quote_snapshots`, every run is model-priced; keep `modelPricedShare` honest so the notice shows.

## Verified
`tsc -b`, build; rendered in headless Chrome on mock data (underlying خودرو, template bull-call-spread: 20 trades, win rate 45%, 6 skipped). Not run against a real API.

## Known limits
- The page shows the first of several runs by default.
- No pagination of the trades table (about 20 to 60 rows per run).
- Chart x-axis has about 6 date labels.
