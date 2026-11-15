# Data Quality Report — Option Underlyings (daily)

- Generated: 2026-11-08 00:42
- Window: **2023-11-08 .. 2026-11-08** (3 years)
- Symbols: **27**
- Total daily rows: **17,368**
- Market trading days in window (union of all symbols): **718**
- First / last market day: **2023-11-08 .. 2026-11-07**

## 1. Halted / closed days (روزهای بسته) — total 2,452
Rows where a record exists but trade_count==0 or volume==0.

| symbol | halted days |
|---|---|
| دزاگرس | 299 |
| خودرو | 136 |
| وبصادر | 128 |
| خساپا | 120 |
| اخابر | 111 |
| فملي | 107 |
| وتجارت | 100 |
| شپنا | 98 |
| تاپيكو | 92 |
| فزر | 90 |
| وبملت | 89 |
| فرابورس | 86 |
| خبهمن | 82 |
| ذوب | 82 |
| تاصيكو | 79 |
| بساما | 79 |
| دارونو | 75 |
| جوانه كوچك | 74 |
| طعام | 74 |
| موج | 70 |
| شستا | 67 |
| اطلس | 62 |
| توان | 59 |
| كاريس | 59 |
| اهرم | 59 |
| هم تراز | 55 |
| تابان | 20 |

## 2. Price-adjustment events (افزایش سرمایه / تعدیل) — total 69
Days where the opening reference price differs from the previous closing price (capital increase, rights/bonus issue, or dividend). `event_type` is a ratio-based heuristic.

| symbol | date | ratio | event_type |
|---|---|---|---|
| اخابر | 2024-07-22 | 0.953947 | dividend_or_adjustment |
| اخابر | 2025-04-06 | 0.0625 | capital_increase |
| اخابر | 2025-07-09 | 0.936306 | dividend_or_adjustment |
| اخابر | 2026-07-18 | 0.932179 | dividend_or_adjustment |
| اطلس | 2023-11-21 | 0.1 | capital_increase |
| بساما | 2024-07-23 | 0.974403 | dividend_or_adjustment |
| بساما | 2024-12-11 | 0.416373 | capital_increase |
| بساما | 2026-07-27 | 0.994499 | dividend_or_adjustment |
| تابان | 2026-09-23 | 0.968345 | dividend_or_adjustment |
| تاصيكو | 2024-08-24 | 0.895604 | capital_increase |
| تاصيكو | 2025-09-13 | 0.893411 | capital_increase |
| تاصيكو | 2026-03-30 | 0.725347 | capital_increase |
| تاصيكو | 2026-09-14 | 0.51602 | capital_increase |
| تاصيكو | 2026-09-15 | 0.964938 | dividend_or_adjustment |
| تاصيكو | 2026-09-16 | 1.982589 | dividend_or_adjustment |
| تاپيكو | 2024-09-10 | 0.875234 | capital_increase |
| تاپيكو | 2025-09-20 | 0.880454 | capital_increase |
| تاپيكو | 2026-09-15 | 0.932329 | dividend_or_adjustment |
| خبهمن | 2024-07-21 | 0.960227 | dividend_or_adjustment |
| خبهمن | 2025-07-26 | 0.944418 | dividend_or_adjustment |
| خبهمن | 2026-07-26 | 0.872123 | capital_increase |
| خساپا | 2025-05-06 | 0.120596 | capital_increase |
| خودرو | 2025-04-22 | 0.117632 | capital_increase |
| دزاگرس | 2025-08-11 | 0.929412 | dividend_or_adjustment |
| دزاگرس | 2026-01-05 | 0.221111 | capital_increase |
| دزاگرس | 2026-07-25 | 0.984334 | dividend_or_adjustment |
| ذوب | 2024-03-17 | 0.10602 | capital_increase |
| شستا | 2024-10-21 | 0.862511 | capital_increase |
| شستا | 2025-10-25 | 0.86778 | capital_increase |
| شستا | 2026-10-25 | 0.942935 | dividend_or_adjustment |
| شپنا | 2024-02-26 | 0.611818 | capital_increase |
| شپنا | 2024-07-21 | 0.891071 | capital_increase |
| شپنا | 2025-03-25 | 0.704303 | capital_increase |
| شپنا | 2025-07-22 | 0.905512 | dividend_or_adjustment |
| شپنا | 2026-05-09 | 0.840131 | capital_increase |
| شپنا | 2026-07-22 | 0.925253 | dividend_or_adjustment |
| فرابورس | 2024-06-10 | 0.992378 | dividend_or_adjustment |
| فرابورس | 2024-07-24 | 0.795455 | capital_increase |
| فرابورس | 2025-09-24 | 0.83535 | capital_increase |
| فرابورس | 2026-07-13 | 0.990437 | dividend_or_adjustment |
| فرابورس | 2026-10-31 | 0.797753 | capital_increase |
| فزر | 2024-02-17 | 0.856287 | capital_increase |
| فزر | 2024-09-15 | 0.555749 | capital_increase |
| فزر | 2025-04-16 | 0.91022 | dividend_or_adjustment |
| فزر | 2026-02-21 | 0.919495 | dividend_or_adjustment |
| فملي | 2024-02-21 | 0.769547 | capital_increase |
| فملي | 2024-07-23 | 0.951886 | dividend_or_adjustment |
| فملي | 2025-05-12 | 0.742798 | capital_increase |
| فملي | 2025-07-26 | 0.945185 | dividend_or_adjustment |
| فملي | 2026-07-26 | 0.978017 | dividend_or_adjustment |
| فملي | 2026-08-01 | 0.729271 | capital_increase |
| هم تراز | 2025-09-21 | 1.053 | dividend_or_adjustment |
| وبصادر | 2023-12-25 | 0.904785 | dividend_or_adjustment |
| وبصادر | 2024-06-23 | 0.989851 | dividend_or_adjustment |
| وبصادر | 2025-01-15 | 0.275225 | capital_increase |
| وبصادر | 2025-07-26 | 0.975124 | dividend_or_adjustment |
| وبملت | 2024-01-07 | 0.497085 | capital_increase |
| وبملت | 2024-06-22 | 0.963685 | dividend_or_adjustment |
| وبملت | 2025-02-25 | 0.586614 | capital_increase |
| وبملت | 2025-07-26 | 0.963115 | dividend_or_adjustment |
| وبملت | 2025-08-03 | 0.508936 | capital_increase |
| وبملت | 2026-04-05 | 0.703074 | capital_increase |
| وبملت | 2026-07-25 | 0.958855 | dividend_or_adjustment |
| وتجارت | 2024-03-04 | 0.666831 | capital_increase |
| وتجارت | 2024-06-23 | 0.956919 | dividend_or_adjustment |
| وتجارت | 2025-03-16 | 0.226725 | capital_increase |
| وتجارت | 2025-07-26 | 0.976695 | dividend_or_adjustment |
| وتجارت | 2026-03-30 | 0.930521 | dividend_or_adjustment |
| وتجارت | 2026-07-26 | 0.982857 | dividend_or_adjustment |

## 3. Gaps (شکاف‌ها) — total 9 gap occurrences
A gap = market trading day(s) on which the symbol had no record while other symbols traded (suspensions, late listing, long halts).

| symbol | gap occurrences | total missing days | largest gap |
|---|---|---|---|
| دزاگرس | 2 | 2 | 1 |
| اطلس | 1 | 1 | 1 |
| بساما | 1 | 1 | 1 |
| توان | 1 | 1 | 1 |
| فرابورس | 1 | 1 | 1 |
| فزر | 1 | 1 | 1 |
| كاريس | 1 | 1 | 1 |
| موج | 1 | 1 | 1 |

## 4. Coverage per symbol
| symbol | ins_code | first_date | last_date | rows |
|---|---|---|---|---|
| اخابر | 22811176775480091 | 2023-11-08 | 2026-11-07 | 718 |
| اطلس | 11427939669935844 | 2023-11-08 | 2026-11-07 | 717 |
| اهرم | 17914401175772326 | 2023-11-08 | 2026-11-07 | 718 |
| بساما | 41625340598198551 | 2023-11-08 | 2026-11-07 | 717 |
| تابان | 30714151942396396 | 2026-08-02 | 2026-11-07 | 67 |
| تاصيكو | 23293437377896568 | 2023-11-08 | 2026-11-07 | 718 |
| تاپيكو | 22560050433388046 | 2023-11-08 | 2026-11-07 | 718 |
| توان | 41927452991671109 | 2023-11-08 | 2026-11-07 | 717 |
| جوانه كوچك | 67455383896188985 | 2024-11-10 | 2026-11-07 | 479 |
| خبهمن | 26824673819862694 | 2023-11-08 | 2026-11-07 | 718 |
| خساپا | 44891482026867833 | 2023-11-08 | 2026-11-07 | 718 |
| خودرو | 65883838195688438 | 2023-11-08 | 2026-11-07 | 718 |
| دارونو | 34074071043606558 | 2024-11-26 | 2026-11-07 | 467 |
| دزاگرس | 43450146028916610 | 2024-01-31 | 2026-11-07 | 657 |
| ذوب | 71483646978964608 | 2023-11-08 | 2026-11-07 | 718 |
| شستا | 2400322364771558 | 2023-11-08 | 2026-11-07 | 718 |
| شپنا | 7745894403636165 | 2023-11-08 | 2026-11-07 | 718 |
| طعام | 31230051169165044 | 2025-05-28 | 2026-11-07 | 348 |
| فرابورس | 58741071099161284 | 2023-11-08 | 2026-11-07 | 717 |
| فزر | 8175784894140974 | 2023-11-08 | 2026-11-07 | 717 |
| فملي | 35425587644337450 | 2023-11-08 | 2026-11-07 | 718 |
| كاريس | 69067576215760005 | 2023-11-08 | 2026-11-07 | 717 |
| موج | 67141987086032267 | 2023-11-08 | 2026-11-07 | 717 |
| هم تراز | 51920757918600374 | 2025-09-14 | 2026-11-07 | 279 |
| وبصادر | 28320293733348826 | 2023-11-08 | 2026-11-07 | 718 |
| وبملت | 778253364357513 | 2023-11-08 | 2026-11-07 | 718 |
| وتجارت | 63917421733088077 | 2023-11-08 | 2026-11-07 | 718 |