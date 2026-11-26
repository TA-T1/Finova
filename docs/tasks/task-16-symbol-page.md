# Task 16: Symbol page

Owner role: Front. PR: #10 (`feat/front-mvp`).
Depends on: task 1. Data: `GET /market/underlyings/:symbol` (contract 1.3; the backend serves it today as `GET /market/symbol/:symbol`, the contract proposes the new path and keeps the old one as an alias).

## What the task was
A page for one underlying: price and change, expiries, open interest by strike chart, put/call ratio, Max Pain.

## What the page shows (`/symbol/:symbol`)
- **Header:** symbol, name, kind, last price, change % and change in rial, refresh indicator, links to the chain and to the strategy builder for this symbol.
- **Cards:** put/call ratio (PCR) with a plain-language reading (more puts or more calls open), call vs put open interest with percentages, Max Pain strike with its expiry and total payout, nearest expiry with days left and contract count.
- **Open interest by strike chart:** grouped bars (calls blue, puts amber) per strike, a dashed line at the strike nearest to the current price, the Max Pain strike highlighted, tooltips with exact values. Totals are over all active expiries; Max Pain is of the nearest expiry (as the backend computes it).
- **Expiries:** a chip per expiry with date (Jalali), days left, contract count; each links to `/chain?symbol=...&expiry=...`.
- **States:** loading skeleton; `404` shows "no active option contracts for this symbol"; other errors show an error state with retry; empty open interest shows an empty state. Data refreshes every 30 s.

## Files
| File | |
|---|---|
| `frontend/src/pages/Symbol/SymbolPage.tsx` | the page (replaces the placeholder) |
| `frontend/src/components/Custom/charts/OpenInterestChart.tsx` | bar chart in plain SVG (no chart library was added) |
| `frontend/src/components/Custom/charts/scale.ts` | tick and scale helpers shared with the line chart |
| `frontend/src/hooks/useMarketQueries.ts` | `useSymbol` |
| `frontend/src/types/marketTypes.ts` | `SymbolDetail`, `StrikeOpenInterest` |
| `frontend/src/mocks/mockMarket.ts` | `symbolDetail`: OI totals, PCR, per-strike OI, max pain computed from the mock contracts |

## Contract notes
- `pcr` is `null` when call open interest is zero; the card shows a dash and says so.
- `price`, `priceChange`, `priceChangePercent` can be `null`.
- `maxPain` can be `null` (no active contracts).
- The page does not need any field the contract lacks.

## Verified
Rendered in headless Chrome on mock data (all four mock underlyings). Not tested against the real endpoint: it must be merged (the path may differ, see above; change the one constant in `marketService.ts` if so).

## Limits
- The chart labels about 10 strikes at most; with many strikes the bars get thin (underlyings can have 30+ strikes).
- Numbers on the axis are short Latin forms (750K); card values use Persian digits.
