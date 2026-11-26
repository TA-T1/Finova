# Task: Market watch and option chain pages, on mock data (frontend)

## Important: there is no backend yet
The backend endpoints for these pages do **not** exist yet and nothing is running on a server. Do not try to call, start or wait for a backend, and do not write any backend code. Build both pages entirely on **mock data shaped like the API contract document**. A later task will connect them to the real backend. Your job is to make that switch a config change with no component changes.

## Context
We are building a student project: an analytics site for Iranian stock options, similar to optionbaaz.ir. The UI is in **Persian and right-to-left**. A frontend skeleton (layout, menu, empty pages, API client layer) was built in Sprint 1.

## Step 0: Find the project information yourself
Nothing has been filled in for you. Explore the repo before writing any code, and find:
- **The frontend app**: look for a folder with a `package.json` (for example `web/`, `frontend/`, `client/`, `ui/`). Read `package.json`, the router setup, the existing pages and the API client layer.
- **The API contract document**: look for `api.md`, `openapi.yaml`, `swagger.*`, or a docs section listing endpoints with JSON examples. Find the market watch, chain and underlying/symbol endpoints in it.
- **The planned backend base URL**: check the frontend's `.env*` files and API client, and the backend's config files or `.env.example` if they exist. If nothing is found, use `http://localhost:8080` as the default.

Then write a short summary of what you found, with file paths, before you start coding.

**If something is missing:**
- No frontend app yet → create one with React + Vite + TypeScript in a `web/` folder, RTL and Persian.
- No API contract, or it doesn't cover these endpoints → use the fields listed in this brief, and list every assumption in your summary so the backend team can match it.

## Before writing code
- Follow the existing frontend's stack, routing, folder layout, API client, styling and components. Do not add a second UI or state library.
- **The mock data and TypeScript types must match the API contract field for field.** If the contract is unclear or missing a field these pages need, list the gaps in your summary instead of inventing fields silently.

## API layer and mocks
- Add typed API functions, for example:
  - `getMarketWatch(params)`
  - `getChain(symbol, expiry)`
  - `getUnderlying(symbol)` — used for the list of expiries and the stock price
- Add an env flag, for example `VITE_USE_MOCK`, **defaulting to `true`**. When it is on, the functions return mock data with a short artificial delay (200–400 ms). When it is off, they call the real base URL with the paths and params from the contract.
- Keep all mock code in its own folder (for example `src/mocks/`), so it can be left in place for demos and offline work.
- **Mock data:**
  - About 3–4 underlyings, 2–3 expiries each, and 6–10 strikes per expiry, with both a call and a put at each strike. That is roughly 100–150 contracts.
  - Use realistic Iranian-style values: Persian symbols such as `ضخود1234` / `طخود1234`, prices in rial, contract size 1000, and sensible IV (20–80%) and delta values.
  - In mock mode, implement sorting, filtering and pagination inside the mock layer, so it behaves like the real server.
  - On each refresh, slightly randomize prices and volumes, so the auto-refresh is visibly working.

## Page 1: Market watch (`/market-watch` or the existing route)
A table of all option contracts.

**Columns**, with the header text in Persian:
1. contract symbol
2. underlying symbol
3. type (call / put, color-coded)
4. strike
5. expiry and days left
6. last price and % change
7. bid / ask
8. volume
9. trade value
10. open interest
11. IV
12. delta
13. Black-Scholes price and the % difference from the market price
14. breakeven
15. leverage
16. ITM / OTM status

**Behavior:**
- **Sorting:** click a column header to sort, and click again to reverse. Pass the sort as API params, as defined in the contract.
- **Simple filters** above the table: underlying (searchable select), type (all / call / put) and expiry. The advanced filter panel is a later sprint.
- **Pagination,** as defined in the contract.
- **Auto refresh** every 30 seconds:
  - show "last updated" with the time;
  - pause refreshing while the browser tab is hidden;
  - add a manual refresh button.
- **Number formatting:**
  - thousand separators;
  - percentages to 1–2 decimals;
  - mark whether prices are in rial or toman;
  - green and red for positive and negative change.
- **States:** a loading skeleton, an error message with a retry button, and an empty state when the filters match nothing. Make it possible to trigger the error state in mock mode, for example with a second flag or a query param, so it can be checked.
- **Links:** clicking a contract's underlying symbol goes to `/symbol/:symbol`. That page can stay a placeholder for now.
- **Layout:** the table scrolls horizontally on narrow screens. Keep the header row and the symbol column sticky.

## Page 2: Option chain (`/chain` or the existing route)
Calls and puts of one underlying and one expiry, side by side.

**Top bar:**
- Select the underlying, then select its expiry. The expiries come from the symbol endpoint. Default to the nearest expiry.
- Show the underlying's current price and % change, and the days left to the selected expiry.
- Put the selected symbol and expiry in the URL query string, so the page can be shared and reloaded.

**Table, mirrored around the strike column:**
- **Call side:** last price, % change, bid, ask, volume, open interest, IV, delta.
- **Middle:** strike price, emphasized.
- **Put side:** the same fields, mirrored.

**Highlighting:**
- A call is in the money when strike < underlying price.
- A put is in the money when strike > underlying price.
- Give in-the-money cells a light tint.
- Mark the strike row closest to the underlying price, or draw a line between the two strikes the price falls between.

**Other behavior:**
- The same 30-second refresh and the same loading, error and empty states as the market watch.
- Clicking the underlying's name goes to `/symbol/:symbol`.

## General
- **RTL:** everything is right-to-left. Check that sort icons, sticky columns and the mirrored chain still look right in RTL.
- **Shared components:** keep them in the existing components folder, so the symbol page and later pages can reuse them. For example: a number cell, a change % badge, a call/put badge and a refresh indicator.
- **Types:** strong TypeScript types for every API response, taken from the contract document, in one place the integration task can update.

## Done when
- With mock mode on (the default):
  - both pages work fully;
  - sorting, filters and pagination behave like a real server;
  - data visibly refreshes every 30s;
  - in-the-money highlighting is correct for both calls and puts;
  - the loading, error and empty states can all be seen.
- The real-API code path exists and follows the contract, but is **not** tested against a server. Say this clearly in your summary.
- No console errors, and the build passes.
- Your summary lists:
  - any gaps or ambiguities found in the API contract;
  - every assumption you made about response shapes.

## Out of scope
- Calling or running any backend.
- Any backend code.
- The advanced filter panel. That is Sprint 3.
- The symbol page's content. A placeholder route is enough.
- Strategy, signals, portfolio and login pages.
