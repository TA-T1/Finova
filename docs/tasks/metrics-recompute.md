# Offline metrics recompute

## What was asked

Add a database-only command to populate `option_metrics` for quote datasets
that were imported without calculated metrics. It must use the same calculation
and storage path as live collection, ignore market hours, default each row's
timestamp to its quote timestamp, and support dry runs and a timestamp override.

## What was built

- Added `go run ./cmd/metrics recompute [-at <RFC3339>] [-dry-run]`.
- Added `Repository.LatestQuotesForMetrics` to load active contracts with their
  latest option quote and latest underlying quote.
- Added input construction with the same last-price-then-close spot selection
  as live collection. Contracts without a quote, positive strike, supported
  option type, or usable spot are logged as skipped with a reason.
- Moved HV20 loading/calculation into a shared market helper and routed both
  the collector and the command through `ComputeAndStoreMetrics`.
- The command logs the counts read, calculated, skipped by reason, and stored.
  Dry-run performs all reads and calculations but does not write.
- Updated the API and calculation-layer docs. The demo import/export was not
  changed to include `option_metrics`; after this command merges, the demo data
  can be regenerated with metrics by importing it and running recompute.

## Decisions

- Active contracts are selected using today's Asia/Tehran date. `-at` changes
  calculation timestamps but does not change which contracts are active or
  which quote rows are loaded.
- Quote timestamps are kept per contract by default, rather than assigning one
  current timestamp to the entire recompute pass. This matches the quote/metric
  key and makes repeat runs idempotent.
- A missing underlying daily history or an HV20 database read failure is logged;
  other metrics do not depend on HV20, matching live collector behavior.
- `-dry-run` still loads quotes and daily closes and computes every valid row,
  but skips the upsert.

## How it was verified

- `go build ./...`, `go vet ./...`, and `go test ./...` passed.
- `TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55432/Finova?sslmode=disable go test ./internal/market`
  passed against a temporary PostgreSQL 17 cluster. The DB tests created a
  throwaway schema and applied all available migrations there.
- Applied every `backend/migrations/*.up.sql` migration to an empty PostgreSQL
  17 database with `psql -v ON_ERROR_STOP=1`.
- Imported `backend/testdata/demo`: 30 underlyings, 19,009 daily prices, 1,818
  contracts, 1,818 option quotes, 30 underlying quotes, and 59,642 snapshots.
- Ran `go run ./cmd/metrics recompute` twice. Both runs reported
  `read=1694 calculated=1694 skipped=0 stored=1694`; `option_metrics` had 1,694
  rows after each run.
- Ran `go run ./cmd/metrics recompute -dry-run`: it reported
  `read=1694 calculated=1694 skipped=0 stored=0`; the table still had 1,694
  rows.
- A query of active contracts with quote volume above zero found 559 liquid
  contracts. All 559 had non-null delta and market price; 528 also had
  non-null implied volatility. Of the 31 null-IV cases, 21 option prices were
  at or below intrinsic value and 10 were below the discounted call lower
  bound. The calculation layer correctly leaves IV null for these prices
  instead of fabricating a volatility.
- Started `cmd/api` on port 18080 and requested
  `GET /api/v1/market/options?sort=value&order=desc`: HTTP 200, 50 items, and
  `total=1818`. The first item included value, market price, and delta; its IV
  was correctly null for that quote.

The temporary PostgreSQL cluster and API process were stopped after testing.
The database integration test uses `TEST_DATABASE_URL` and a temporary schema,
and skips when the variable is not set.

## Integration notes

- Apply the backend migrations, import `backend/testdata/demo`, and run
  `go run ./cmd/metrics recompute` before starting the API if the imported
  dataset has no metrics.
- `RISK_FREE_RATE` configures the same rate used by the collector and defaults
  to `0.30`.
- Only active contracts with an available quote and usable spot/strike are
  calculated. Older quote rows are not recomputed; use `-at` only to explicitly
  change the timestamp for the latest quote rows.
- Re-running without `-at` upserts the same metric rows. The command performs
  no network requests and does not apply migrations itself.

## Known limits

- The command recalculates only the latest option quote for each active
  contract, not historical quote snapshots.
- Some volume-positive contracts still have null IV when their observed price
  violates the model's no-arbitrage bounds. Producing IV for those would
  require changing the calculation semantics or quote data.
- Underlyings with fewer than 21 completed daily closes have no HV20, so their
  theoretical price and HV-based Greek fallback remain unavailable.
- The database integration tests and live/demo workflow require a reachable
  PostgreSQL instance; results are reported with the completed checks.
