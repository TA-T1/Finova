# EDA and features for the underlying stocks

Three years of daily data for 27 option underlyings: audit, price adjustment, profile, returns, and the `features` table.

```bash
pip install -r requirements.txt
mkdir -p data/raw && cp ../data-pipeline/output/{underlyings_daily.csv,underlyings.csv,quality_report.md} data/raw/
python scripts/01_audit.py                 # steps 0-3: load, structure, price/volume consistency, halts and gaps
python scripts/02_adjustments.py           # step 4: adjustment events and corrected adjusted prices
python scripts/03_profile_and_returns.py   # steps 5-6b: symbol profile, returns, distribution and risk
python scripts/04_return_dynamics.py       # step 6c: autocorrelation, volatility clustering, weekday effects
python scripts/05_features.py              # task 7: features table + no-look-ahead checks (add --db <url> to load a database)
pytest tests -q                            # no-look-ahead tests
```

Outputs go to `outputs/` (git-ignored, except `outputs/features/`, which holds the committed features table:
`features.parquet`, `features.csv`, dictionary, coverage, checks). `notebooks/eda_report.ipynb` shows every table and chart.
The features task is described in `features/README.md`.
