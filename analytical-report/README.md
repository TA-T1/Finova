# Analytical report (task 10): option strategy backtests

The written report is the shared document "Finova Analytical Report: Option Strategy Backtests"
(10 sections: summary, data, method, results by strategy / underlying / year, model price against real prices,
sensitivity, link to the feature table, limitations, recommendations). This folder holds everything behind its numbers.

| Path | What it is |
|---|---|
| `scripts/run_backtests.py` | 9 templates x 24 underlyings x 6 scenarios (entry 30/14/7 days, target/stop exit, options priced at 1.5x and 2.2x HV20), made with `backtest/finova_backtest` |
| `scripts/analyze.py` | summary tables and charts, real-price check against the model, join with `EDA/outputs/features/features.parquet` (HV rank, 20-day return) |
| `scripts/make_tables.py` | the text tables used in the report (`outputs/report_tables.md`) |
| `outputs/*.csv`, `*.json` | every table behind the report (large per-trade files are regenerated, not committed) |
| `figures/*.png` | the six charts of the report |

Run everything from this folder (`cd analytical-report`).

Inputs (all in this repository): `backtest/` (task 8), `backend/testdata/demo/` (task 7),
`data-pipeline/output/underlyings_daily.csv`, `EDA/outputs/features/features.parquet` (features task).

```bash
pip install -r requirements.txt
python scripts/run_backtests.py     # about 1 minute; writes outputs/runs.csv, trades.csv, skipped.csv, equity_curves_base.json
python scripts/analyze.py           # tables and charts
python scripts/make_tables.py > outputs/report_tables.md
```

Notes
- Every trade is priced with Black-Scholes at HV20 (no real option price exists for a past expiry yet), so the results compare strategies; they are not profit figures.
- Arabic and Persian letters are unified before joining option and price data (`scripts/common.py`, function `fa`).
- Task 9 (stored backtest runs) is not used here; these runs come straight from the engine.
