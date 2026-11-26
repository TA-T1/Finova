# Makefile and project run guide

## What was asked

Make the project usable outside the backend team: add practical Make targets,
replace the root README with setup and run instructions, and verify the workflow
against both Docker PostgreSQL and a local PostgreSQL installation.

## What was built

- Added `backend/Makefile` targets:
  - `db`: start PostgreSQL 16 from `docker-compose.yml`
  - `migrate`: apply every migration in name order
  - `migrate-one FILE=...`: apply one migration
  - `demo-load`: import the demo and recompute metrics
  - `run`: start the Go API
  - `test`: run Go tests
  - `snapshot`: run the snapshot daemon
- The Makefile loads `backend/.env` and exports it to commands. `DATABASE_URL`
  has the Docker/PostgreSQL connection string as a Makefile fallback.
- Replaced the root README with requirements, layout, setup, offline demo,
  live mode, frontend, backtest, and API links.
- Documented every key in `backend/.env.example`, including TSETMC and snapshot
  settings.

## Decisions

- Docker remains optional. `db` is the only target that calls Docker; migration,
  demo, API, test, and snapshot targets work with either Docker PostgreSQL or a
  locally installed server.
- `migrate` is intentionally for a new/empty database. If it fails or migrations
  are partially applied, the target tells the user to use `migrate-one` instead
  of silently continuing.
- `demo-load` calls the existing backfill import command and the metrics
  recompute command, so the offline chain and market watch have no network
  dependency.
- Snapshot mode uses `snapshot daemon`; manual collection behavior remains in
  the API and calculation documentation.

## Verification

Verified the README workflow on Windows with GNU Make 3.81 and PostgreSQL 17
(using a throwaway cluster on port 55434):

```bash
cd backend
make migrate
make demo-load
make test
```

The migration loop applied all available migrations in name order. `demo-load`
imported 30 underlyings, 1,818 contracts, 1,818 option quotes, 19,009 daily
prices, and 59,642 snapshots, then recomputed 1,694 option metric rows.
`make test` passed across all Go packages. `make migrate-one` was also checked
against migration 000012 and completed with re-runnable “already exists”
notices.

The Docker branch could not be executed in this environment because Docker
Desktop/WSL2 virtualization is disabled or unavailable. Compose configuration
and the `make db` command shape were checked; they require a host that enables
virtualization and Docker Desktop/WSL2. Docker verification on a capable host
should run:

```bash
cd backend
make db
make migrate demo-load run
```

For a database that has earlier migrations already applied:

```bash
cd backend
make migrate-one FILE=migrations/000012_backtest_run_details.up.sql
```

The local PostgreSQL workflow was completed with `DATABASE_URL` set explicitly
to the throwaway server. A fresh-clone check must copy
`backend/.env.example` to `backend/.env`, start or configure PostgreSQL, apply
migrations, load the demo, and start the API without undocumented setup steps.

The local API check started `make run` with `APP_PORT=18082` and confirmed:

```text
Listening and serving HTTP on :18082
GET /api/v1/health                       -> 200
GET /api/v1/market/options?pageSize=1    -> 200
```

## Frontend and backtest integration

- The frontend starts with `npm ci && npm run dev`.
- The API base is `http://localhost:8080` unless `APP_PORT` is changed.
- Backtest setup and JSON output are linked to `backtest/README.md`.
- API contracts remain in `docs/api-contract.md`; operational details remain in
  `docs/api.md`.

## Known limits

- `make db` requires Docker Compose and creates PostgreSQL-only infrastructure.
  The API, frontend, and Python engine are not containerized.
- `make migrate` may fail clearly against a database that already has schema
  objects. Use `migrate-one` or an existing migration workflow for that case.
- `backend/.env` remains tracked by Git even though `*.env` patterns in
  `.gitignore` cover it. Do not remove or untrack it as part of this task.
