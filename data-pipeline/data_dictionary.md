# Data Dictionary — `output/underlyings_daily.csv`

Long / tidy daily panel of TSE/IFB **option underlyings** (stocks & ETFs that
currently have listed options). **One row per `(symbol, date)`.** Encoded as
UTF-8 with BOM (opens correctly in Excel). Prices are in **Rials**.

Source: TSETMC public JSON API (`cdn.tsetmc.com`). Window: last 3 years.

## Identity & date
| column | meaning |
|---|---|
| `symbol` | Persian ticker of the underlying (نماد پایه), e.g. `اهرم`, `خودرو` |
| `name` | same as `symbol` (kept for schema compatibility) |
| `ins_code` | TSETMC instrument id (`insCode`) — stable join key |
| `date` | trading date, Gregorian ISO `YYYY-MM-DD` |
| `date_jalali` | same date in Jalali (Solar Hijri) `YYYY-MM-DD` |

## Raw prices & activity (as reported by TSETMC)
| column | meaning |
|---|---|
| `open` | first trade price (priceFirst) |
| `high` | day high (priceMax) |
| `low` | day low (priceMin) |
| `last_price` | last trade price — آخرین معامله (pDrCotVal) |
| `final_price` | **closing/volume-weighted price — قیمت پایانی (pClosing). Canonical "close".** |
| `prev_final` | previous session reference price (priceYesterday); drives adjustment |
| `volume` | shares traded (qTotTran5J) |
| `value` | turnover in Rials (qTotCap) |
| `trade_count` | number of trades (zTotTran) |

## Adjusted prices (قیمت تعدیل‌شده)
Back-adjusted for capital increases / dividends by the **ratio method**: when
`prev_final` on a day differs from the previous day's `final_price`, every
earlier day is scaled by the cumulative ratio so the series has no artificial
jump. The most recent day always equals the raw price (`adj_factor`→1.0).
Use the `adj_*` columns for returns, backtests and technical indicators.

| column | meaning |
|---|---|
| `adj_factor` | cumulative back-adjustment multiplier applied to that day |
| `adj_open` `adj_high` `adj_low` `adj_last` `adj_final` | adjusted versions of the prices above |
| `adj_event` | `True` on a day with a capital-increase / dividend / adjustment |
| `adj_ratio` | ratio `prev_final / previous final_price` on an event day (else 1.0) |
| `event_type` | heuristic: `capital_increase` if ratio < 0.90, else `dividend_or_adjustment`. **Heuristic only** — confirm against codal.ir for exact corporate-action type. |

## Money flow — real vs legal (جریان حقیقی / حقوقی)
`ind_*` = individual / retail (حقیقی, `_I`). `corp_*` = legal / institutional
(حقوقی, `_N`). Empty on closed days (no trades → no client-type record).

| column | meaning |
|---|---|
| `ind_buy_vol` / `ind_sell_vol` | shares bought / sold by individuals |
| `ind_buy_val` / `ind_sell_val` | Rial value bought / sold by individuals |
| `ind_buy_count` / `ind_sell_count` | number of individual buyers / sellers |
| `corp_buy_vol` / `corp_sell_vol` | shares bought / sold by legal entities |
| `corp_buy_val` / `corp_sell_val` | Rial value bought / sold by legal entities |
| `corp_buy_count` / `corp_sell_count` | number of legal buyers / sellers |
| `ind_net_value` | `ind_buy_val - ind_sell_val` — **net retail money inflow (ورود/خروج پول حقیقی)**; positive = money entering from individuals |
| `corp_net_value` | `corp_buy_val - corp_sell_val` — net legal-entity money inflow |

## Quality flags (بررسی کیفیت)
| column | meaning |
|---|---|
| `is_halted` | `True` when a record exists but `trade_count==0` / `volume==0` — a closed/suspended day (روز بسته); price carried forward |
| `gap_days_before` | number of market trading days (days when *other* symbols traded) with no record for this symbol immediately before this row — captures suspensions / gaps (شکاف). `0` at each symbol's first row, so pre-listing periods are not counted |

See `output/quality_report.md` for per-symbol summaries of halted days,
adjustment events, and gaps.

## Notes / caveats
- Days the whole market is closed (weekends, public holidays) simply have **no
  row** — absence of a date is not a gap.
- `event_type` classification is a ratio heuristic; large dividends can look
  like small capital increases. Cross-check corporate actions on codal.ir if the
  exact type matters.
- Newly listed ETFs (e.g. `تابان`, `طعام`, `هم تراز`) have fewer rows because
  they listed partway through the window — expected, not missing data.
