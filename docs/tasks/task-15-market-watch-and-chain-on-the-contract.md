# Task 15: Connect market watch and chain to the real API

Owner role: Front. PR: #10 (`feat/front-mvp`, base `main`; it also contains the 2 commits of `feat/front-sprint2`).
Depends on: task 1 (merge of front-sprint2, done inside this branch), task 5 (market watch API; not merged), softly task 3 (contract).
The contract used is `docs/api-contract.md` (v1 draft, on branch `feat/cal_layer_livadata`, not yet in `main`).

## What the task was
Switch the market watch and chain pages from mock data to the real API, following the agreed contract.

## Status
**Checked against the real server on 2026-11-26** (`main` plus the demo dataset, `VITE_USE_MOCK=false`): market watch, chain and symbol page work.
The `[requested]` fields (`prevClose`, `value`, `contractId`) and sort keys now exist and are used. Strategy and backtest pages stay on mock data.
Current gaps and backend issues are in `docs/frontend-api-notes.md`; the "Gaps in the contract" section below is historical.

## Changes (all under `frontend/`)
| File | Change |
|---|---|
| `src/types/marketTypes.ts` | Rewritten to the contract: `OptionQuote` (quote fields + all calculated fields), `PagedResponse` with `totalPages`, `ChainResponse` with nested `strikes[{strikePrice, call, put}]`, `SymbolDetail`, `ExpiriesResponse`, `MarketStatus`, `MarketWatchParams` (`sort`, `order`, `page`, `pageSize`), the list of sortable keys. Fields the pages need that the contract lacks are marked `[requested]`. |
| `src/services/marketService.ts` | Typed functions with the mock/real switch: `getStatus`, `getMarketWatch`, `getChain`, `getSymbol`, `getUnderlyings`, `getExpiries` (plus the strategy and backtest functions of tasks 17 and 18). Paths: `/api/v1/market/...`. |
| `src/hooks/useMarketQueries.ts` | Hooks for all of the above; market data refreshes every 30 s and pauses while the tab is hidden. |
| `src/mocks/mockMarket.ts`, `mockApi.ts`, `blackScholes.ts` | Mock market now emits contract-shaped rows (underlyingPrice, daysToExpiry, moneyness, marketPrice, intrinsic/time value, break-even and percent, leverage, effective leverage, IV/HV, all Greeks, theoretical price and gap, `metricsTs`), a nested chain, a symbol-page payload (open interest totals, PCR, per-strike open interest, max pain), expiries per underlying, market status. Added `bsGreeks`. Mock sort/paginate/filter follows the contract (default order, nulls last, `400` for an unknown `sort`, default page size 50, max 500). |
| `src/pages/MarketWatch/MarketWatchPage.tsx`, `MarketWatchTable.tsx` | Use `sort`/`order`; expiry filter from `/market/expiries` (filtered by the chosen underlying, with `daysToExpiry`); open/closed badge and last-update time from `/market/status`. Table uses `daysToExpiry` from the API. Columns the API cannot sort (`bestBid`, `value`, `moneyness`) are no longer clickable; the break-even column shows the price and `breakEvenPercent` and sorts by the percentage. |
| `src/pages/Chain/ChainPage.tsx`, `ChainTable.tsx`, `chainUtils.ts` | Use the nested chain response; price and change from the symbol endpoint; expiry list from the symbol endpoint's `expiries`. A chain with no contracts is a 404 in the contract, so the page shows its error state. |
| `frontend/.env.example` | Comment updated. |
| `docs/frontend-api-notes.md` | Page-to-endpoint table and the list of contract gaps (below). |

## Gaps in the contract (the pages show a dash or work around them)
1. **`prevClose` and `value`** are not in `/market/options` or the chain sides. The "last price with % change" and "trade value" columns show a dash until the backend sends them. `option_quotes.value` is already stored; previous close exists in the adapter (`OptionLeg.Yesterday`) but is not persisted.
2. **Sorting:** no `bestBid`, `value`, `moneyness`; `breakEvenPercent` instead of `breakEven`.
3. **`contractId` on chain sides** is missing (needed by task 17).
4. A chain with no contracts is a `404`, not an empty list.

## How to run
```bash
cd frontend && npm ci && npm run dev          # mock data (default)
VITE_USE_MOCK=false VITE_API_BASE_URL=http://localhost:8080 npm run dev   # real backend
```
Mock test switches in the URL: `?mockError=1`, `?mockEmpty=1`.

## Verified
`tsc -b` and `npm run build` pass; the pages were rendered in headless Chrome on mock data. `npm run lint` fails only on the pre-existing `src/types/apiTypes.ts` (`any`).

## For people who depend on this
- Backend (task 5): the fields and parameters the page sends and expects are exactly those of the contract; please also add `prevClose` and `value`, and `contractId` on chain sides, or tell Front what to use instead.
- Integration (task 14): first thing to test is `/market/options` sorting and paging, then `/market/chain`.
