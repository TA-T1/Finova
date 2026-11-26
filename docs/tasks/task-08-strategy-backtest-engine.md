# Task 8: Strategy backtest engine

Owner role: Data. PR: #9 (`feat/data-backtest-engine`, base `main`).
Depends on: task 3 (template definitions; the draft list in `docs/api-contract.md` was used), softly task 7. Unblocks: task 9 (hard), task 10 (soft), and through 9: tasks 11 and 18.

All files are new; no existing file was changed. Everything is under `backtest/`, plus `docs/strategy-backtest.md`.

## Purpose
Run a rule such as "30 days before expiry, open a bull call spread one strike apart, hold to expiry" on the past expiries of one underlying and measure it: per-expiry results, win rate, average P&L, max drawdown, equity curve.

## Files
| File | What it holds |
|---|---|
| `backtest/finova_backtest/pricing.py` | Black-Scholes (`bs_price`), `historical_volatility`, `hv20`, `intrinsic`. Same conventions as the Go package `internal/options`. |
| `backtest/finova_backtest/templates.py` | `StrikeRule` (`atm`, `otm n`, `itm n`), `LegSpec`, `Template`, the 9 `TEMPLATES`, `pick_strike`. |
| `backtest/finova_backtest/data.py` | `Quote`, `MarketData` (in-memory index), `load_dir` (demo dataset files), `load_db` (Postgres, needs `psycopg`), `load_prices_csv` (adjusted prices), `with_daily`, `max_abs_log_return`. |
| `backtest/finova_backtest/engine.py` | `Rule`, `Fees`, `Trade`, `Skipped`, `Result`; `run_backtest`; payoff bounds; fees; exits; metrics; equity curve; `to_backtest_run`; `expiries_weekly`, `completed_expiries`. |
| `backtest/finova_backtest/cli.py`, `__main__.py` | `python -m finova_backtest templates` and `... run`. |
| `backtest/finova_backtest/__init__.py` | Public exports. |
| `backtest/scripts/export_api_runs.py` | Runs every template on given underlyings and writes results in the API shape (camelCase). Produced the front end's `backtestRuns.json`. |
| `backtest/tests/` | `test_pricing.py`, `test_templates.py`, `test_engine.py`, `test_data.py`, `conftest.py` (33 tests, no database needed). |
| `backtest/README.md`, `requirements.txt`, `.gitignore` | Usage and assumptions; deps (`psycopg` only for DB loading, `pytest` for tests; the engine itself uses the standard library). |
| `docs/strategy-backtest.md` | Proposal for storing results in `backtest_runs` and serving them (tasks 9, 11, 18). |

## The rule (`Rule`)
| Field | Default | Meaning |
|---|---|---|
| `template_id` | required | one of the 9 templates |
| `entry_days` | 30 | calendar days before expiry; entry is the last trading day on or before that date, at most `max_entry_gap_days` (5) earlier, else the expiry is skipped |
| `quantity` | 1 | contracts per leg unit; a share leg is `ratio x quantity x contract_size` shares |
| `exit` | `expiry` | or `target_stop`: close when open P&L reaches `target_pct` or `-stop_pct` of the capital at risk, checked at each day's close |
| `fill` | `last` | `last` (last price, else settlement), `mid`, or `cross` (buy at ask, sell at bid); mid/cross fall back to last when the row has no bid/ask |
| `allocation` | 0.05 | fraction of starting equity risked per trade |
| `contract_size` | 1000 | |
| `rate` | 0.30 | risk-free rate for model prices |
| `allow_model` | true | allow Black-Scholes prices where history has none |
| `min_premium` | 1.0 rial | legs priced below this are not tradable; the expiry is skipped |
| `max_jump` | 0.12 | skip the trade if the underlying moves more than this (log return) in one day while it is open (capital increase / dividend in raw data) |
| `fees` | `Fees()` | see below |

Invalid rules raise `ValueError`.

## Templates
`long-call`, `long-put`, `covered-call`, `protective-put`, `collar`, `bull-call-spread`, `bear-put-spread`, `long-straddle`, `long-strangle`. These are the draft list of `docs/api-contract.md` 2.3, implemented as written. Strikes are picked among the strikes **listed** for the expiry:
- `atm`: the listed strike closest to the underlying price (the lower one on a tie);
- `otm n`: n listed strikes further out of the money than ATM (up for a call, down for a put);
- `itm n`: the opposite.
**Task 3 still needs Data to confirm this list** (names, legs, the "n listed strikes" rule). If it changes, change `templates.py` and the Go side together.

## How one expiry is processed (`_run_one`)
1. Entry day and underlying price (snapshot price, else daily close). No data: skipped with a reason.
2. Listed strikes: from `quote_snapshots` on the entry day; otherwise (if `allow_model`) a model strike grid: the step keeps the same proportion of the price as today's listed strikes, rounded to 1, 2, 2.5 or 5 times a power of ten.
3. Legs are priced: snapshot row first (`price_source = snapshot`), else Black-Scholes at HV20 of the entry day (`model`). Mixed trades are `mixed`.
4. Payoff bounds (max profit, max loss) are computed from the piecewise-linear payoff. Capital at risk = max loss (falls back to the net premium if the loss is unlimited).
5. Exit: at expiry (settlement at the underlying's close on the expiry day, or the last trading day before it, with intrinsic values) or earlier on target/stop using that day's marks (snapshot row else model).
6. Trades whose holding period contains a price jump above `max_jump`, or with a leg below `min_premium`, are skipped with a reason.

## Fees (`Fees`)
Proportional. **Placeholders near the Tehran tariff, not a broker's numbers.** Option commission 0.1% of premium value per trade; exercise fee 0.1% of strike value for each option that settles in the money (long or short); shares 0.4% to buy and 0.9% to sell. Held to expiry, options are not traded again, so only the exercise fee applies. A share leg is valued as if sold at the settlement price.

## Results
- Per trade (`Trade`): expiry, entry/exit dates and reason, spots, legs with entry/exit prices and price source, `net_premium` (positive = debit), `capital_at_risk`, `max_profit`, `max_loss`, `gross_pnl`, `fees`, `pnl`, `return_on_risk`, `price_source`, `chain_source`.
- Metrics: trades, skipped, `win_rate`, `avg_pnl`, `avg_return_on_risk`, `total_pnl`, `total_fees`, best/worst, `profit_factor` (null with no loss), `max_drawdown` (positive fraction), `total_return`, `model_priced_share`.
- Equity curve: starts at 1.0 at the first entry; each trade adds `allocation x return_on_risk` (capped at losing its whole allocation) in the order trades close; the account cannot go below 0. **No compounding**: weekly expiries overlap, and compounding overlapping trades would count the same money twice. Do not read the curve as one account's balance.
- `to_backtest_run(result)` returns a dict with the `backtest_runs` columns (`model_name = "strategy:<template>"`, `params`, `start_date`, `end_date`, `win_rate`, `max_drawdown`, `total_return`, `equity_curve` as `[{date, equity}]`) plus `metrics`, `trades`, `skipped`.

## Data inputs
- `load_dir(dir, prices_csv=None)`: the demo dataset of task 7 (`quote_snapshots.csv.gz`, `underlyings.csv.gz`, `underlying_daily_prices.csv.gz`).
- `load_db(url)`: the same tables from Postgres.
- `load_prices_csv(path)`: the back-adjusted prices of `data-pipeline/output/underlyings_daily.csv` (`adj_last`, `adj_final`). **Recommended** for model prices: raw prices have capital-increase jumps that corrupt HV20.

## CLI
```bash
cd backtest
python -m finova_backtest templates
python -m finova_backtest run --data-dir ../backend/testdata/demo \
    --prices-csv ../data-pipeline/output/underlyings_daily.csv \
    --underlying خودرو --template bull-call-spread --entry-days 30 \
    --expiries weekly --out run.json
python -m pytest
```
`--expiries observed` uses expiries seen in `quote_snapshots` that have passed (none exist yet, the command says so and exits 3). `--expiries weekly` uses a synthetic Wednesday calendar, by default the last three years (`--from`, `--to`). Other flags: `--exit target_stop --target 0.5 --stop 0.5`, `--fill`, `--allocation`, `--no-model`, `--quantity`.

## Tests (33)
- Pricing: textbook values (S=K=100, T=1, 20%, r=5%: call 10.4506, put 5.5735), put-call parity, expiry value, HV by hand, jump filtering.
- Templates: ATM tie, otm/itm directions, grid limits, catalogue.
- Engine: **bull call spread by hand** (buy 2000 @150, sell 2100 @80: debit 70,000, max profit 30,000, max loss 70,000, settle 2150 gives +30,000; fees 4,330 give +25,670; settle 1950 gives -70,000), payoff bounds, straddle, covered call, model fallback equals Black-Scholes at HV20, `allow_model=False` skips, target exit and stop exit with hand numbers, equity/drawdown, empty result, price-jump skip, minimum premium, account floor, rule validation, weekly calendar.
- Data: loading a backfill export directory.

## Findings and limits (important for tasks 9, 10, 11, 18)
1. **No past expiry exists in the data.** `quote_snapshots` holds only contracts listed today; expired ones cannot be backfilled. Until the daily snapshot has run past expiries, the only way to backtest is `--expiries weekly`, where **every price is a model price**. Results then describe a strategy on Black-Scholes prices at HV20 (no volatility smile, no real spreads or liquidity), not real quotes. `metrics.model_priced_share` and each trade's `price_source` say so, and the front end shows a notice.
2. Settlement uses the underlying's close, while the real exchange settles on an average price.
3. Backfilled rows have no bid/ask; `fill = mid/cross` equals `last` on them.
4. Fees are placeholders.
5. Sample results on model prices vary widely by strategy and underlying; they were not tuned and should not be quoted as findings without the report (task 10).

## What task 9 must do
- Choose the templates and underlyings, run `run_backtest`, and store `to_backtest_run(...)` rows.
- `backtest_runs` has no columns for `metrics` and `trades`. `docs/strategy-backtest.md` suggests migration `000010_backtest_run_details` (`ALTER TABLE backtest_runs ADD COLUMN metrics JSONB, ADD COLUMN trades JSONB`) or a `backtest_trades` table. This must land first.
- Write `equity_curve` exactly as `[{date, equity}]` starting at 1.0; `win_rate` and `total_return` are fractions; `max_drawdown` is a positive fraction.

## Open items
- Confirm the template list with Back (task 3), and keep Go (tasks 12, 13) and Python in sync.
- `docs/api-contract.md` section 3 says strategy backtests do not exist; it needs updating with task 11.
- Fees and the settlement rule should be replaced with real values if they are known.
