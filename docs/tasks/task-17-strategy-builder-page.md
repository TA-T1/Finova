# Task 17: Strategy builder page

Owner role: Front. PR: #10 (`feat/front-mvp`).
Depends on: task 12 (strategy builder API) and task 13 (templates), both Back and **not merged**; softly task 3. The page runs on mock data now.

## What the task was
A form to add and remove legs, a template picker, and the P&L chart with break-even, max profit and max loss.

## What the page does (`/strategy?symbol=&expiry=`)
- Pick an underlying and an expiry (from the symbol endpoint's active expiries). Changing either clears the legs.
- **Template picker:** choose a template and a number of contracts; "ساخت از الگو" calls `POST /strategies/build`, and the returned legs fill the table.
- **Legs table:** each leg shows side (click to flip buy/sell), contract symbol and type, strike, quantity (contracts, or shares for a share leg) and price per share (empty means market price; the placeholder shows the price the server used). Legs can be removed.
- **Add a leg by hand:** type (call, put, underlying), strike from the chain, buy/sell.
- **Live analysis:** whenever the legs change, `POST /strategies/analyze` runs. Shown: net premium (labelled debit or credit), max profit and max loss (the word for "unlimited" when `null`), break-even points, position delta, and the **P&L-at-expiry chart** (green above zero, red below, a line at the current price and at each break-even). Errors from the server (for example `422` mixed expiries) are shown next to the last good result.
- The page states that the calculation ignores fees and taxes (contract section 2).

## Files
| File | |
|---|---|
| `frontend/src/pages/Strategy/StrategyPage.tsx` | the page |
| `frontend/src/components/Custom/charts/LineChart.tsx` | SVG line chart (signed area, step mode, vertical marker lines on two label levels) |
| `frontend/src/types/strategyTypes.ts` | templates, legs, analysis, build request/response, exactly as contract section 2 |
| `frontend/src/services/marketService.ts` | `getStrategyTemplates`, `buildStrategy`, `analyzeStrategy` (mock/real switch; paths `/api/v1/strategies/...`) |
| `frontend/src/hooks/useMarketQueries.ts` | `useStrategyTemplates`, `useStrategyAnalysis` |
| `frontend/src/mocks/mockStrategy.ts` | mock server: the 9 draft templates, strike picking, build, analyze (validation errors `400`/`404`/`422`) |
| `frontend/src/utils/payoff.ts` | the payoff maths the mock uses: P&L at a price, net premium, max profit and loss, break-evens, the 41-point curve plus strikes |
| `frontend/tests/payoff.test.ts` | 6 tests, run with `npm test` (Node's built-in test runner) |
| `frontend/src/routes/routes.tsx`, `layouts/AppLayout/AppLayout.tsx` | route and menu entry |

## Payoff tests (hand calculations)
Bull call spread (buy 2000 @150, sell 2100 @80): net premium 70,000, max profit 30,000, max loss 70,000, break-even 2070. Long call: unlimited profit, loss is the premium, break-even 2150. Long straddle: two break-evens (1810, 2190). Covered call: shares plus a short call (max profit 160,000, break-even 1940). Short put. Curve: 41 points from 50% to 150% of spot plus every strike, ascending.

## Gaps and assumptions
1. **Chain sides have no `contractId` in the contract**, and a leg is added by `contractId`. The page needs it on the chain (or the analyze endpoint must accept a contract symbol). The mock sends it.
2. An underlying leg needs `underlyingId`; the page takes it from `/market/underlyings` by symbol.
3. Task 12's plan says fees are included; contract section 2 says fees and taxes are ignored. The page follows the contract and says so.
4. The "today" curve (a cut candidate in the sprint plan) is **not** drawn; only the expiry curve.
5. The server must return `price` filled on every leg (contract 2.1); the page uses it as the placeholder.
6. Saved strategies (`/strategies` CRUD) need auth and are not in this page.

## For Back (tasks 12, 13)
The builder's request and response shapes are the contract's; the mock in `mockStrategy.ts` and `utils/payoff.ts` is a readable reference of the intended maths (position Greeks as `±quantity x contractSize x greek`, break-evens ascending, `null` for unbounded profit or loss, payoff curve points as in 2.2). The template ids must match the ones in `backtest/finova_backtest/templates.py` (task 8).

## Verified
`tsc -b`, build, `npm test`; template build and manual leg flow driven in headless Chrome on mock data. Not run against the real API.
