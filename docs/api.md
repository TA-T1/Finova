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

| Method | Path | Description |
|---|---|---|
| `GET`  | `/health` | Liveness + DB ping |
| `POST` | `/market/collect/options` | Run one option-chain collection pass; returns counts (`rows`, `underlyings`, `contracts`, `quotes`, `errors`) |
| `POST` | `/market/collect/daily/:insCode?adjusted=true` | Backfill an underlying's daily history |
| `GET`  | `/market/options?underlying=<sym>&type=call\|put&underlyingKind=stock\|fund\|index\|other&limit=<n>` | Stored contracts joined with their latest quote |
| `GET`  | `/market/underlyings?kind=stock\|fund\|index\|other` | Known underlyings (optionally filtered by kind) |

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

TSETMC's `GET /ClosingPrice/GetClosingPriceDailyList/{insCode}/{adjusted}` is keyed by instrument code,
and `Client.DailyPrices` already accepts any code, so it should also work with an option contract's `insCode`.
**This has not been verified against a real option `insCode`.** It matters for the Sprint 3 decision, so check it first.
The chain endpoint above returns only the current day.

### Known limitations and future work

| Item | Status |
|---|---|
| `LastTradeTime`, underlying `Name` and `Volume` | **Filled by the enrichment requests** (decision 2026-11-15: first left unfilled, then fixed after finding the endpoints in the browser). They are unset only when those requests fail, or for a contract that did not trade (`nil`), or an underlying missing from the market watch (`Name` = `Symbol`, `Volume` = `0` meaning *unknown*). Callers should not store `Volume` = 0 as a real value. |
| Trade date on a holiday | **Unverified.** The enrichment takes the date from `GetClosingPriceInfo.dEven`, and it was only observed on a trading day (2026-11-15). We expect it to stay at the last session's date on Thursday, Friday and holidays, but nobody has checked yet. The snapshot has a guard for this case (see "How a non-trading day is detected"). Check the first closed weekday after deployment. |
| Cost | Each snapshot now downloads about 3.7 MB more (the market watch). At a 30-60 s polling cadence, consider calling it less often or caching the names. |
| Names of contracts' underlyings not in the market watch | Fall back to the symbol. The collector gets full names from `GetInstrumentInfo` when it creates an underlying. |
| Real samples | `backend/testdata/tsetmc_market_watch.json` is the real market watch, **trimmed** to the instruments in the option chain (1,458 rows) with the best-limits ladder (`blDs`) removed, to keep it at about 0.8 MB. `tsetmc_closing_price_info.json` is the closing info of اهرم. `capture_tsetmc_samples.sh` fetches the untrimmed originals. |

## Daily end-of-day snapshot (`cmd/snapshot`, `internal/snapshot`)

`quote_snapshots` keeps one row per contract per trading day (migration
`000007_quote_snapshots`; **new migration, the backend team should review it**).
`option_quotes` holds intraday ticks only, so this table is our real price history for the backtest.
It may be the only history we get (see "Historical option prices" above).

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
underlying price, and upserts all rows (about 1,800, about 100 ms) in **one transaction** with one bulk
statement. A second run on the same day overwrites the first run's values and creates no duplicates.
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
