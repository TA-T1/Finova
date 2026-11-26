# Frontend and the API: what the pages use, and what is missing

The pages follow `docs/api-contract.md`; backtests follow `docs/strategy-backtest.md` (a proposal from the backtest engine).

**Real API (checked 2026-11-26 against `main` and the demo dataset):** the market watch, option chain and symbol pages work with `VITE_USE_MOCK=false`.
**Still on mock data:** the strategy builder and the backtest page, because the backend has no `/strategies/*` or `/backtests` endpoints. They stay on mock
data (market lists included) until `VITE_STRATEGY_API=true` / `VITE_BACKTEST_API=true`; the real branch of those two has never run.

### Running against the real backend
```bash
docker compose up -d db
for f in backend/migrations/*.up.sql; do docker compose exec -T db psql -U Finova -d Finova -v ON_ERROR_STOP=1 -q < "$f"; done
cd backend && cp -n .env.example .env && go run ./cmd/backfill import testdata/demo && go run ./cmd/api
cd frontend && cp -n .env.example .env.local && npm run dev       # .env.example sets VITE_USE_MOCK=false
```
Without an env file the app still starts on mock data. The demo dataset has no calculated metrics yet, so IV, Greeks, theoretical price,
break-even, leverage and moneyness show a dash; it also has no previous close, so the "% change" column shows a dash.

| Page | Route | Endpoints |
|---|---|---|
| Market watch | `/market-watch` | `GET /market/options`, `/market/underlyings`, `/market/expiries`, `/market/status` |
| Option chain | `/chain?symbol=&expiry=` | `GET /market/chain`, `/market/underlyings/:symbol`, `/market/underlyings` |
| Symbol page | `/symbol/:symbol` | `GET /market/underlyings/:symbol` |
| Strategy builder | `/strategy?symbol=&expiry=` | `GET /strategies/templates`, `POST /strategies/build`, `POST /strategies/analyze` |
| Backtest results | `/backtests?underlying=&template=` | `GET /backtests`, `GET /backtests/:id` |

## Resolved gaps
The backend now sends `prevClose` and `value` (watch rows and chain sides) and `contractId` (chain sides), and sorts by `bestBid`, `value`,
`moneyness` and `breakEven`. The pages use all of them.

## Open points
1. **Underlying leg id.** An underlying leg needs `underlyingId`; the symbol page has none, so the builder takes it from `/market/underlyings`.
2. **Strategy fees.** Contract 2 ignores fees and taxes, while the sprint plan (task 12) includes them. The page says "without fees". The "today" curve is not drawn.
3. **Backtests.** `docs/api-contract.md` section 3 says there is no strategy backtest. The page uses the shape in `docs/strategy-backtest.md`.
4. **Chain with no contracts** is a `404`, so the chain page shows its error state, not the empty state.

## Backend issues to report (not worked around unless said)
1. `GET /market/underlyings` sends `"items": null` instead of `[]` when the list is empty (for example `?kind=` with an unknown value, which is also accepted instead of a `400`). The API layer turns it into `[]`.
2. `GET /market/options` includes **expired contracts** (124 of 1,818 on 2026-11-26), while `/market/expiries`, `/market/chain` and the symbol page list only active ones. The watch shows them with negative days left, and the expiry filter cannot select them.
3. `daysToExpiry` on `/market/options` is the value stored with the metrics, not today's: after a day without collection it is stale (expired contracts keep "1 day left"). `/market/chain` computes it live.
4. `priceGapPercent` is unbounded when the theoretical price is near 0 (up to about 2.8e275 seen). Sorting by it puts these first and the cell is hundreds of characters wide. It should be `null` below a minimum theoretical price.
5. **Symbols are spelled two ways.** The collector and the history backfill store `underlyings` and `option_contracts` with Arabic `ي`/`ك` as TSETMC sends
   them (7 of 30 underlyings, 214 contracts), while the daily snapshot normalizes to Persian letters. So `quote_snapshots` holds both spellings: the 7 underlyings
   appear twice (their 2026-11-24 live rows do not join to `underlyings` or to their own history), which also splits data for the backtest.
   `/market/underlyings/کاریس` (Persian letters) is a `404`. The app's search box accepts both. One spelling should be chosen and applied in the collector,
   the backfill and the snapshot, and stored rows migrated.
6. `quoteTs`, `lastUpdate` and `metricsTs` carry the server's local offset (`+01:00` here), not `+03:30` as in the contract's examples. They are valid RFC 3339 and displayed in Tehran time.
7. `limit` is still accepted as a deprecated alias of `pageSize` (the contract says it is removed). Harmless.
