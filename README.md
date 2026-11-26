# Finova

Finova is an algorithmic-trading application for Iranian stock options. It tracks
and analyzes market data, builds and evaluates option strategies, and provides
entry/exit signals in a web interface.

## Repository layout

| Directory | Purpose |
|---|---|
| `backend/` | Go 1.27 API, PostgreSQL repositories, market crawler, metrics, snapshot and demo commands |
| `frontend/` | React/Vite web application |
| `backtest/` | Pure-Python strategy backtest engine |
| `data-pipeline/` | Python market and signal data pipeline |
| `EDA/` | Exploratory data analysis |
| `docs/` | API contracts and project write-ups |

## Requirements

- Go **1.27.1**
- Node.js **20.19+ or 22.12+** and npm (Vite 7's runtime requirement)
- Python **3.11 or newer**
- PostgreSQL **16** or a compatible reachable instance
- `psql`
- `make`
- Docker is optional; `docker-compose.yml` starts only PostgreSQL 16. The API,
  frontend and Python tools run directly on the host.

## Setup

From the repository root:

```bash
cp backend/.env.example backend/.env
cd backend
make db          # optional; starts Docker PostgreSQL
make migrate
make demo-load
make run
```

`backend/Makefile` loads `backend/.env` when it exists and exports it to child
commands. If `DATABASE_URL` is set in your shell, that value takes precedence
over the Makefile default. The Docker database uses
`postgres://Finova:Finova@localhost:5432/Finova?sslmode=disable`.

### Environment variables

| Key | Default | Purpose |
|---|---|---|
| `APP_PORT` | `8080` | HTTP port for `go run ./cmd/api` |
| `DATABASE_URL` | `postgres://Finova:Finova@localhost:5432/Finova?sslmode=disable` | PostgreSQL connection string |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000,http://localhost:5173` | Comma-separated browser origins allowed by the API |
| `COLLECT_INTERVAL` | empty/disabled | Background option-chain collection interval, for example `60s` or `2m` |
| `RISK_FREE_RATE` | `0.30` | Annual risk-free rate as a decimal fraction for metrics |
| `TSETMC_BASE_URL` | `https://cdn.tsetmc.com/api` | TSETMC API base URL |
| `TSETMC_USER_AGENT` | built-in desktop user agent | HTTP User-Agent for TSETMC requests |
| `TSETMC_TIMEOUT` | `10s` | Per-request timeout |
| `TSETMC_MAX_ATTEMPTS` | `3` | Total TSETMC attempts, including the first |
| `TSETMC_RATE_LIMIT` | `2` | Requests per second; `0` disables the limit |
| `SNAPSHOT_TIME` | `13:00` | Daily snapshot time in Asia/Tehran |
| `SNAPSHOT_TRIES` | `3` | Attempts per scheduled snapshot |
| `SNAPSHOT_RETRY_DELAY` | `5m` | Delay between snapshot attempts |

Copying the example is enough for Docker. Replace `DATABASE_URL` when using a
local PostgreSQL server. `backend/.env` is still tracked in this repository even
though `.gitignore` also matches it.

## Offline demo

Start the Docker database only if needed:

```bash
cd backend
make db
make migrate demo-load run
```

The targets require an empty database for the full migration loop. `make migrate`
applies `backend/migrations/*.up.sql` in name order with
`psql -v ON_ERROR_STOP=1`. For a database that already has earlier migrations,
apply a single missing migration with:

```bash
cd backend
make migrate-one FILE=migrations/000012_backtest_run_details.up.sql
```

`make demo-load` imports `testdata/demo` and recomputes offline option metrics.
The API is then available at `http://localhost:8080` (or `APP_PORT`).

To load a strategy backtest run, follow the backtest engine's run/output section:
[backtest/README.md](backtest/README.md). It writes JSON with `--out`; apply the
Python writer's database step after migration `000012`.

## Live mode

Set `COLLECT_INTERVAL`, for example `30s`, in `backend/.env` and run `make run`.
The option-chain collector only executes during Tehran market hours:
Saturday through Wednesday, approximately 09:00 to 12:30 Asia/Tehran.

Manual option-chain collection returns HTTP `409` outside market hours. Daily
snapshot collection can run independently:

```bash
cd backend
make snapshot
```

The snapshot target runs `go run ./cmd/snapshot daemon`.

## Frontend

```bash
cd frontend
npm ci
npm run dev
```

To use the API instead of mocks, set the frontend environment:

```dotenv
VITE_USE_MOCK=false
VITE_API_BASE_URL=http://localhost:8080
```

## Backtest engine

The strategy engine is documented in [backtest/README.md](backtest/README.md).

## API

Endpoint shapes and conventions are in [docs/api-contract.md](docs/api-contract.md).
Operational notes and command examples are in [docs/api.md](docs/api.md).