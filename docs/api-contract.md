# API contract (v1)

Status: **draft for sign-off** by backend, frontend and data. It lists the final
shape of every endpoint the frontend uses. Where it differs from the table in
[api.md](api.md), this file wins. Calculated fields come from
[calculation-layer.md](calculation-layer.md).

Each endpoint is tagged **exists** (already in `main`, shape unchanged),
**changes** (exists, shape changes) or **new**. Section 5 lists what still needs
a decision.

## Conventions

- Base path `/api/v1`. JSON in and out, UTF-8, Persian text as is.
- Field names are camelCase. Fields are never omitted: an unknown value is `null`.
- **Exchange prices** (`lastPrice`, `strikePrice`, `bestBid`, ...) are integers in Rials.
- **Calculated values** (`marketPrice`, `timeValue`, `delta`, ...) are numbers, per
  share, not per contract. Multiply by `contractSize` for a contract.
- **Volatilities and rates** are decimal fractions (`0.35` = 35%). Fields whose
  name ends in `Percent` are percentages (`2.5` = 2.5%).
- **Greeks:** `delta` and `gamma` in raw model units; `vega` per +1 volatility
  point; `theta` per calendar day; `rho` per +1 rate point.
- Dates are Gregorian `YYYY-MM-DD` (Tehran calendar day). Timestamps are RFC 3339.
- Errors: `{"error": "message"}` with `400` (bad input), `404` (not found),
  `409` (market closed), `422` (valid input that cannot be built), `502` (source down).
- **Auth:** strategy endpoints (section 2.5) are per user and need
  `Authorization: Bearer <token>`. The auth endpoints themselves are not part of
  this contract.

---

## 1. Market

### 1.1 `GET /market/status` — exists

```json
{ "marketOpen": true, "lastUpdate": "2026-11-23T10:04:12+03:30", "timezone": "Asia/Tehran" }
```

`lastUpdate` is the newest stored quote timestamp, or `null` before the first pass.

### 1.2 `GET /market/underlyings` — exists

Query: `kind` = `stock | fund | index | other` (optional).

```json
{
  "count": 27,
  "items": [
    { "id": 3, "symbol": "خودرو", "name": "ایران‌خودرو", "isin": "IRO1IKCO0001",
      "insCode": "778253364357513", "kind": "stock" }
  ]
}
```

`name`, `isin` and `insCode` can be `null`.

### 1.3 `GET /market/underlyings/:symbol` — done

This is the symbol page. The backend served it at `GET /market/symbol/:symbol`;
the frontend expects `/underlyings/:symbol`. **Decision: `/market/underlyings/:symbol`**,
because it sits under the resource `/market/underlyings` it belongs to. The old
path stays as a deprecated alias until the frontend has switched, then it is
removed. The response is unchanged:

```json
{
  "symbol": "خودرو", "name": "ایران‌خودرو", "kind": "stock",
  "price": 2450, "priceChange": 50, "priceChangePercent": 2.08,
  "expiries": [ { "expiryDate": "2026-12-23", "contractCount": 24 } ],
  "openInterest": { "calls": 120000, "puts": 80000, "pcr": 0.667 },
  "strikeOpenInterest": [ { "strikePrice": 2000, "callOpenInterest": 5000, "putOpenInterest": 1200 } ],
  "maxPain": { "expiryDate": "2026-12-23", "strikePrice": 2400, "totalPayout": 1.2e10 }
}
```

`404` when the symbol has no active option contracts. `pcr` is `null` when call
open interest is 0. `price`, `priceChange` and `priceChangePercent` can be `null`.

### 1.4 `GET /market/expiries` — done

Query: `underlying` = symbol (optional). Only expiries on or after today in Tehran,
ascending.

```json
{
  "underlyingSymbol": "خودرو",
  "items": [ { "expiryDate": "2026-12-23", "daysToExpiry": 30, "contractCount": 24 } ]
}
```

Without `underlying`, `underlyingSymbol` is `null` and the items cover the whole
market. `404` for an unknown `underlying`.

### 1.5 `GET /market/options` — done

The watch-list table: every contract with its latest quote and calculated fields.

| Query | Meaning |
|---|---|
| `underlying` | underlying symbol |
| `underlyingKind` | `stock`, `fund`, `index`, `other` |
| `type` | `call` or `put` |
| `expiry` | `YYYY-MM-DD`; only that expiry |
| `sort` | one field from the list below. Default order: `underlyingSymbol`, `expiryDate`, `strikePrice`, `optionType` |
| `order` | `asc` (default) or `desc` |
| `page` | 1-based, default `1` |
| `pageSize` | default `50`, maximum `500` |

Sortable fields: `symbol`, `underlyingSymbol`, `optionType`, `strikePrice`,
`expiryDate`, `daysToExpiry`, `lastPrice`, `volume`, `openInterest`,
`impliedVolatility`, `delta`, `leverage`, `effectiveLeverage`, `timeValue`,
`intrinsicValue`, `priceGapPercent`, `breakEvenPercent`, `bestBid`, `value`,
`moneyness`, `breakEven`. `null` values sort last in both directions. An unknown
`sort` is `400`.

`limit` is a deprecated alias for `pageSize`; when both are supplied,
`pageSize` takes precedence.

```json
{
  "items": [ { "...": "see below" } ],
  "count": 50, "page": 1, "pageSize": 50, "total": 1800, "totalPages": 36
}
```

`count` is the number of items on this page (it was the only count before). An
`expiry` or `page` with no rows gives an empty `items`, not `404`.

Item:

```json
{
  "contractId": 101, "symbol": "ضخود1234",
  "underlyingId": 3, "underlyingSymbol": "خودرو", "underlyingKind": "stock",
  "optionType": "call", "strikePrice": 2000, "expiryDate": "2026-12-23", "contractSize": 1000,
  "lastPrice": 520, "closePrice": 510, "bestBid": 515, "bestAsk": 525,
  "volume": 12000, "value": 6240000, "openInterest": 45000, "prevClose": 500,
  "quoteTs": "2026-11-23T10:04:12+03:30",

  "underlyingPrice": 2450, "daysToExpiry": 30, "moneyness": "ITM",
  "marketPrice": 520, "priceSource": "last",
  "intrinsicValue": 450, "timeValue": 70,
  "breakEven": 2520, "breakEvenPercent": 2.86,
  "leverage": 4.71, "effectiveLeverage": 3.9,
  "impliedVolatility": 0.38, "historicalVolatility": 0.41, "greeksVolatility": 0.38,
  "delta": 0.83, "gamma": 0.0012, "vega": 1.9, "theta": -2.1, "rho": 0.4,
  "theoreticalPrice": 531.2, "priceGap": -11.2, "priceGapPercent": -2.11,
  "metricsTs": "2026-11-23T10:04:12+03:30"
}
```

**Calculated fields.** All of them (from `underlyingPrice` to `metricsTs`) are
`null` for a contract that has no metrics yet. Individually they are `null` as
described in [calculation-layer.md](calculation-layer.md): for example
`impliedVolatility` when the quote is stale, and everything price-based when the
contract has no usable price.

| Field | Type | Meaning |
|---|---|---|
| `underlyingPrice` | number | spot used |
| `daysToExpiry` | integer | calendar days, `0` on expiry day |
| `moneyness` | `"ITM"`, `"ATM"`, `"OTM"` | |
| `marketPrice` | number | premium the numbers are based on |
| `priceSource` | `"last"`, `"mid"`, `"close"` | where `marketPrice` came from |
| `intrinsicValue`, `timeValue` | number | |
| `breakEven`, `breakEvenPercent` | number | break-even price; move needed from spot, in percent |
| `leverage`, `effectiveLeverage` | number | `spot / premium`; `delta * leverage` |
| `impliedVolatility` | number | |
| `historicalVolatility` | number | HV20 of the underlying |
| `greeksVolatility` | number | volatility the Greeks used (implied, else HV20) |
| `delta`, `gamma`, `vega`, `theta`, `rho` | number | units in Conventions |
| `theoreticalPrice` | number | Black-Scholes at HV20 |
| `priceGap`, `priceGapPercent` | number | `marketPrice - theoreticalPrice`, and that in percent |
| `metricsTs` | timestamp | when they were calculated |

### 1.6 `GET /market/chain` — done

One expiry's strikes, call and put side by side. Query: `underlying` (required),
`expiry` (required, `YYYY-MM-DD`). `400` if either is missing or malformed, `404`
if there are no active contracts for them.

```json
{
  "underlyingSymbol": "خودرو", "expiryDate": "2026-12-23",
  "daysToExpiry": 30, "underlyingPrice": 2450,
  "strikes": [
    { "strikePrice": 2000,
      "call": { "contractId": 101, "symbol": "ضخود1234", "lastPrice": 520, "closePrice": 510,
                "bestBid": 515, "bestAsk": 525, "volume": 12000, "value": 6240000,
                "openInterest": 45000, "prevClose": 500,
                "quoteTs": "2026-11-23T10:04:12+03:30",
                "moneyness": "ITM", "marketPrice": 520, "priceSource": "last", "...": "..." },
      "put": null }
  ]
}
```

New: `daysToExpiry` and `underlyingPrice` at the top (integer Rials; `null` without
an underlying quote), and the calculated fields from 1.5 on each side. Because
the first two are hoisted, a side does **not** repeat `underlyingPrice` or
`daysToExpiry`. A side that is not listed is `null`.

### 1.7 Operational endpoints — exists, not for the frontend

`POST /market/collect/options`: runs one pass; `409` when the market is closed.
The response gains `metrics` (rows of calculated metrics stored):
`{ "source", "ts", "rows", "underlyings", "contracts", "quotes", "metrics", "errors", "firstError" }`.

`POST /market/collect/daily/:insCode?adjusted=true`: backfills daily prices;
`{ "insCode", "adjusted", "days" }`.

### Market API additions implemented with this contract

- Every chain side includes `contractId` for strategy-leg selection.
- `/market/options` items and chain sides include `value` (Rials) and
  `prevClose` (Rials). Historical quotes written before migration 000011 have
  `prevClose: null`.
- `/market/options` adds the `bestBid`, `value`, `moneyness`, and `breakEven`
  sort keys.
- `/market/options` accepts deprecated `limit` as an alias for `pageSize`;
  `pageSize` wins if both are provided.
- CORS origins are configured with `CORS_ALLOWED_ORIGINS`, a comma-separated
  list. The default allows `http://localhost:3000` and `http://localhost:5173`.

---

## 2. Strategies

A strategy is a set of legs on one underlying and one expiry. Everything is
computed at expiry from the legs' prices; fees and taxes are ignored.

### 2.1 Leg

Request:

```json
{ "contractId": 101, "side": "buy", "quantity": 1, "price": 520 }
```

or, for the shares behind a covered call or protective put:

```json
{ "underlyingId": 3, "side": "buy", "quantity": 1000, "price": 2450 }
```

- Exactly one of `contractId` and `underlyingId`. `side` is `buy` or `sell`.
- `quantity` is a number of **contracts** for an option leg and a number of
  **shares** for an underlying leg. It must be at least 1.
- `price` is optional, per share, integer Rials. When it is missing the server
  uses the contract's current `marketPrice` (the underlying's last price for an
  underlying leg); it is always filled in the response, and always stored.
- All option legs of one strategy must have the same underlying and expiry (`422`).

Response leg: the request fields (with `price` filled in) plus
`type` (`"call"`, `"put"` or `"underlying"`), `symbol`, `strikePrice` (`null` for
an underlying leg), `expiryDate` (`null` for an underlying leg) and
`contractSize` (`null` for an underlying leg).

### 2.2 Analysis

Returned by 2.3, 2.4 and 2.5.

```json
{
  "netPremium": 3200000,
  "maxProfit": 1800000,
  "maxLoss": 3200000,
  "breakEvens": [2520],
  "payoff": [ { "underlyingPrice": 1225, "pnl": -3200000 } ],
  "greeks": { "delta": 420, "gamma": 0.6, "vega": 950, "theta": -1100, "rho": 200 }
}
```

- `netPremium`: sum over legs of `±price × quantity × contractSize` (a share leg
  uses `quantity` alone), in Rials. Positive: you pay (debit). Negative: you receive (credit).
- `maxProfit`, `maxLoss`: over every underlying price from 0 upwards, in Rials.
  `maxLoss` is a positive amount at risk. Either is `null` when unbounded.
- `breakEvens`: underlying prices where the payoff at expiry is 0, ascending.
- `payoff`: profit or loss at expiry, ascending in `underlyingPrice`: 41 evenly
  spaced points from 50% to 150% of the current underlying price, plus every leg strike.
- `greeks`: position Greeks, summed over legs as `±quantity × contractSize × greek`
  (an underlying leg contributes delta `±quantity`). `null` if any option leg has
  no Greeks.

### 2.3 `GET /strategies/templates` — new

The strategy templates. Public.

```json
{
  "count": 9,
  "items": [
    { "id": "bull-call-spread", "name": "Bull call spread", "nameFa": "بول کال اسپرد",
      "outlook": "bullish",
      "legs": [
        { "type": "call", "side": "buy",  "strike": { "rule": "atm" },              "ratio": 1 },
        { "type": "call", "side": "sell", "strike": { "rule": "otm", "steps": 1 }, "ratio": 1 }
      ] }
  ]
}
```

`outlook` is `bullish`, `bearish`, `neutral` or `volatile`. `type` is `call`,
`put` or `underlying` (an underlying leg has no `strike`). Strike rules act on the
strikes listed for the chosen expiry, not on percentages:

| Rule | Meaning |
|---|---|
| `{ "rule": "atm" }` | the listed strike closest to the underlying price (the lower one on a tie) |
| `{ "rule": "otm", "steps": n }` | `n` listed strikes further out of the money than the ATM strike: higher for a call, lower for a put |
| `{ "rule": "itm", "steps": n }` | `n` listed strikes further into the money than the ATM strike |

`ratio` scales the leg against the `quantity` of the build request.

Draft list (see section 5, item 3):

| `id` | Legs |
|---|---|
| `long-call` | buy call ATM |
| `long-put` | buy put ATM |
| `covered-call` | buy underlying; sell call OTM 1 |
| `protective-put` | buy underlying; buy put ATM |
| `collar` | buy underlying; buy put OTM 1; sell call OTM 1 |
| `bull-call-spread` | buy call ATM; sell call OTM 1 |
| `bear-put-spread` | buy put ATM; sell put OTM 1 |
| `long-straddle` | buy call ATM; buy put ATM |
| `long-strangle` | buy call OTM 1; buy put OTM 1 |

### 2.4 Build and analyze — new

`POST /strategies/build` creates concrete legs from a template. Nothing is saved.

```json
{ "templateId": "bull-call-spread", "underlying": "خودرو", "expiry": "2026-12-23", "quantity": 1 }
```

`quantity` is contracts per leg unit (default 1). An underlying leg gets
`ratio × quantity × contractSize` shares. Response: `{ "templateId", "name", "legs": [leg], "analysis": {…} }`.
`404` for an unknown template or underlying; `422` when a strike rule has no
listed strike, or a leg has no price.

`POST /strategies/analyze` analyses legs the user assembled. Nothing is saved.

```json
{ "legs": [ { "contractId": 101, "side": "buy", "quantity": 1 }, { "contractId": 102, "side": "sell", "quantity": 1 } ] }
```

Response: `{ "legs": [leg], "analysis": {…} }`. `400` for malformed legs,
`404` for an unknown contract, `422` for mixed expiries or underlyings.

### 2.5 Saved strategies — new, needs auth

| Method and path | Body | Response |
|---|---|---|
| `GET /strategies` | | `{ "count", "items": [ { "id", "name", "createdAt", "legCount" } ] }`, newest first |
| `POST /strategies` | `{ "name", "legs": [leg] }` | `201`, strategy |
| `GET /strategies/:id` | | `200`, strategy |
| `PUT /strategies/:id` | `{ "name", "legs": [leg] }`, replaces both | `200`, strategy |
| `DELETE /strategies/:id` | | `204` |

Strategy: `{ "id", "name", "createdAt", "legs": [leg], "analysis": {…} }`. The
analysis uses the saved leg prices for `netPremium` and `payoff`, and current
Greeks. Another user's id is `404`. `name` must not be empty (`400`).

---

## 3. Signals and backtests

Both are written by the data pipeline into the shared database
(`signals`, `backtest_runs`); the backend only reads them.

### 3.1 `GET /signals` — new

Query: `underlying` (symbol), `contractId`, `action` (`enter` or `exit`),
`limit` (default 50, maximum 200). Newest first.

```json
{
  "count": 1,
  "items": [
    { "id": 15, "underlyingId": 3, "underlyingSymbol": "خودرو",
      "contractId": null, "contractSymbol": null,
      "modelName": "rsi-meanrev-v1", "action": "enter", "direction": "long",
      "entryPrice": 2450, "targetPrice": 2600, "stopPrice": 2380,
      "confidence": 0.62, "generatedAt": "2026-11-23T10:02:00+03:30",
      "reason": { "rsi": 28.4 } }
  ]
}
```

`direction`, the three prices, `confidence` (0 to 1) and `reason` can be `null`.
`reason` is free-form JSON from the model; the frontend only displays it.

### 3.2 `GET /backtests` — new

Query: `underlying` (`params.underlying`), `template` (the ID after `strategy:`),
`model` (exact `modelName`), and `limit` (default `100`, maximum `500`). Filters
may be combined. Runs are newest first.

```json
{
  "count": 1,
  "items": [
    { "id": 4, "modelName": "strategy:bull-call-spread", "template": "bull-call-spread",
      "underlying": "خودرو", "startDate": "2023-11-01", "endDate": "2026-11-01",
      "winRate": 0.56, "maxDrawdown": 0.18, "totalReturn": 0.42,
      "tradeCount": 90, "avgPnl": 1250000, "modelPricedShare": 1.0,
      "createdAt": "2026-11-10T09:00:00+03:30" }
  ]
}
```

`template` is `null` for a run whose `modelName` lacks the `strategy:` prefix.
`underlying` is `null` when `params.underlying` is absent. `tradeCount` is `0`
when `metrics` is null or has no `trades` key; `avgPnl` and `modelPricedShare`
are `null` in that case.

### 3.3 `GET /backtests/:id` — new

The list summary above plus `params`, `metrics`, `trades`, `skipped`, and
`equityCurve`.

```json
{
  "id": 4, "modelName": "rsi-meanrev-v1", "startDate": "2023-11-01", "endDate": "2026-11-01",
  "winRate": 0.56, "maxDrawdown": 0.18, "totalReturn": 0.42, "createdAt": "2026-11-10T09:00:00+03:30",
  "template": "bull-call-spread", "underlying": "خودرو",
  "tradeCount": 90, "avgPnl": 1250000, "modelPricedShare": 1.0,
  "params": { "underlying": "خودرو", "templateId": "bull-call-spread" },
  "metrics": { "trades": 90, "winRate": 0.56, "avgPnl": 1250000 },
  "trades": [ { "expiry": "2026-12-23", "entryDate": "2026-11-23",
                "legs": [ { "entryPrice": 150, "priceSource": "snapshot" } ] } ],
  "skipped": [ { "expiry": "2026-12-24", "reason": "no listed chain" } ],
  "equityCurve": [ { "date": "2023-11-01", "equity": 1.0 }, { "date": "2023-11-02", "equity": 1.004 } ]
}
```

Units: `winRate` is a fraction from 0 to 1; `totalReturn` is a fraction (`0.42` =
+42%); `maxDrawdown` is a positive fraction (`0.18` = a peak-to-trough fall of
18%); `equity` starts at `1.0`. The data pipeline must write
`backtest_runs.equity_curve` in exactly the `[{date, equity}]` shape above.

A backtest here is a backtest of a signal model. There is no backtest of a
strategy template: the database has no history of option prices to run one on.
There is no `POST /backtests`; runs come from the pipeline. `404` for an unknown id.

---

## 4. Not in this contract

Auth (`/auth/*`), the portfolio (`portfolio_trades`) and the `tse` / `optionbaaz`
sources.

## 5. Needs a decision

1. **Strategy templates (2.3), data team.** Names, legs and strike rules above are a
   draft, written without the data team. They need to confirm the list, the Persian
   names, and that "n strikes out of the money" is how strikes should be picked.
2. **Backtests (3), data team.** Read-only; the `equity_curve` shape and the units of
   `winRate`, `totalReturn` and `maxDrawdown` above must match what the pipeline writes.
3. **Auth.** Strategy endpoints need a token (Conventions). The frontend currently
   calls `http://1.2.3.4:8000` with `/v1/auth/login`; the backend serves
   `/api/v1` on port 8080 (`APP_PORT`). The base URL and path need to be aligned.
4. **Calculation defaults** (see calculation-layer.md): risk-free rate 0.30, and the
   option price priority last, then mid, then close.
