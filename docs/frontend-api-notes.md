# Frontend and the API: what the pages use, and what is missing

The pages follow `docs/api-contract.md` (v1, still a draft for sign-off; it is on branch `feat/cal_layer_livadata`, not in `main` yet) and, for backtests,
`docs/strategy-backtest.md` (a proposal from the backtest engine). Mock data is the default
(`VITE_USE_MOCK=true`); `VITE_USE_MOCK=false` calls the real server at `VITE_API_BASE_URL` with the paths below.
**The real branch has not been run against a server**: the endpoints it needs are not all merged
(calculation layer, strategy and backtest APIs). The mock layer returns exactly the contract's shapes, so
the switch should be a config change; the first real test will tell.

| Page | Route | Endpoints |
|---|---|---|
| Market watch | `/market-watch` | `GET /market/options`, `/market/underlyings`, `/market/expiries`, `/market/status` |
| Option chain | `/chain?symbol=&expiry=` | `GET /market/chain`, `/market/underlyings/:symbol`, `/market/underlyings` |
| Symbol page | `/symbol/:symbol` | `GET /market/underlyings/:symbol` |
| Strategy builder | `/strategy?symbol=&expiry=` | `GET /strategies/templates`, `POST /strategies/build`, `POST /strategies/analyze` |
| Backtest results | `/backtests?underlying=&template=` | `GET /backtests`, `GET /backtests/:id` |

## Gaps in the contract (the pages show "—" or work around them)

1. **`prevClose` and `value` on `/market/options` and the chain sides.** The brief asks for the last price with its % change, and for the
   traded value. Neither is in contract 1.5. The types mark them optional (`[requested]`) and the cells show "—" until the backend sends them.
   `option_quotes.value` is already stored; the previous close is `OptionLeg.Yesterday` in the adapter but is not persisted.
2. **`contractId` on the chain sides.** The strategy builder adds a leg by `contractId` (contract 2.1), and the chain response (1.6) has only
   `symbol` per side. The builder needs `contractId` there (or `POST /strategies/analyze` must accept a contract `symbol`). The mock sends it.
3. **Sorting.** The contract's sort list has no `bestBid`, `value` or `moneyness`, and has `breakEvenPercent` but not `breakEven`. Those
   column headers are not clickable, and the break-even column sorts by its percentage.
4. **Underlying leg id.** An underlying leg needs `underlyingId`. The symbol page has no id, so the builder takes it from `/market/underlyings`.
5. **Strategy fees.** Contract 2 ignores fees and taxes, while the sprint plan (task 12) says the builder includes fees. The page says "without fees".
   The "today" curve on the chart (a cut candidate in the plan) is not drawn: only the expiry curve.
6. **Backtests.** `docs/api-contract.md` section 3 says there is no strategy backtest. The page uses the shape in `docs/strategy-backtest.md`
   (`tradeCount`, `modelPricedShare`, `metrics`, `trades`, `skipped`, `equityCurve`). The mock data is real engine output (`backtest/scripts/export_api_runs.py`).
   Until the daily snapshot has run past an expiry, every backtest is priced with a model; the page shows a notice when `modelPricedShare > 0`.
7. **Chain with no contracts** is a `404` in the contract (1.6), so the chain page shows its error state for it, not the empty state.
