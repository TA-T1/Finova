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

`backend/testdata/market_sample.json`: the 27 underlyings are real latest quotes
from `data-pipeline/output/underlyings_daily.csv` (2026-11-07). The 36 contracts
are **synthetic** (`source_id` starts with `SYNTH`): they exist only to give
the right shape and are not real market prices.
