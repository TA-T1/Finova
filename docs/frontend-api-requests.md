# What the market pages need from the backend

The **market watch** (`/market-watch`) and **option chain** (`/chain`) pages are built and
work on mock data (`VITE_USE_MOCK=true`, the default). To connect them to the real server,
the backend has to provide the endpoints and fields below. Nothing else on the frontend
changes: set `VITE_USE_MOCK=false` (and `VITE_API_BASE_URL` if needed).

Everything marked **new** or **extend** does not exist today. The real-API code path in
`frontend/src/services/marketService.ts` follows this document, but it has **not been
tested against a server**.

- Source of truth for the shapes: `frontend/src/types/marketTypes.ts`
- The mock that behaves like the server: `frontend/src/mocks/`

## 1. Endpoints

All paths are under `/api/v1/market` (the router prefix in `backend/internal/httpapi/router.go`).

| # | Method + path | Status | Used by |
|---|---|---|---|
| 1 | `GET /options` | **extend** (exists) | market watch table |
| 2 | `GET /chain?underlying=&expiry=` | **new** | option chain table |
| 3 | `GET /underlyings/:symbol` | **new** | chain top bar: price, expiries |
| 4 | `GET /expiries` | **new** | expiry filter of the market watch |
| 5 | `GET /underlyings` | exists, **no change** | the underlying selects |

Please keep the current behaviour of `/options` working (`underlying`, `type`,
`underlyingKind`, `limit`, and the `{count, items}` envelope), because other code uses it.

### 1. `GET /options` (extend)

Query parameters:

| Param | Status | Meaning |
|---|---|---|
| `underlying` | exists | underlying symbol, exact match, e.g. `خودرو` |
| `type` | exists | `call` or `put` |
| `underlyingKind` | exists | `stock`, `fund`, `index`, `other` |
| `expiry` | **new** | `YYYY-MM-DD`; only that expiry |
| `page` | **new** | 1-based, default 1 |
| `pageSize` | **new** | default 25, max 200 (replaces `limit`; keep `limit` working) |
| `sortBy` | **new** | one of the keys below |
| `sortDir` | **new** | `asc` or `desc` |

`sortBy` values: `symbol`, `underlyingSymbol`, `optionType`, `strikePrice`, `expiryDate`,
`lastPrice`, `bestBid`, `volume`, `value`, `openInterest`, `impliedVolatility`, `delta`,
`priceGapPercent`, `breakEven`, `leverage`, `moneyness`.

- **Sorting happens on the server over ALL matching rows, before paging.**
- **Nulls go last in both directions** (`NULLS LAST`).
- Ties are broken by `contractId`, so a page never repeats or skips a row.
- `moneyness` sorts as `ITM`, `ATM`, `OTM`.
- A page beyond the last one returns `items: []` with the real `total` (not an error).

Response: the existing envelope plus paging fields.

```json
{
  "count": 25,
  "total": 144,
  "page": 1,
  "pageSize": 25,
  "items": [ { "...one OptionQuote, see section 2..." } ]
}
```

### 2. `GET /chain?underlying=خودرو&expiry=2026-11-27` (new)

Both sides (calls and puts) of one underlying for one expiry. Same item shape as `/options`.
No paging (at most about 40 strikes x 2). Sorted by `strikePrice` ascending, call before put.

```json
{ "count": 12, "items": [ { "...OptionQuote..." } ] }
```

- Unknown underlying: HTTP 404, `{"error": "..."}`.
- Known underlying but no contracts for that expiry: HTTP 200, `{"count": 0, "items": []}`.

### 3. `GET /underlyings/:symbol` (new)

The existing underlying fields, plus the price and the list of expiries. The `:symbol` is
URL-encoded Persian text.

```json
{
  "id": 1,
  "symbol": "خودرو",
  "name": "ایران‌خودرو",
  "isin": "IRO1IKCO0001",
  "insCode": "778253364357513",
  "kind": "stock",
  "lastPrice": 712,
  "closePrice": 709,
  "prevClose": 717,
  "expiryDates": ["2026-11-27", "2026-12-25", "2027-01-22"]
}
```

- `expiryDates`: distinct expiries that have contracts, **ascending**, `YYYY-MM-DD`.
- Unknown symbol: HTTP 404, `{"error": "..."}`.
- The chain page uses `lastPrice` for the in-the-money highlighting and `prevClose` for the
  % change. Both come from the latest underlying quote (`underlying_quotes`).

### 4. `GET /expiries` (new)

All distinct expiries across all contracts, ascending, for the expiry filter.

```json
{
  "count": 6,
  "items": [
    { "expiryDate": "2026-11-27", "contractCount": 54 },
    { "expiryDate": "2026-12-11", "contractCount": 12 }
  ]
}
```

## 2. The contract object (`OptionQuote`)

Used by `/options` and `/chain`. Fields marked **exists** are in `docs/api.md` and
`backend/internal/market/repository.go` today (camelCase, as they are).

| Field | Type | Status | Notes |
|---|---|---|---|
| `contractId` | int | exists | |
| `symbol` | string | exists | e.g. `ضخود9001` |
| `underlyingId` | int | exists | |
| `underlyingSymbol` | string | exists | |
| `underlyingKind` | string | exists | |
| `optionType` | `"call"`/`"put"` | exists | |
| `strikePrice` | int | exists | rial |
| `expiryDate` | string | exists | ISO; a DATE, so `"2026-11-27T00:00:00Z"`. Only the `YYYY-MM-DD` part is used |
| `contractSize` | int | exists | |
| `lastPrice` | int or null | exists | |
| `closePrice` | int or null | exists | |
| `bestBid` | int or null | exists | |
| `bestAsk` | int or null | exists | |
| `volume` | int or null | exists | |
| `openInterest` | int or null | exists | |
| `quoteTs` | string or null | exists | ISO timestamp |
| `prevClose` | int or null | **new** | previous day's price, for the % change. The TSETMC field is `priceYesterday` |
| `value` | int or null | **new** | traded value today, rial (`qTotCap`) |
| `impliedVolatility` | number or null | **new** | decimal fraction: `0.35` = 35% |
| `delta` | number or null | **new** | call 0..1, put -1..0 |
| `theoreticalPrice` | number or null | **new** | Black-Scholes price at the historical volatility, per share, rial |
| `priceGapPercent` | number or null | **new** | `(market - theoretical) / theoretical * 100`; **already a percentage** |
| `breakEven` | number or null | **new** | rial |
| `leverage` | number or null | **new** | spot / premium; null when the premium is 0 |
| `moneyness` | `"ITM"`/`"ATM"`/`"OTM"` or null | **new** | |

All the **new calculated fields already exist in the calculation engine**: they are the
output of `options.Evaluate` in `backend/internal/options/evaluate.go`. The engine is not
exposed over HTTP yet. The names above match it:

| Field | Engine field |
|---|---|
| `impliedVolatility` | `Evaluation.ImpliedVolatility` (null when `HasImpliedVolatility` is false) |
| `delta` | `Evaluation.Greeks.Delta` |
| `theoreticalPrice` | `Evaluation.TheoreticalPrice` (null when `HasTheoreticalPrice` is false) |
| `priceGapPercent` | `Evaluation.PriceGapPercent` |
| `breakEven` | `Evaluation.Metrics.BreakEven` |
| `leverage` | `Evaluation.Metrics.Leverage` (the engine uses 0 for "undefined"; please send null) |
| `moneyness` | `Evaluation.Metrics.Moneyness` |

Example item:

```json
{
  "contractId": 1,
  "symbol": "ضخود9101",
  "underlyingId": 1,
  "underlyingSymbol": "خودرو",
  "underlyingKind": "stock",
  "optionType": "call",
  "strikePrice": 700,
  "expiryDate": "2026-11-27T00:00:00Z",
  "contractSize": 1000,
  "lastPrice": 34,
  "closePrice": 33,
  "bestBid": 33,
  "bestAsk": 36,
  "volume": 57395,
  "openInterest": 184614,
  "quoteTs": "2026-11-15T09:00:01Z",
  "prevClose": 32,
  "value": 1900000000,
  "impliedVolatility": 0.464,
  "delta": 0.64,
  "theoreticalPrice": 32.0,
  "priceGapPercent": 6.2,
  "breakEven": 734,
  "leverage": 20.9,
  "moneyness": "ITM"
}
```

## 3. Conventions the frontend relies on

- **Prices are integers in rial**, per share. The UI says "قیمت‌ها به ریال". If the server
  will ever send toman, tell us: the UI has to change its label and formatting.
- **A missing value is `null`**, never `0` (the Go structs already use pointers). The UI shows
  "—" for null.
- **Trade value** and other large numbers fit in a JS number (below 2^53); no strings needed.
- **The error body** is `{"error": "message"}` with a suitable HTTP status. The UI shows the
  `error` text to the user, so please keep it short and in Persian or neutral.
- **Days to expiry** is computed on the client from `expiryDate` and today's date in
  `Asia/Tehran`; no field is needed.
- **% change of a price** is computed on the client: `(lastPrice - prevClose) / prevClose`.
- **Call in the money** on the chain page means `strikePrice < underlying lastPrice`; **put**
  means `strikePrice > underlying lastPrice`. The market-watch status column shows the
  server's `moneyness` (which also has `ATM`, a +-0.5% band around the price).

## 4. Things to sort out between the two teams

1. **CORS.** `backend/internal/httpapi/router.go` has no CORS middleware. A browser page on
   `http://localhost:3000` cannot call the Go server on `:8080` until it has one (for example
   `github.com/gin-contrib/cors`), or until the frontend dev server proxies `/api`. Which one?
2. **Base URL and path prefix.** The frontend defaults to `http://localhost:8080` and the
   `/api/v1/market/...` paths above. The existing login service in the frontend calls
   `/v1/auth/login` (no `/api`), which does not match the router's `/api/v1`. Please confirm
   the real prefix for all endpoints.
3. **Pagination of `/options` replaces `limit`.** Is it fine to keep `limit` as an alias of
   `pageSize` for the old callers?
4. **Calculated fields in a list endpoint.** `options.Evaluate` has to run for every contract
   on every request (about 1,800 rows when unfiltered), including the implied-volatility
   solver. If that is too slow, compute them in the collector and store them next to the
   quote. The frontend does not care which, only that the fields are present.
5. **Spot and historical volatility.** `Evaluate` needs the underlying's current price
   (available from `underlying_quotes`) and a historical volatility (the "data team"
   product mentioned in `evaluate.go`). If the historical volatility is not available yet,
   `theoreticalPrice` and `priceGapPercent` can be null; the UI handles it.
6. **Stale prices.** In the real TSETMC data, a contract with `volume = 0` still has a
   non-zero `lastPrice` carried over from earlier days. Should the API send `null` for the
   last price (and the calculated fields) in that case? The UI shows whatever it is sent.

## 5. Gaps and assumptions on the frontend side

For reference, so nothing is hidden:

- `docs/api.md` defines only `GET /market/options` and `GET /market/underlyings`, and gives no
  JSON examples. The shapes of those two were taken from the Go JSON tags in
  `backend/internal/market/repository.go`.
- Everything marked **new** or **extend** above is an assumption made by the frontend. The
  names follow the existing backend style (camelCase) and the engine in
  `backend/internal/options`.
- The sort keys are the field names, so the UI sends `sortBy=lastPrice` for the "last price"
  column. The combined columns sort by one key: "last price / % change" by `lastPrice`,
  "bid / ask" by `bestBid`, "Black-Scholes price / difference" by `priceGapPercent`,
  "expiry / days left" by `expiryDate`.
