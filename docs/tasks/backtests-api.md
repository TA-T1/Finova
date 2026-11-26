# Backtest read API

## What was asked

Serve saved strategy backtest runs to the frontend from
`backtest_runs`. Keep the API read-only, add list filters and a detail endpoint,
and convert the engine's snake_case JSON keys recursively to camelCase.

## What was built

- Added `backend/internal/backtest` with repository, handler, key conversion,
  and tests.
- Added `GET /api/v1/backtests` with `underlying`, `template`, `model`, and
  `limit` query parameters.
- Added `GET /api/v1/backtests/:id`.
- Registered the package under `/api/v1/backtests` in the API router.
- Documented both endpoints in `docs/api.md` and `docs/api-contract.md`.

The list response contains `count` and `items`. Items always include
`template`, `underlying`, dates, metric summary fields, and `createdAt`; absent
values are JSON `null`.

## Decisions

- `limit` defaults to `100` and is rejected outside `1..500`.
- Combined filters use fixed SQL predicates:
  `params->>'underlying'`, `model_name = 'strategy:' || template`, and exact
  `model_name`.
- `tradeCount` is `0` when `metrics` is null or has no `trades` key because it
  is a required number in the frontend type. `avgPnl` and `modelPricedShare`
  are `null` in that case.
- `params`, `metrics`, `trades`, `skipped`, and `equityCurve` are decoded from
  JSONB and recursively convert snake_case keys to camelCase.
- Null `trades`, `skipped`, and `equityCurve` arrays return `[]`; null
  `metrics` returns `null`.
- Detail IDs are rejected as non-integer by `400`; unknown integer IDs return
  `404`.

## Example requests

```bash
curl "http://localhost:8080/api/v1/backtests?underlying=%D8%AE%D9%88%D8%AF%D8%B1%D9%88&template=long-call&limit=20"
curl "http://localhost:8080/api/v1/backtests/1"
```

## Verification

Verification includes unit tests for query parsing, key conversion, status
responses, null metrics, combined repository filters, detail 404, and a
snake_case stored run compared against the frontend mock fixture.

Checks required for completion:

```bash
go build ./...
go vet ./...
go test ./...
```

Database tests use `TEST_DATABASE_URL` and throwaway schemas; they skip when
the variable is unset. The temporary PostgreSQL 17 database used for this work
can apply every migration from `backend/migrations/*.up.sql`.

## Frontend integration

- Set mock mode off and call `getBacktests(params)` and `getBacktest(id)`.
- The existing frontend types and mock response shape match the list/detail
  fields.
- Strategy runs use `modelName` values prefixed with `strategy:`; signal-model
  runs keep `template` and `underlying` as `null`.
- Nested engine fields arrive in camelCase, including trade legs.

## Known limits

- The API is read-only; no POST endpoint is implemented.
- JSONB data is not schema-validated. Malformed JSON cannot be read as a valid
  array/object and is logged/reported as a server error.
- Trade/metric values are returned as stored; the backend does not recompute or
  normalize engine output.
