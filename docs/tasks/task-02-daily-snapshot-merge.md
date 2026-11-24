# Task 2: Merge the daily snapshot into main

Owner role: Crawl. PR: #8 (`feat/crawl-snapshot-rebased`, base `main`).
Depends on: nothing hard (soft: task 1). Unblocks: task 7 (history backfill), and every later user of `quote_snapshots`.

## What the task was
Bring `feat/crawl-daily-snapshot` up to date with `main`, resolve the conflicts (mostly the TSETMC adapter), rename its migration `000007_quote_snapshots` to `000008` (main already has a `000007`), and merge.

## What was done

### Merge, not rebase
A rebase was tried first. It conflicted on its very first commit, because `main` already carries older copies of this branch's own files (they came in through the earlier crawl-adaptor PR). Rebasing 10 commits would have meant resolving the same files again and again. So `main` was merged into the branch once instead. The end state is the same as a finished rebase. The old branch `feat/crawl-daily-snapshot` is untouched and can be deleted after the PR merges.

### The 9 conflicts and how each was resolved
| File | Resolution |
|---|---|
| `backend/cmd/fetchdemo/main.go` | branch version (adds trade time and underlying name/volume to the printout) |
| `backend/internal/config/config.go` | branch version (adds `SNAPSHOT_TIME`, `SNAPSHOT_TRIES`, `SNAPSHOT_RETRY_DELAY`) |
| `backend/internal/datasource/market.go` | branch version (comments updated: trade time and name/volume are now filled by enrichment) |
| `backend/internal/datasource/tsetmc/client.go` | branch version (per-attempt timeout follows a custom `http.Client`) |
| `.../tsetmc/client_test.go` | branch version |
| `.../tsetmc/market.go` | branch version (enrichment requests, `LastTradeTime`, underlying name and volume) |
| `.../tsetmc/market_test.go` | branch version |
| `backend/scripts/capture_tsetmc_samples.sh` | branch version |
| `docs/api.md` | branch version, plus the `/market/chain` and `/market/symbol/:symbol` documentation that `main` added and the branch did not have |

For every code file the branch version was a strict superset of the copy on `main` (verified with `git diff main:file HEAD:file`: only additions on the branch side), so taking it loses nothing from `main`.

### Migration rename
`backend/migrations/000007_quote_snapshots.{up,down}.sql` became `000008_quote_snapshots.{up,down}.sql`. The test `internal/snapshot/db_test.go` and `docs/api.md` were updated to the new name. The calculation-layer branch uses `000009`, so there is no clash.

## What the branch contains (as merged)
- `internal/source/`: type aliases (`source.Source` is `datasource.MarketSource`) so new code can use the names from the task brief.
- `internal/datasource/tsetmc/market.go`: `FetchMarket` with normalization, retries, rate limit, and two enrichment requests (full names, underlying volume, last trade time).
- `internal/snapshot/`: `Run` (fetch once, skip non-trading days, join underlying price, one transaction with `COPY` into a staging table then one upsert), `Daemon` (daily scheduler, default 13:00 Asia/Tehran, retries), trading-day detection that needs no holiday calendar.
- `cmd/snapshot`: `snapshot run` and `snapshot daemon`.
- Migration `000008_quote_snapshots`: one row per contract per trading day, primary key `(trade_date, contract_symbol)`.
- Docs: `docs/01-crawl-adapter.md`, `docs/02-crawl-daily-snapshot.md`, the snapshot section of `docs/api.md`.

## How it was verified
- `go build ./...`, `go vet ./...`, `go test ./...` pass.
- The DB tests ran against a real Postgres 16 (`TEST_DATABASE_URL`): all 8 DB tests plus scheduler and trading-day tests pass.
- `snapshot run` was run live against TSETMC after the market closed on 2026-11-24: 1,818 rows written, reason "contract has a last trade time today", about 1 s.

## For people who depend on this
- Table `quote_snapshots` columns: `trade_date`, `contract_symbol`, `underlying_symbol`, `option_type`, `strike`, `expiry`, `close_price` (settlement), `last_price`, `bid_price`, `ask_price`, `volume`, `value`, `trade_count`, `open_interest`, `underlying_price` (NULL when unknown), `captured_at`. Money columns are BIGINT rials.
- Run migrations in order; `000008` must come after `000007_underlying_prev_close`.
- Env vars: `SNAPSHOT_TIME` (default `13:00`), `SNAPSHOT_TRIES` (3), `SNAPSHOT_RETRY_DELAY` (5m), plus the existing `TSETMC_*` and `DATABASE_URL`.
- The daemon only schedules the next 13:00. If it is down at 13:00 that day is missed; run `snapshot run` by hand after a restart. Manual runs during the session store partial figures; run them after the close.

## Known limits / open items
- Behaviour on a holiday is still unverified (the trade date comes from `GetClosingPriceInfo.dEven`, only observed on a trading day). The snapshot has a guard that compares with the previous snapshot; `backend/scripts/verify_live.sh` should be run on a Thursday or Friday.
- Each snapshot downloads about 3.7 MB extra (the market watch used for enrichment).
- The migration was added by the Crawl role; the backend team should review it.
- The committed `backend/.env` (task 1) is not touched here.
