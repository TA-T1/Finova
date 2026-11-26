# Market-data crawl module

This backend crawls Iranian options-market data (اختیار معامله) from public,
undocumented JSON APIs and stores it in Postgres. The module is provider-neutral:
all callers depend on `internal/datasource` types, and each site is an adapter.

## Package layout

```
internal/
  datasource/                 Source interface + neutral DTOs (OptionRow, DailyPrice, InstrumentInfo)
    tsetmc/                   TSETMC adapter (PRIMARY, fully implemented)
      client.go               HTTP client (base URL, headers, timeout)
      source.go               endpoint parsing + mapping to DTOs
    tse/                      tse.ir adapter (stub — returns ErrNotSupported)
    optionbaaz/               optionbaaz.ir adapter (stub — lowest priority)
  market/
    repository.go             pgx upserts + read queries
    collector.go              fetch -> upsert orchestration + background scheduler
    handler.go                Gin HTTP endpoints
```

To add a source, implement `datasource.Source` in a new sub-package; no changes
to the collector, repository, or DB layer are needed.

## Data sources

### TSETMC — primary (`https://cdn.tsetmc.com/api`)

Plain JSON GETs. Require a browser `User-Agent` and `Referer`/`Origin` of
`https://www.tsetmc.com` (set automatically by the client); no cookies or auth.

| Endpoint | Purpose | Tables fed |
|---|---|---|
| `GET /Instrument/GetInstrumentOptionMarketWatch/0` | Full live option chain: one row per strike/expiry pairing call (`_C`) + put (`_P`) legs with best bid/ask, volume, value, open interest, plus the underlying snapshot | `option_contracts`, `option_quotes`, `underlying_quotes` |
| `GET /ClosingPrice/GetClosingPriceDailyList/{insCode}/{adjusted}` | End-of-day OHLC history (`adjusted` = `0` raw, `1` adjusted) | `underlying_daily_prices` |
| `GET /Instrument/GetInstrumentInfo/{insCode}` | Instrument identity: ISIN (`cIsin`), symbol (`lVal18AFC`), name (`lVal30`) | `underlyings` |

Key field mapping (TSETMC → schema): `pDrCotVal`→last/close, `pClosing`→final
(settlement), `qTotTran5J`→volume, `qTotCap`→value, `oP`→open interest,
`pMeDem`/`qTitMeDem`→best bid + volume, `pMeOf`/`qTitMeOf`→best ask + volume.
`beginDate`/`endDate`/`dEven` are Gregorian `YYYYMMDD`. `insCode`s are stored as
TEXT in `underlyings.ins_code` / `option_contracts.ins_code` (migration 000005).

### tse.ir / optionbaaz.ir — secondary (stubs)

`www.tse.ir` is mostly overlapping disclosure/reference data; `optionbaaz.ir`
mainly re-derives Greeks/IV, which this project computes itself. Both are stubbed
behind the same interface so they can be fleshed out later without wider changes.

## HTTP endpoints (`/api/v1`)

> The agreed final request/response shapes, including the strategy and backtest
> endpoints, are in [api-contract.md](api-contract.md). Where it differs from the
> table below, it wins.

| Method | Path | Description |
|---|---|---|
| `GET`  | `/health` | Liveness + DB ping |
| `POST` | `/market/collect/options` | Run one option-chain collection pass; returns counts (`rows`, `underlyings`, `contracts`, `quotes`, `metrics`, `errors`; `metrics` is the number of calculated rows stored, see [calculation-layer.md](calculation-layer.md)) |
| `POST` | `/market/collect/daily/:insCode?adjusted=true` | Backfill an underlying's daily history |
| `GET`  | `/market/options?underlying=<sym>&underlyingKind=stock\|fund\|index\|other&type=call\|put&expiry=YYYY-MM-DD&sort=<field>&order=asc\|desc&page=1&pageSize=50` | Latest quotes and calculated metrics, filtered and paged (`limit` is a deprecated `pageSize` alias) |
| `GET`  | `/market/underlyings?kind=stock\|fund\|index\|other` | Known underlyings (optionally filtered by kind) |
| `GET`  | `/market/expiries?underlying=<sym>` | Active expiries and days to expiry, optionally filtered by underlying |
| `GET`  | `/market/chain?underlying=<sym>&expiry=YYYY-MM-DD` | One expiry's strike rows, pairing each listed call and put |
| `GET`  | `/market/underlyings/:symbol` | Underlying quote, active expiries, open-interest summary/chart data, and max pain |
| `GET`  | `/market/symbol/:symbol` | Deprecated alias for `/market/underlyings/:symbol` |

`/market/options` returns `{ items, count, page, pageSize, total, totalPages }`.
`items` contain the latest quote and calculated fields, with unavailable values
as `null`. Sorting and pagination are applied by the database; null sort values
remain last in either direction. The option chain keeps its nested strikes and
adds top-level days to expiry and underlying price.

The chain response has a `strikes` array; each row contains `strikePrice` and a
`call` and/or `put` object (a missing side is `null`). Expiries are Gregorian
dates. Both endpoints include only expiries on or after the current date in
Asia/Tehran. The symbol page's `openInterest` totals use the latest quote per
contract; `pcr` is put OI divided by call OI and is `null` when call OI is zero.
Its `strikeOpenInterest` values aggregate all active expiries. `maxPain` uses
the nearest active expiry and the strike with the lowest total intrinsic
expiration payout, weighting each contract by open interest and contract size.

Underlying `priceChange` is last price minus the previous close, and
`priceChangePercent` is relative to that previous close. Newly collected
underlying quotes persist the TSETMC previous-close field; older quotes keep a
null previous close until refreshed.

### Underlying classification (اختیار سهام vs. others)

Each underlying is tagged with a `kind` so stock options (اختیار سهام) can be
separated from fund/ETF and index options. The kind is derived in
`internal/market/classify.go` from the instrument's market sector (`cSecVal`
from `GetInstrumentInfo`):

| kind | meaning | rule |
|---|---|---|
| `fund`  | option on a tradable fund/ETF unit | sector `68` ("صندوق سرمايه گذاري قابل معامله") or sector name contains "صندوق" |
| `stock` | option on a single company's shares | any other non-empty industry sector |
| `index` | cash-settled option on an index | reserved; none listed on TSETMC today |
| `other` | unclassified | no sector info (e.g. identity lookup failed) |

A re-collection never downgrades a known kind back to `other`.

## Configuration (env)

| Var | Default | Meaning |
|---|---|---|
| `TSETMC_BASE_URL` | `https://cdn.tsetmc.com/api` | API base (no trailing slash) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000,http://localhost:5173` | Comma-separated browser origins allowed by the API |
| `TSETMC_USER_AGENT` | built-in desktop UA | override UA sent to TSETMC |
| `COLLECT_INTERVAL` | *(empty = off)* | enable background collector, e.g. `60s`, `2m` |

> The server must run from a network that can reach `cdn.tsetmc.com` (Iran).

## Market snapshot adapter (`MarketSource`)

`internal/datasource/market.go` defines the snapshot API the collection
service should use: `FetchMarket(ctx) (*MarketSnapshot, error)` returns every
active option contract (with its latest quote) plus quotes for every
underlying that has options, in one call.

Field names follow the brief in `docs/01-crawl-adapter.md`; `docs/api.md` had no
contract for these structs. Prices are `float64` here (the older
`OptionRow`/`OptionLeg` types use `int64`); `Expiry` is a Gregorian date at
00:00 `Asia/Tehran`; missing numbers are `0`; `LastTradeTime` is `nil` when
unknown.

**Names.** The brief calls these `internal/source`, `source.Source`, `source.NewFake`... They are available under exactly those names:
`internal/source` is a thin package of type aliases over `internal/datasource` (`source.Source` *is* `datasource.MarketSource`), so the two names are interchangeable.
Nothing was renamed or removed: `datasource.Source`, `OptionRow`, the `tsetmc` client and the collector that use them are unchanged.
New code (the snapshot, the collection service) should import `internal/source`.
Only `cmd/...` may build the TSETMC client; a test (`internal/source`) fails if a library package imports it or mentions a TSETMC host.

### FakeSource (offline development)

```go
src, err := datasource.NewFake("testdata/market_sample.json") // implements MarketSource
snap, err := src.FetchMarket(ctx)
src.SetError(errors.New("boom")) // make later calls fail, to test error paths
```

`backend/testdata/market_sample.json` (fixture for `FakeSource`): the 27 underlyings are real latest quotes
from `data-pipeline/output/underlyings_daily.csv` (2026-11-07). The 36 contracts
are **synthetic** (`source_id` starts with `SYNTH`): they exist only to give
the right shape and are not real market prices.

### Live implementation: `tsetmc.Client.FetchMarket`

`GET /Instrument/GetInstrumentOptionMarketWatch/0` gives the snapshot. Each response row holds a call,
a put and the underlying's quote; the adapter splits it into `OptionContract`s and de-duplicated
`UnderlyingQuote`s (about 900 rows give 1,800 contracts and 28 underlyings on 2026-11-15).
The chain has no names, no underlying volume and no trade times, so `FetchMarket` then makes
two more requests to fill them (see "Enrichment" below). Those are best effort: if they fail, the chain is still returned.

| Our field | TSETMC field |
|---|---|
| `SourceID` | `insCode_C` / `insCode_P` |
| `Symbol` | `lVal18AFC_C` / `_P` |
| `UnderlyingSymbol` | `lval30_UA` (fallback: parsed from `lVal30_*`) |
| `Strike` | `strikePrice` (fallback: name) |
| `Expiry` | `endDate` (Gregorian `YYYYMMDD`; fallback: Jalali date in the name) |
| `ContractSize` | `contractSize` |
| `LastPrice` / `ClosePrice` / `PrevClose` | `pDrCotVal` / `pClosing` / `priceYesterday` |
| `BidPrice`, `BidVolume` / `AskPrice`, `AskVolume` | `pMeDem`, `qTitMeDem` / `pMeOf`, `qTitMeOf` |
| `Volume` / `Value` / `TradeCount` / `OpenInterest` | `qTotTran5J` / `qTotCap` / `zTotTran` / `oP` |
| underlying `SourceID`, `LastPrice`, `ClosePrice`, `PrevClose` | `uaInsCode`, `pDrCotVal_UA`, `pClosing_UA`, `priceYesterday_UA` |

The chain endpoint itself lacks `LastTradeTime`, underlying `Volume` and underlying `Name`. They come from the enrichment below.

#### Enrichment (2 extra requests per snapshot)

| Request | Gives | Notes |
|---|---|---|
| `GET /ClosingPrice/GetMarketWatch?market=0&paperTypes[0..8]=1..9&showTraded=false&withBestLimits=false&hEven=0&RefID=0` | for every instrument: full name `lvc`, volume `qtj`, last trade time of day `hEven` (`HHMMSS`, no date) | about 3.7 MB and 1.2 s. `dEven` is always 0 here. |
| `GET /ClosingPrice/GetClosingPriceInfo/{insCode}` for the most traded underlying | the date of the latest trading session, `dEven` (fallback `finalLastDate`) | about 40 ms |

- `UnderlyingQuote.Name` = `lvc` (normalized), `Volume` = `qtj`. An underlying absent from the watch
  (the real capture has one, وبصادر, a halted symbol) keeps `Name` = `Symbol` and `Volume` = `0`.
- `OptionContract.LastTradeTime` = session date + `hEven`, but only for contracts whose chain `Volume` is > 0.
  Contracts that did not trade keep `nil`. Their `hEven` is only a pre-open marker such as `061038`.
  Times before 09:00 are rejected.
- In the real capture, 1,000 of the 1,001 traded contracts have a time between 09:00 and 12:30.
  One (ضموج844) reports 14:19:20, probably an exercise or settlement update.
- If either request fails, a warning `enrichment failed` is logged and the unfilled fields stay at their defaults.
  If only the closing-info request fails, names and volume are still filled.

Behaviour:
- **Normalization:** Arabic `ي`/`ك` become `ی`/`ک`, Persian and Arabic digits become ASCII,
  and zero-width characters and surrounding whitespace are removed.
- **Dates:** `Expiry` is a Gregorian date at 00:00 `Asia/Tehran`. Jalali dates are converted with `go-persian-calendar`.
- **Missing values:** a missing, `null` or empty number is `0`.
- **Bad rows:** a malformed row is skipped with a `slog` warning.
  An empty or unrecognised response, or one with no valid contract, returns an error.

Reliability, configured by env vars (see `backend/.env.example`):

| Var | Default | Meaning |
|---|---|---|
| `TSETMC_TIMEOUT` | `10s` | timeout of each HTTP attempt |
| `TSETMC_MAX_ATTEMPTS` | `3` | total attempts; retries on network errors, timeouts, 5xx and 429 with exponential backoff (500ms, 1s, ...); no retry on other 4xx |
| `TSETMC_RATE_LIMIT` | `2` | requests per second (`0` = unlimited). TSETMC documents no limit, so this is a polite default |

`go run ./cmd/fetchdemo` (from `backend/`) calls the real source once and prints the counts, 5 sample contracts and the call duration.
`backend/scripts/capture_tsetmc_samples.sh` re-captures the real samples into `backend/testdata/`.
Tests run offline against `httptest.Server` serving `testdata/tsetmc_option_watch.json`.

### Historical option prices

**Yes, TSETMC serves option price history.** Verified in the browser on 2026-11-15 with
`GET /ClosingPrice/GetClosingPriceDailyList/{insCode}/0`, the same endpoint `Client.DailyPrices` already wraps:

| Instrument | Rows | Range |
|---|---|---|
| active call ضهرم8030 (`61500711433740234`) | 61 | 2026-08-19 .. 2026-11-14 (since listing) |
| its put طهرم8030 (`58980125072230074`) | 61 | 2026-08-19 .. 2026-11-14 |
| underlying اهرم | 1,175 | 2021-12-20 .. 2026-11-14 |

Each row has `dEven`, `priceFirst`, `priceMax`, `priceMin`, `pDrCotVal` (last), `pClosing`, `priceYesterday`,
`qTotTran5J` (volume), `qTotCap` (value) and `zTotTran` (trade count).

What this means for the Sprint 3 backfill:
- **Missing from history:** bid, ask and open interest, which exist only in the live chain. `quote_snapshots` rows backfilled from here can fill
  `close_price`, `last_price`, `volume`, `value`, `trade_count` and `underlying_price` (joined from the underlying's own history by date),
  but must leave bid/ask/open interest at 0 or NULL.
- **Reach:** a contract's history starts at its listing, so about 60 days for a current contract. Contracts that have already expired do not appear in the
  chain, so their `insCode`s are unknown. Backfilling them would need another way to list them (not investigated).
- **Today is not included:** the latest row is yesterday's. Today comes from the live snapshot.
- **`adjusted=1`:** returned a single row for the options and the underlying in this check, so use `adjusted=0` for options.
- **Cost:** one request per contract (about 1,800), so a full backfill of the current contracts needs about 1,800 requests, around 15 minutes at the default 2 requests per second.

### Known limitations and future work

| Item | Status |
|---|---|
| `LastTradeTime`, underlying `Name` and `Volume` | **Filled by the enrichment requests** (decision 2026-11-15: first left unfilled, then fixed after finding the endpoints in the browser). They are unset only when those requests fail, or for a contract that did not trade (`nil`), or an underlying missing from the market watch (`Name` = `Symbol`, `Volume` = `0` meaning *unknown*). Callers should not store `Volume` = 0 as a real value. |
| Existing callers of the TSETMC client | The retry/timeout/rate-limit layer applies to the **existing** methods too (`OptionMarketWatch`, `DailyPrices`, `InstrumentInfo`), so the collector gets it for free. New defaults: 10 s per request (was 30 s), 3 attempts (was 1), 2 requests per second (was unlimited). A custom `http.Client` passed with `WithHTTPClient` keeps its own `Timeout`. To restore the old behaviour set `TSETMC_TIMEOUT=30s`, `TSETMC_MAX_ATTEMPTS=1` and `TSETMC_RATE_LIMIT=0`. |
| Trade date on a holiday | **Unverified.** The enrichment takes the date from `GetClosingPriceInfo.dEven`, and it was only observed on a trading day (2026-11-15). We expect it to stay at the last session's date on Thursday, Friday and holidays, but nobody has checked yet. The snapshot has a guard for this case (see "How a non-trading day is detected"). Run `bash backend/scripts/verify_live.sh` on a Thursday or Friday (the next one is 2026-11-19): it fails if `dEven` equals today on a closed day. On 2026-11-15 (a trading day) it passed its other checks against the live source. |
| Cost | Each snapshot now downloads about 3.7 MB more (the market watch). At a 30-60 s polling cadence, consider calling it less often or caching the names. |
| Names of contracts' underlyings not in the market watch | Fall back to the symbol. The collector gets full names from `GetInstrumentInfo` when it creates an underlying. |
| Real samples | `backend/testdata/tsetmc_market_watch.json` is the real market watch, **trimmed** to the instruments in the option chain (1,458 rows) with the best-limits ladder (`blDs`) removed, to keep it at about 0.8 MB. `tsetmc_closing_price_info.json` is the closing info of اهرم. `capture_tsetmc_samples.sh` fetches the untrimmed originals. |

## Daily end-of-day snapshot (`cmd/snapshot`, `internal/snapshot`)

`quote_snapshots` keeps one row per contract per trading day (migration
`000008_quote_snapshots`; **new migration, the backend team should review it**).
`option_quotes` holds intraday ticks only, so this table is our real price history for the backtest.
It may be the only history we get (see "Historical option prices" above: history exists for about the last 60 days of current contracts, without bid/ask/open interest).

| Column | Notes |
|---|---|
| `trade_date` | Tehran calendar date of the trading day. With `contract_symbol` it is the primary key, the upsert target. |
| `contract_symbol`, `underlying_symbol`, `option_type` (`call`/`put`), `strike`, `expiry` | Contract identity. |
| `close_price`, `last_price`, `bid_price`, `ask_price` | `bid_price` and `ask_price` are as of `captured_at`. |
| `volume`, `value`, `trade_count`, `open_interest` | `0` means the source had no figure or nothing traded. |
| `underlying_price` | The underlying's last price at `captured_at`, joined by underlying symbol. **`NULL` when unknown** (no matching underlying, or a last price of 0). Treat NULL as "do not use this row for pricing". |
| `captured_at` | The adapter's `FetchedAt`. |

Money columns are `BIGINT` rials, like the rest of the schema.

```bash
go run ./cmd/snapshot run      # one snapshot now, then exit
go run ./cmd/snapshot daemon   # a snapshot every day at SNAPSHOT_TIME, until SIGINT/SIGTERM
```

| Var | Default | Meaning |
|---|---|---|
| `SNAPSHOT_TIME` | `13:00` | daily run time, `HH:MM` in Asia/Tehran (the market closes at about 12:30) |
| `SNAPSHOT_TRIES` | `3` | total attempts per day if a run fails |
| `SNAPSHOT_RETRY_DELAY` | `5m` | wait between attempts. After the last failed attempt it logs the error and waits for the next day. |

`snapshot.Run(ctx, src, db)` fetches the market once. It skips non-trading days, joins each contract with its
underlying price, and writes all rows (about 1,800) in **one transaction**: `COPY` into a temporary staging table, then one
`INSERT ... ON CONFLICT (trade_date, contract_symbol) DO UPDATE`. A second run on the same day overwrites the first run's values and
creates no duplicates. Against live TSETMC a run takes 3-5 s, almost all of it the three adapter requests (the market watch is 3.7 MB) at the default 2 requests per second.
It returns the number of rows written; skipping a non-trading day returns 0, logs `no trading today`, and is not an error.

### How a non-trading day is detected

The spec says to skip the day when no contract has a last trade time on today's Tehran date.
The TSETMC adapter fills `LastTradeTime` from its enrichment requests, so that rule is the main one. The checks run in this order:
1. **A contract has `LastTradeTime`:** trading happened iff one of them falls on today's Tehran date.
   *Guard:* if that says "yes" but every figure (volume, trade count, last price, close price) is identical to the latest earlier snapshot,
   the day is still skipped, because a source serving a stale date would otherwise store a duplicate row for a holiday.
   This guard exists because the date's behaviour on a holiday is unverified (see "Known limitations").
2. **No contract has a `LastTradeTime` at all** (the enrichment failed, or another source): compare with the latest earlier snapshot in
   `quote_snapshots`. On a closed day TSETMC keeps serving the last trading day's final numbers, so if no traded contract has a different
   volume, trade count, last price or close price, the day is skipped. No holiday calendar is needed.
3. **No earlier snapshot** (the first ever run): Thursday and Friday are skipped, and so is a day on which no contract has volume.

Caveats:
- **Mid-session runs:** a manual `snapshot run` taken during the session stores partial figures. The 13:00 run overwrites them the same day.
  But if the next day is a holiday, the final figures then differ from that partial row, and a duplicate row can be written for the holiday.
  Take manual snapshots after the close.
- **Missed days:** the daemon only schedules the next 13:00. If it is down at 13:00, that day is missed, and backfilling is out of scope (Sprint 3).
  After a restart, run `snapshot run` by hand.

## History backfill and demo dataset (`cmd/backfill`)

`quote_snapshots` only gets a row per day from the day the daily snapshot starts running.
`backfill history` fills the past from TSETMC's per-instrument history
(`GetClosingPriceDailyList/{insCode}/0`, see "Historical option prices").

```bash
go run ./cmd/backfill history            # every contract in today's chain, about 15 min at 2 requests/s
go run ./cmd/backfill history -limit 20  # quick trial
```

- **What is filled:** `close_price` (settlement price), `last_price`, `volume`, `value`, `trade_count`, and `underlying_price`
  (the underlying's own history, same date, last price, else settlement price; `NULL` if that day is missing).
- **What is not:** `bid_price`, `ask_price` and `open_interest` stay `0`, which means *unknown* in this table. A backtest must not use a
  backfilled row for bid/ask pricing.
- **Which days:** only days before today in Tehran, never after the contract's expiry, and not days on which the contract did not trade
  (no volume and no price). Today belongs to the live snapshot.
- **Never overwrites:** rows are inserted with `ON CONFLICT DO NOTHING` (`snapshot.InsertMissing`), so a live snapshot with bid/ask/open interest
  always wins, and the command can be run again safely. It writes in batches of 20,000 rows, so an interrupted run keeps its progress.
- **Reach:** only contracts in the current chain (about 60 trading days each). Expired contracts are not listed, so they are not backfilled.
- A failed contract is logged and counted (`failed=`), and the run goes on. The summary line prints the counts and the date range.

### Demo dataset (offline)

```bash
go run ./cmd/backfill export <dir>            # save the market tables to <dir>/*.csv.gz
go run ./cmd/backfill import [-replace] <dir> # load them (migrations applied first)
go run ./cmd/backfill save-market file.json   # one live MarketSnapshot, loadable with source.NewFake
```

The dataset has `underlyings`, `underlying_daily_prices`, `option_contracts`, the newest `option_quotes` and `option_metrics` rows per contract, the newest
`underlying_quotes` row per underlying, and `quote_snapshots`. `option_metrics.csv.gz` is optional on import (the committed copy does not have it yet, see
`docs/tasks/task-07-history-backfill-and-demo-dataset.md`). `import` refuses a database whose market tables are not empty, unless
`-replace` is given (it then runs `TRUNCATE ... RESTART IDENTITY CASCADE` on these tables, which also empties tables that reference them).
It is one transaction, and the id sequences are moved past the loaded ids.

The committed copy is in `backend/testdata/demo/`, taken on 2026-11-24 after the market closed (1,818 contracts, 30 underlyings):
`quote_snapshots` has 59,642 rows from 2026-07-29 to 2026-11-24 (history for all current contracts, plus a live snapshot of 2026-11-24 with bid/ask/open interest);
`underlying_daily_prices` holds the **back-adjusted** three-year series of `data-pipeline/output/underlyings_daily.csv` (rounded to whole rials),
not the raw series, because raw prices have jumps at capital increases that would show up as volatility in HV20. Load it with:

Migration `000012_backtest_run_details` adds the JSONB `metrics`, `trades` and
`skipped` fields to `backtest_runs` for strategy backtests. The migration loop
below applies it along with the rest of the schema.

```bash
for f in backend/migrations/*.up.sql; do psql "$DATABASE_URL" -f "$f"; done   # or your migration tool
cd backend && go run ./cmd/backfill import testdata/demo
```

Note on `adjusted=1`: for these instruments the endpoint returned a single row (one day), so a series backfilled with
`POST /market/collect/daily/:insCode?adjusted=true` holds one row per underlying. Use `adjusted=false` (raw) to get the full history.
Raw prices have one-day jumps at capital increases and dividends.
