# Market watch API

## What was asked

Bring the backend market watch, option chain, expiry, and symbol endpoints in
line with `docs/api-contract.md` and the frontend's market types. Keep the
frontend untouched, retain the deprecated symbol path, and add the frontend's
requested quote fields and CORS support.

## What was built

| Area | Change |
|---|---|
| `GET /market/options` | Added validated filters, sorting, nulls-last behavior, stable contract-id tie breaking, and database-side paging with `total`/`totalPages`. Returns the latest quote and a left-joined `option_metrics_latest` record; calculated values are nullable. `limit` remains a deprecated alias. |
| `GET /market/chain` | Preserved `strikes[{strikePrice, call, put}]`, added top-level Tehran `daysToExpiry` and latest underlying quote price, and added metrics plus `contractId`, `value`, and `prevClose` to each side. Missing metrics and sides remain `null`. |
| `GET /market/expiries` | Added market-wide or underlying-filtered active expiries, contract counts, and Tehran calendar days to expiry. |
| Symbol page | Confirmed `/market/underlyings/:symbol` and deprecated `/market/symbol/:symbol` use the same response handler. |
| Previous close | Added migration `000011_option_quote_prev_close`, writes from `OptionLeg.Yesterday`, and recreated `option_quotes_latest` to expose the new column. Unknown and historical previous closes stay `null`. |
| CORS | Added `CORS_ALLOWED_ORIGINS` (comma-separated; localhost ports 3000 and 5173 by default) and middleware for configured origins. |
| Documentation | Updated the endpoint table and contract additions. |

No new latest-view indexes were needed: migrations 000001 and 000009 already
create `(contract_id, ts DESC)` indexes for the quote and metrics `DISTINCT ON`
views.

## Decisions

- The SQL sort expressions are a fixed whitelist. `pageSize` wins when both it
  and the deprecated `limit` are supplied.
- The total count and requested page use separate SQL queries, so an empty page
  past the end still reports the matching total.
- Chain `underlyingPrice` uses the latest underlying quote's last price, falling
  back to its close; it is `null` when neither is available.
- The migration uses `ADD COLUMN IF NOT EXISTS` and `CREATE OR REPLACE VIEW` so
  000011 can be applied repeatedly after 000009.

## How it was verified

- `go build ./...`
- `go vet ./...`
- `go test ./...`
- Unit and `httptest` coverage includes parameter defaults/validation, all
  whitelisted sort keys, the limit alias, bad dates, 400 responses, and CORS.
- Database integration tests cover filters, totals and empty pages, nulls-last
  sorting in both directions, stable page boundaries, Tehran expiry counts,
  chain metrics/quote fields, and reapplying/reversing migration 000011.

The database tests require `TEST_DATABASE_URL` and skip when it is unset. No
usable test database was available in the implementation environment, so the
PostgreSQL integration tests, migration execution, and realistic-data
`EXPLAIN ANALYZE` were not run here. The SQL tests are ready to run against a
throwaway database.

## Frontend and integration notes

- Set `VITE_USE_MOCK=false` and point `VITE_API_BASE_URL` to this backend.
- `/market/options` accepts `underlying`, `underlyingKind`, `type`, `expiry`,
  `sort`, `order`, `page`, and `pageSize` (or deprecated `limit`).
- The chain side's `contractId` can be used directly by the strategy builder.
- `value` and `prevClose` are now available on watch rows and chain sides.
  Existing quote snapshots pre-dating 000011 do not have a previous close.
- Add the frontend origin to `CORS_ALLOWED_ORIGINS` if it is not one of the
  localhost defaults.
- Apply `backend/migrations/000011_option_quote_prev_close.up.sql` after the
  migrations currently in `main`.

## Known limits

- The API returns metrics only when a collection pass has stored an
  `option_metrics` row; until then all calculated fields are `null`.
- Previous-close values are not backfilled for old option quote snapshots.
- Migration 000008 belongs to the unmerged daily snapshot work. Existing
  databases will need that migration applied by hand when that branch merges.
- Performance was not measured with `EXPLAIN ANALYZE` because no usable test
  database was available.
