# Strategy backtest engine

Runs a rule such as *"30 days before expiry, open a bull call spread one strike apart, hold to
expiry"* on the past expiries of one underlying, and measures it: per-expiry results, win rate,
average P&L, maximum drawdown and an equity curve.

Pure Python (standard library only). `psycopg` is needed only to read from PostgreSQL.

```bash
cd backtest
python -m finova_backtest templates
python -m finova_backtest run --data-dir ../backend/testdata/demo --underlying خودرو \
    --template bull-call-spread --entry-days 30 --expiries weekly --out run.json
python -m pytest            # 30 tests, no database needed
```

## The rule

| Field | Default | Meaning |
|---|---|---|
| `template_id` | – | one of the 9 templates (`python -m finova_backtest templates`) |
| `entry_days` | 30 | calendar days before expiry; the entry is the last trading day on or before that date (at most `max_entry_gap_days` = 5 earlier, else the expiry is skipped) |
| `quantity` | 1 | contracts per leg unit (a share leg is `ratio x quantity x 1000` shares) |
| `exit` | `expiry` | `expiry`, or `target_stop`: close early when the open P&L reaches `target_pct` or `-stop_pct` of the capital at risk, checked on each day's close |
| `fill` | `last` | price to trade at: `last` (last price, else settlement), `mid` (bid/ask mid) or `cross` (buy at ask, sell at bid). `mid` and `cross` fall back to `last` when a row has no bid/ask |
| `allocation` | 0.05 | fraction of the starting equity risked per trade (see equity) |
| `fees` | see below | proportional fees |
| `allow_model` | true | allow Black-Scholes prices where the history has none |

**Templates** are the draft list in `docs/api-contract.md` 2.3 (long call/put, covered call,
protective put, collar, bull call spread, bear put spread, long straddle, long strangle). Strikes
are picked among the *listed* strikes of the expiry: `atm` is the listed strike nearest to the
underlying price (the lower one on a tie), `otm n` / `itm n` move `n` listed strikes away from it.

## Prices

1. **Snapshot** - the `quote_snapshots` row of that contract and day (`price_source = "snapshot"`).
2. **Model** - when there is no usable row (no listed chain that day, an unlisted strike): Black-Scholes at
   the underlying's HV20 on that day (`"model"`). A trade with both kinds of legs is `"mixed"`.
   The conventions are the same as the Go calculation layer: r = 0.30, T = calendar days / 365,
   HV20 = sample stdev of the last 20 log returns x sqrt(240). Returns bigger than 12% are dropped from HV20,
   because a raw price series has capital-increase jumps that are not volatility (the exchange limit is 5% a day).
   With no listed chain, the strike grid is the underlying's usual step, scaled to the price of that day.
3. **Settlement** - at expiry the underlying's close on the expiry day (the last trading day before it) and the
   intrinsic value of each option. This is a simplification: the real exchange settles on an average price.

## Fees

Proportional, in `Fees` (defaults are round placeholders near the Tehran tariff, **not a broker's numbers**):
option commission 0.1% of the premium value on each trade; an exercise fee of 0.1% of the strike value for
every option that settles in the money (long or short); shares 0.4% to buy and 0.9% to sell (with the sales tax).
Held to expiry, the options are not traded again, so only the exercise fee applies. The share leg is valued
as if sold at the settlement price.

## Results

* **Capital at risk** of a trade is the strategy's maximum loss at expiry, from its payoff (for a covered call,
  buying the shares at the entry price). If the loss were unlimited it falls back to the net premium.
* `return_on_risk = pnl / capital_at_risk`, after fees.
* **Equity** starts at 1.0. Trades are added in the order they close: `equity += allocation x return_on_risk`
  (a trade cannot lose more than 100% of its allocation). The curve has a point at the first entry and one at each exit.
  There is no compounding: weekly expiries overlap in time, and compounding overlapping trades would count the same
  money twice. With `allocation` 0.05 and 4 to 5 trades open at once, about 20-25% of the starting equity is at risk.
* `max_drawdown` is the largest peak-to-trough fall of that curve (a positive fraction); `win_rate` counts trades
  with `pnl > 0`; `profit_factor` is gross wins / gross losses (null with no loss).

`to_backtest_run(result)` returns a dict shaped like a `backtest_runs` row: `model_name`
(`strategy:<template>`), `params`, `start_date`, `end_date`, `win_rate`, `max_drawdown`, `total_return`,
`equity_curve` (`[{date, equity}]`, as `docs/api-contract.md` 3.3 asks), plus `metrics`, `trades` and `skipped`.
`--out` writes it as JSON. See `docs/strategy-backtest.md` for the proposed table and API shape.

## What the data can and cannot do (read before trusting a number)

* `quote_snapshots` holds only contracts that are **listed today** (about 60 trading days each). **No expiry in it
  has passed yet**, so `--expiries observed` has nothing to run on until the daily snapshot has been running past an
  expiry. Until then use `--expiries weekly`, a synthetic Wednesday calendar: every price is then a Black-Scholes
  price at HV20, so the backtest measures how a strategy would have behaved on **model prices**, not real quotes.
  The result says so: `metrics.model_priced_share` is 1.0 and each trade has `price_source = "model"`.
* Backfilled rows have no bid/ask and no open interest: `fill = mid/cross` equals `last` on them.
* Model prices at HV20 ignore the volatility smile and the market's real spreads and liquidity. Fees are placeholders.
* The same entry rule on weekly expiries gives overlapping trades; do not read the equity curve as one account's balance.
* `--expiries weekly` covers the last three years by default (`--from` / `--to` change it).
