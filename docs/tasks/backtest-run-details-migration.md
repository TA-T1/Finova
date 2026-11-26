# Backtest run details migration

## What was asked

Add storage for the strategy backtest engine's `metrics`, `trades`, and
`skipped` JSON payloads in `backtest_runs`, along with an index for filtering by
model and underlying. Keep the migration re-runnable, verify its rollback, and
document the dependency for the Python writer and read API.

## What was built

- Added `000012_backtest_run_details.up.sql` with nullable JSONB columns
  `metrics`, `trades`, and `skipped`.
- Added `idx_backtest_runs_model_underlying` on
  `(model_name, (params->>'underlying'))`.
- Added a matching down migration that drops the index before the columns.
- Updated the strategy backtest storage note and the migration guidance in
  `docs/api.md`.

The existing `backtest_runs` definition in migration 000003 has only
`model_name` as `NOT NULL`; it has no foreign keys. No existing constraint
needed relaxing. Migration number 000010 remains unused.

## Decisions

- Store the engine payloads as JSONB rather than introducing a trade table;
  this matches `to_backtest_run()` and the existing run-detail API shape.
- Leave all three columns nullable so existing signal-model runs remain valid.
- Index the model and `params->>'underlying'` because those are the fields used
  to distinguish strategy runs in list filtering.

## How it was verified

Used the installed PostgreSQL 17 tools to create an isolated, temporary
database cluster. With `psql -v ON_ERROR_STOP=1`, applied every available
`backend/migrations/*.up.sql` migration from an empty database, applied 000012
again, ran its down migration, then applied 000012 again.

- The second up completed with only “already exists, skipping” notices.
- After down, checks of `information_schema.columns` and `pg_indexes` found
  none of the three columns or the new index.
- After the final up, those checks found all three columns and the index.

The temporary PostgreSQL cluster was stopped and removed. This task changes
SQL and documentation only; no application build or Go test was needed.

## Python and read API notes

- The Python backtest writer must apply migration 000012 before inserting
  strategy runs. It can write the engine's `metrics`, per-expiry `trades`, and
  `skipped` values directly to the respective JSONB columns.
- The list API can use the new expression index when filtering by
  `model_name` and the underlying in `params`.
- The detail API should return `metrics`, `trades`, and `skipped` using the
  frontend's camelCase JSON shape. The migration stores JSON as supplied; it
  does not transform or validate payload keys.

## Known limits

- JSONB payload structure is intentionally not enforced by database
  constraints; producer and API code must keep the documented shape.
- The migration adds storage and an index only; it does not implement the
  Python writer or backtest read endpoints.
