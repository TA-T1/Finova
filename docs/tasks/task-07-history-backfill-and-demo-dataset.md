# Task 7: Option history backfill and demo dataset

Owner role: Crawl. PR: #11 (`feat/crawl-history-backfill`), **stacked on #8**: its base is `feat/crawl-snapshot-rebased`; retarget to `main` once #8 is merged.
Depends on: task 2 (hard). Unblocks: task 9 (backtest runs), the offline demo (task 19).

## What the task was
Use TSETMC `GetClosingPriceDailyList` to fill `quote_snapshots` for the current contracts (history has no bid/ask or open interest, about 60 days per contract), and save a full market snapshot that can be loaded into a database for an offline demo.

## What was built

### `backend/internal/backfill/` (history backfill)
`Run(ctx, src, write, opts...)`:
1. Reads today's option chain (`OptionMarketWatch`) and lists every call and put once, sorted by symbol.
2. Fetches each underlying's daily history once (for `underlying_price`), then each contract's history (`GetClosingPriceDailyList/{insCode}/0`, raw).
3. Builds `snapshot.Row` values: `close_price` = settlement price (`pClosing`), `last_price` = last trade (`pDrCotVal`), `volume`, `value`, `trade_count` (`zTotTran`), `underlying_price` = the underlying's last price (else settlement) on the same date, NULL if that day is missing.
4. Drops days from today on (Tehran), days after the contract's expiry, and days with no volume and no price.
5. Writes in batches of 20,000 rows, so an interrupted run keeps its progress.
A failed contract or underlying is logged and counted (`failed`), and the run continues. Options: `WithLimit(n)`, `WithClock`, `WithLogger`.

Bid, ask and open interest are not in history, so they stay `0`, which in this table means "unknown". A backtest must not use a backfilled row for bid/ask pricing.

### Never overwrites live data
`snapshot.InsertMissing` (new, in `internal/snapshot/store.go`) uses `ON CONFLICT (trade_date, contract_symbol) DO NOTHING`. The existing upsert was refactored (`mergeHead` + `mergeUpdate`/`mergeSkip`, shared `write`) so the live path behaves exactly as before. A live snapshot with bid/ask/open interest always wins, and the backfill can be rerun safely.

### `backend/internal/demodata/` (export / import)
Tables, in load order: `underlyings`, `underlying_daily_prices`, `option_contracts`, `option_quotes` (newest row per contract only), `underlying_quotes` (newest per underlying), `quote_snapshots`. Each becomes `<table>.csv.gz` with a header row.
- `Export` uses `COPY (query) TO STDOUT` per table.
- `Import` loads all tables in one transaction, takes the column list from each file's header, resets the id sequences past the loaded ids, and refuses a database whose market tables are not empty unless `replace` is set (`TRUNCATE ... RESTART IDENTITY CASCADE`). It checks all files exist before touching the database.

### `backend/cmd/backfill`
```
backfill history [-limit N]         fill quote_snapshots from TSETMC (about 15 min at 2 req/s)
backfill export <dir>               save the market tables to <dir>/*.csv.gz
backfill import [-replace] <dir>    load them
backfill save-market <file.json>    save one live MarketSnapshot (loadable with source.NewFake)
```
`history` and `save-market` reach TSETMC; `export` and `import` need only `DATABASE_URL`.

### Other changes
- `datasource.DailyPrice` gained `TradeCount`; the TSETMC client reads `zTotTran` into it.
- `docs/api.md`: new section "History backfill and demo dataset".

### The committed demo dataset: `backend/testdata/demo/` (1.1 MB)
Taken 2026-11-24 after the close:
- 1,818 contracts, 30 underlyings.
- `quote_snapshots`: 59,642 rows, 2026-07-29 to 2026-11-24: history for all current contracts (57,824 rows, 0 failed contracts) plus a real live snapshot of 2026-11-24 (1,818 rows, with bid/ask/open interest) taken with `snapshot run`.
- `underlying_daily_prices`: 19,009 rows, 2023-11-08 to 2026-11-24. This is the **back-adjusted** three-year series from `data-pipeline/output/underlyings_daily.csv` (rounded to whole rials), with raw prices only after that file's last date (2026-11-07). Reason below.
- newest option and underlying quotes.

Load into a migrated database:
```bash
for f in backend/migrations/*.up.sql; do psql "$DATABASE_URL" -f "$f"; done
cd backend && go run ./cmd/backfill import testdata/demo
```

## Tests
- `internal/backfill`: rows from history by hand (prices, identity, bid/ask/OI zero, underlying price), today and after-expiry rows dropped, underlying falls back to settlement price, missing underlying history gives NULL price, a failed contract is counted and skipped, `WithLimit`, underlying history fetched once, empty chain is an error.
- `internal/snapshot` (DB): `InsertMissing` leaves existing rows untouched.
- `internal/demodata` (DB): export then import round trip (row counts, newest quote only, sequences usable after import, non-empty database refused, `replace` works), and import with missing files fails before touching the database.
- DB tests need `TEST_DATABASE_URL`; they run against a throw-away schema and skip otherwise.

## Findings other people need
1. **`adjusted=1` returns one row** for these instruments. So `POST /market/collect/daily/:insCode?adjusted=true` (recommended by `docs/calculation-layer.md`) stores a single day. Raw (`adjusted=false`) has the full history but contains jumps at capital increases (for example 3862 to 611). Those jumps read as volatility in HV20 for the next 20 trading days. The demo dataset uses the adjusted CSV instead. The calculation layer needs a real source of adjusted prices.
2. **No past expiry exists in `quote_snapshots`.** Only contracts listed today are in the chain; expired contracts are not listed, so their `insCode`s are unknown and cannot be backfilled. The backtest (task 8/9) is therefore model-priced until the daily snapshot has run past some expiries.
3. History does not include today; today comes from the live snapshot.
4. The collector refuses to collect outside market hours (409). Seeding the dev database off-hours required bypassing that guard locally; this was not committed.
5. Underlying names from the market watch carry Arabic letters (for example `اتكاسا`); not changed here.

## Limits
- Only contracts in today's chain, about 60 trading days each; illiquid or new contracts have few rows (1,746 of 1,818 contracts have any history).
- A full run is about 1,800 requests, around 15 minutes at the default 2 requests per second.

## For task 9 and other consumers
Treat `bid_price = ask_price = open_interest = 0` as unknown, not as real zeros. Rows from the live snapshot (`trade_date` 2026-11-24 in the demo set) are the only ones with those fields.
