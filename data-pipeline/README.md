# data-pipeline

Collects **multi-year daily data for TSE/IFB option underlyings** (the stocks &
ETFs that currently have listed options), runs quality checks, and writes tidy
CSVs for EDA / signal research.

> Task (fa): داده روزانه چندساله سهام پایه دارای آپشن (قیمت تعدیل‌شده، حجم،
> جریان حقیقی/حقوقی) جمع و بررسی کیفیت می‌شود: شکاف‌ها، افزایش سرمایه، روزهای بسته.

## What it does
1. **Discovers** every current option underlying from TSETMC's live option
   market watch (no hardcoded list; ~27 symbols as of this run).
2. For each, fetches **3 years** of daily data from the TSETMC JSON API:
   - adjusted + raw OHLC, final (پایانی) & last (آخرین) prices, volume, value, trade count
   - retail/institutional money flow (حقیقی / حقوقی): buy/sell volume, value, counts
3. **Quality checks**: closed/halted days (روز بسته), capital-increase /
   adjustment events (افزایش سرمایه), and gaps (شکاف).

## Run
```bash
pip install -r requirements.txt
python fetch_underlyings.py
```
Needs network access to `cdn.tsetmc.com`. Re-running refreshes up to the latest
trading day. Window length is `YEARS_BACK` at the top of `fetch_underlyings.py`.

## Outputs (`output/`)
| file | description |
|---|---|
| `underlyings_daily.csv` | **main deliverable** — long/tidy daily panel, one row per `(symbol, date)` |
| `underlyings.csv` | master list: symbol, ins_code, date coverage, row count |
| `quality_report.md` | per-symbol summary of halted days, adjustment events, gaps |

See [`data_dictionary.md`](data_dictionary.md) for every column.

## For whoever does the EDA
- Prices are in **Rials**. Use the `adj_*` columns for returns / indicators;
  raw columns preserve what actually printed each day.
- Join to other tables on `ins_code` (stable) rather than `symbol` (can change).
- Load: `pandas.read_csv("output/underlyings_daily.csv")` (UTF-8-BOM, Excel-safe).
- `date` is Gregorian ISO; `date_jalali` is provided for convenience.
