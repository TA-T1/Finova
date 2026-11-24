# Strategy backtest: result format and API (proposal)

Status: **proposal** from the engine (`backtest/`). Task 9 (backtest runs) writes it, task 11 (results API)
serves it, task 18 (results page) shows it. `docs/api-contract.md` section 3 only covers *signal model* backtests;
this adds strategy backtests next to them. It changes nothing for signal backtests.

## Storage

`backtest_runs` (migration 000003) already has `model_name`, `params`, `start_date`, `end_date`, `win_rate`,
`max_drawdown`, `total_return`, `equity_curve`. The engine fills them like this:

| Column | Value |
|---|---|
| `model_name` | `strategy:<template id>`, for example `strategy:bull-call-spread` |
| `params` | `{ "underlying": "خودرو", "template_id": ..., "entry_days": 30, "exit": "expiry", ... }` (the whole rule) |
| `start_date`, `end_date` | first entry, last exit |
| `win_rate`, `total_return` | fractions (`0.56` = 56%) |
| `max_drawdown` | positive fraction |
| `equity_curve` | `[{ "date": "2026-11-23", "equity": 1.0 }, ...]`, starts at 1.0 |

Two things do not fit and need **one new migration** (suggested `000010_backtest_run_details`):

```sql
ALTER TABLE backtest_runs
    ADD COLUMN metrics JSONB,   -- trades, skipped, avg_pnl, profit_factor, total_fees, model_priced_share, ...
    ADD COLUMN trades  JSONB;   -- one object per expiry, see below
```

(or a `backtest_trades` table keyed by `run_id`; the JSON shape below is the same either way). The engine's
`to_backtest_run()` output already has the keys `metrics`, `trades` and `skipped`.

## Trade (one per expiry)

```json
{
  "expiry": "2026-12-23", "entry_date": "2026-11-23", "exit_date": "2026-12-23",
  "exit_reason": "expiry",
  "spot_entry": 2000, "spot_exit": 2150,
  "net_premium": 70000, "capital_at_risk": 70000, "max_profit": 30000, "max_loss": 70000,
  "gross_pnl": 30000, "fees": 4330, "pnl": 25670, "return_on_risk": 0.3667,
  "price_source": "snapshot", "chain_source": "snapshot",
  "legs": [ { "type": "call", "side": "buy", "qty": 1, "strike": 2000, "entry_price": 150, "exit_price": 150, "price_source": "snapshot" } ]
}
```

Money is in rials per position (price x 1000 x contracts), `exit_reason` is `expiry`, `target` or `stop`, and
`price_source` is `snapshot`, `model` or `mixed`. Expiries the engine could not trade are in `skipped`:
`[{ "expiry": "...", "reason": "no listed chain on the entry day" }]`.

## API (task 11)

Same conventions as `api-contract.md`: camelCase, `null` for unknown, no omitted fields.

`GET /backtests?underlying=<symbol>&template=<id>&model=<modelName>&limit=` (extends 3.2: two new filters, and two new
fields on each item: `underlying`, `template`; both `null` for a signal backtest):

```json
{ "count": 1, "items": [ {
    "id": 12, "modelName": "strategy:bull-call-spread", "template": "bull-call-spread", "underlying": "خودرو",
    "startDate": "2025-02-05", "endDate": "2026-11-18", "winRate": 0.46, "maxDrawdown": 0.21, "totalReturn": 0.18,
    "trades": 90, "avgPnl": 1250000, "modelPricedShare": 1.0, "createdAt": "2026-11-24T20:00:00+03:30" } ] }
```

`GET /backtests/:id` (extends 3.3) adds, for a strategy run: `params`, `metrics` (camelCase copy of the engine's
metrics), `trades` (the trade objects above in camelCase) and `skipped`, next to `equityCurve`.

The template ids and legs come from `GET /strategies/templates` (api-contract 2.3), so the results page can
show the legs of the strategy it is describing.
