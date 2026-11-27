# Task 7 - Feature engineering for underlying stocks (runs after EDA phases 1-4)

Builds the `features` table: one row per symbol and trading day.

Features: returns 1/5/20 days; RSI14; SMA ratios and crossovers (5/20, 20/60); ATR14 %; traded value ratio vs 20-day mean;
retail vs institutional flow; HV20; HV rank (250 and 120 days). Full list: `outputs/features/feature_dictionary.csv`.

Rule: the feature of day t uses only data of day t and earlier.

Inputs: outputs of EDA steps 4 and 6a (`outputs/step4_adjustments/adjusted_series.csv`, `outputs/step6a_returns_features/features_daily.csv`) and `data/raw/underlyings_daily.csv` (copy of `data-pipeline/output/`); shared helpers from `src/common.py`.

```bash
pip install -r requirements.txt   # in the EDA folder
python scripts/05_features.py                          # builds the table + runs the look-ahead checks
python scripts/05_features.py --db sqlite:///features.db   # also loads table `features` into a database
pytest tests/test_features_no_lookahead.py -q           # the no-look-ahead check on its own
```

Output: `outputs/features/features.parquet` (+ `.csv`), `feature_dictionary.csv`, `feature_coverage*.csv`, `checks_summary.csv`, `lookahead_report.csv`, two charts.

Notes
- Halted days keep their row: features NaN, `is_active=0`. Warm-up rows are NaN (HV rank 250 needs ~1 year).
- Volume uses traded value, not share volume (the adjusted share volume of EDA step 4 was estimated with future data).
- Prices are the adjusted series; all features are ratios/returns, so level rescaling does not matter.
