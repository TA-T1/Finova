# Task: Daily end-of-day snapshot of option contracts (Go)

## Step 0: Find the project information yourself
Nothing has been filled in for you. Explore the repo before writing any code, and find:
- **The live data adapter** (task 1): look for `internal/source` or any package defining a `Source` interface and `FetchMarket`. Read its structs and its `FakeSource`.
- **The migrations folder** and the `quote_snapshots` table definition. Search the migrations for `snapshot`.
- **How the app connects to the database**: the config struct, env vars (check `.env.example` and config files for something like `DATABASE_URL`), and the DB library in `go.mod` (pgx, database/sql, sqlx, gorm…).
- **The Go module path, folder layout, config and logging setup**: read `go.mod`, `cmd/`, `internal/`.

Then write a short summary of what you found, with file paths, before you start coding.

**If something is missing:**
- No adapter in the repo yet → stop and say so. This task builds on it.
- No `quote_snapshots` table → create it in a new migration with the shape below.
- No DB config convention → read the connection string from `DATABASE_URL`.

## Context
We are building a student project: an analytics site for Iranian stock options, with a Go backend and PostgreSQL. The `quotes_latest` table only keeps the current state of each contract, so yesterday's prices are lost. This task stores one row per contract per trading day in `quote_snapshots`.

**Why it matters:** the data science team's backtest needs to know what each option cost on past days. Free sources may not provide option price history, so this table may become our only real history. Every day it runs adds data, so it should run reliably from now on.

**This task depends only on the live data adapter** (`internal/source`, interface `Source` with `FetchMarket(ctx)`). It calls the adapter directly. It does **not** read from `quotes_latest`, which belongs to another teammate's service.

## Before writing code
- Follow the repo's existing layout, config, logging and DB access patterns. Use the DB library the repo already uses.
- If the existing migration lacks a column listed below or the unique constraint, add a **new** migration. Do not edit an existing one. Add a note in the PR that the backend team should review it.

## What to build

### Table shape
One row per trading day per contract:

| Column | Notes |
| --- | --- |
| `trade_date` | `date`, the Tehran calendar date of the trading day |
| `contract_symbol` | |
| `underlying_symbol` | |
| `option_type` | `call` / `put` |
| `strike` | |
| `expiry` | `date` |
| `close_price`, `last_price` | |
| `bid_price`, `ask_price` | as of the snapshot time |
| `volume`, `value`, `trade_count` | |
| `open_interest` | |
| `underlying_price` | the underlying stock's last price at snapshot time; the backtest needs both prices from the same moment |
| `captured_at` | `timestamptz` |

Constraint: unique on `(trade_date, contract_symbol)`.

### Snapshot logic
A function such as `snapshot.Run(ctx, src source.Source, db) (int, error)` that:
1. Calls `src.FetchMarket(ctx)` once.
2. Works out today's trading date in `Asia/Tehran`.
3. **Skips non-trading days.** If no contract and no underlying has a last trade time on today's Tehran date, it logs "no trading today" and writes nothing. This covers Thursdays, Fridays and holidays without needing a holiday calendar.
4. Joins each contract with its underlying's price by underlying symbol.
5. Writes all rows in **one transaction**, using batch or `COPY` for speed (about 2,000 rows). It upserts with `ON CONFLICT (trade_date, contract_symbol) DO UPDATE`, so running twice on the same day overwrites instead of duplicating.
6. Returns the number of rows written, and logs the count and the duration.

### How it runs
A command `cmd/snapshot` with two modes:
- `snapshot run`: take one snapshot now and exit. Use it for testing and manual runs.
- `snapshot daemon`: stay running and take a snapshot every day at a configurable time. The default is `13:00 Asia/Tehran`, after the market closes at about 12:30.
  - Use a simple in-process scheduler: compute the next run time, sleep until then, and repeat.
  - If a run fails, retry a few times with a delay (for example 3 tries, 5 minutes apart), then log the error and wait for the next day.
  - Shut down cleanly on SIGINT/SIGTERM.

## Tests
- Use the adapter's `FakeSource` with saved sample data. No network.
- **Trading-day detection:** sample data where trades happened today gets written; sample data with no trades today writes nothing.
- **Idempotency:** running twice for the same day leaves one row per contract, with the second run's values.
- **Underlying join:** each row has the correct `underlying_price`.
- **Database tests:** run them against a real local PostgreSQL when `TEST_DATABASE_URL` is set, and skip them otherwise.
- **Scheduler:** the next-run-time calculation handles before 13:00, after 13:00 and the day boundary in the Tehran time zone.

## Done when
- `snapshot run` on a trading day writes one row per active contract, and a second run creates no duplicates.
- `snapshot run` on a non-trading day writes nothing and says so in the log.
- `snapshot daemon` fires at the configured Tehran time.
- `go test ./...` passes.

## Out of scope
- Backfilling past days. That is a Sprint 3 task and depends on whether the source has history.
- Any calculations: IV, Greeks, Black-Scholes.
- Intraday history. One snapshot per day is enough.
