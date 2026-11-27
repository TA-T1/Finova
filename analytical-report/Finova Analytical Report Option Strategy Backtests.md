# Finova Analytical Report: Option Strategy Backtests

Nov 26, 2026 · @ehsan

## 1. Summary

This report measures nine option strategy templates on 24 underlyings of the Iranian market. It uses the backtest engine (task 8), the option history and demo dataset (task 7), the three-year underlying data (EDA) and the feature table (features task). It is the written output of task 10.

**What a reader should remember**

1. **No real option price exists for a past expiry.** The database only holds contracts that are listed today, and none of them has expired yet (the first expiry is 2026-11-25). So every one of the 26,381 baseline trades is priced with Black-Scholes at HV20, not with a real quote. The backtest tests a strategy on model prices.
2. **The model prices are much lower than real market prices.** On the 8,131 real traded rows compared, the market price is above the model price in 89% of rows (median 1.33 times). For at-the-money options the market's implied volatility is 77%, against 34% for HV20, about 2.2 times higher.
3. **So the headline results flatter strategies that buy options.** At baseline, long straddle wins 58% of trades with a mean return on risk of +70%, and long strangle +105%. Priced at 2.2 times HV20 (the level the market actually showed), the same strategies turn negative: long straddle -14%, long strangle -23%, long put -35%, long call -2%.
4. **The period matters more than the strategy.** Stock prices rose in 2024 to 2026: the mean 20-day return was +2.9%, +3.2% and +7.5% in the three years, and 56% to 60% of 20-day windows were up. Strategies that gain from a rising market look better every year, and put strategies look worse.
5. **HV rank from the features task works as an entry filter inside the model.** Entering when HV rank (120 days) is low gives a higher win rate and a higher mean return for most templates: a higher mean return for 8 of 9 and a higher win rate for 7 of 9 (long straddle: 68% wins and +123%, against 60% and +70% when HV rank is high).

**What to do with it:** use the results to compare strategies with each other and to see how they react to the market, not to quote a profit figure. Re-run the same engine on real prices once the daily snapshot has collected expired contracts (section 10).

## 2. Data

The backtest uses two sources, and they are very different in length.

**1. Underlying stocks: three years, daily (EDA and features task).** 27 symbols from 2023-11-08 to 2026-11-07, 718 market days. After the 59 market-wide closures are removed there are 15,847 symbol-days: 14,916 traded days and 931 days where the symbol was halted. The prices are back-adjusted for 69 adjustment events (34 of them share increases), because raw prices show jumps at capital increases that would read as volatility. Four facts from the EDA matter here: daily returns are strongly serially correlated (median lag-1 autocorrelation 0.35, 26 of 27 symbols significant); volatility clusters (25 of 27); the daily price band changed during the period; and halted days are frequent for some symbols. The feature table (20 features per symbol-day, no look-ahead, checked by three tests) provides HV20 and HV rank.

**2. Option prices: 82 trading days (task 7 demo dataset).** The table below is what the committed dataset holds. It is taken from `quote_snapshots`, filled from TSETMC price history for contracts listed on 2026-11-24, plus one real live snapshot on that day.

| Item | Value |
| --- | --- |
| Contracts | 1,818 on 30 underlyings |
| Option-day rows | 59,642 |
| Period | 2026-07-29 to 2026-11-24 (82 trading days), median 30 rows per contract |
| Rows with at least one trade | 15,536 (26%) |
| Rows with bid and ask | 626, all from one day (2026-11-24) |
| Rows with open interest | 898 |
| Earliest expiry in the data | 2026-11-25 (no row is after an expiry) |

The five most traded underlyings in the option data are اهرم (188 trillion rial traded over the period), خودرو (41.8), شستا (29.3), وبملت (24.5) and فملی (21.3). اهرم alone is more than half of all option value.

**What this means for a backtest**

- A real-price backtest needs contracts that have expired. There are none, so the expiry calendar is synthetic: every Wednesday over the last three years, 2024 to 2026, for each underlying.
- Prices before the last 82 days do not exist for any option. They come from a model.
- The 24 underlyings used are those with at least 400 days of price history. Six were left out: تابان, طعام, هم تراز (short history) and اتکاسا, دوایکس, سامان (only a few days).
- **Data finding for the team:** the same stock appears under two spellings (Arabic and Persian letters, for example تاصیکو and تاصيكو). The raw names make 37 underlyings; after unifying the letters there are 30. Without it the option history and the price history of the same stock do not join. This report unifies the letters before the join. The collector and the database should do the same.
- Option rows where volume is zero (74%) carry only a settlement price, so they are weaker evidence of a price people could trade at.

## 3. Method

**Rule.** A backtest applies one rule to many expiries of one underlying: on the entry day, open a strategy built from a template, hold it to expiry (or close earlier on a target or stop), and record the result. Baseline rule: enter 30 calendar days before expiry (the last trading day on or before that date), hold to expiry, one contract per leg, 5% of starting equity per trade.

**Templates (nine, from the API contract).** Long call, long put, covered call, protective put, collar, bull call spread, bear put spread, long straddle, long strangle. Strikes are chosen among the listed strikes: ATM is the strike closest to the price, and "OTM 1" is one listed strike further out.

**Expiry calendar.** Every Wednesday from about 2024-01 to 2026-11, for each underlying (about 150 expiries per underlying). The calendar is synthetic because no real expired contract exists (section 2).

**Prices.** For every leg, the Black-Scholes price at the entry day, with HV20 as volatility (annualised with 240 trading days, daily returns above 12% dropped as capital actions), a 30% yearly risk-free rate and time = calendar days / 365. This is the same convention as the Go calculation layer. At expiry the option is worth its intrinsic value at the underlying's close.

**Costs.** Placeholders close to the Tehran tariff, not a broker's numbers: option commission 0.1% of premium on each trade, exercise fee 0.1% of strike value for each option that ends in the money, shares 0.4% to buy and 0.9% to sell.

**Skipped expiries.** A trade is skipped, and counted, when a leg is priced below the minimum premium (2,010 cases), the strike is outside the strike grid (2,000), the price jumped more than 12% in a day while the trade was open (281) or there is no trading day near the entry date (216). That is 4,507 skipped against 26,381 baseline trades.

**Measures.** For each trade: profit or loss in rial, and **return on risk** = profit divided by the strategy's maximum possible loss (the amount at risk). For each strategy: win rate (share of trades with a profit after fees), median and mean return on risk, and profit factor (gross wins divided by gross losses).

**Two choices made for this report**

- The engine's own `total_return` adds up thousands of returns and gives numbers such as +2,342% for protective put (median over underlyings), driven by a few trades: the return on risk of one protective put reached +8,269 times. So the report shows the **median**, and a **mean with returns limited to the range -100% to +500%**.
- Trades of the same expiry date are correlated (all underlyings move with the market). The 95% intervals therefore come from resampling whole expiry dates (1,500 resamples), not single trades.

The code that produced every number is in the report package (section 10).

## 4. Results by strategy (baseline)

Baseline: enter 30 days before expiry, hold to expiry, options priced at HV20. 26,381 trades on 24 underlyings.

| Strategy | Trades | Win rate | Median return on risk | Mean return on risk (95% CI) | Profit factor | Skipped |
| --- | --- | --- | --- | --- | --- | --- |
| long-call | 3,161 | 41% | -68% | 56% (30% to 82%) | 3.86 | 8% |
| bull-call-spread | 3,031 | 45% | -49% | 3% (-11% to 17%) | 1.38 | 12% |
| covered-call | 3,031 | 56% | 2% | -0% (-1% to 1%) | 1.55 | 12% |
| protective-put | 3,112 | 44% | -28% | 100% (70% to 133%) | 5.62 | 9% |
| collar | 2,602 | 55% | 28% | 34% (21% to 47%) | 2.03 | 24% |
| long-put | 3,112 | 37% | -100% | 44% (19% to 71%) | 1.14 | 9% |
| bear-put-spread | 2,618 | 39% | -100% | 23% (3% to 44%) | 0.97 | 24% |
| long-straddle | 3,112 | 58% | 28% | 70% (54% to 87%) | 4.83 | 9% |
| long-strangle | 2,602 | 49% | -7% | 105% (79% to 131%) | 5.73 | 24% |

**How to read it**

- **Win rate and mean tell different stories.** Long call, long put and the spreads lose on a typical trade (median -49% to -100%) and make it back with a few large wins. A median of -100% means that more than half of the trades lose the whole premium. This is the normal shape of buying options.
- **Spreads are the weakest.** Bull call spread has a mean return near zero and bear put spread is the only strategy with a profit factor below 1. Selling the far leg gives up the large wins.
- **Covered call is the quietest.** It wins 56% of the time, but its amount at risk is the whole share purchase, so its return in percent is close to zero.
- **Collar and protective put include a share leg.** Their result is mostly the share leg rising over the period (section 5), not the options.
- **Long straddle has the best win rate (58%) and long strangle the best mean (105%).** Both are bets on a large move, and the sample had large moves: over the 30 days of a trade the underlying moved +1% at the median and +3.7% on average (log change).
- **Fees are small for option-only strategies** (a median of 0.1% to 3% of the amount at risk) and large for strategies with shares (collar 27%, protective put 53%), because of the 0.4% and 0.9% share fees against a small amount at risk.
- **Skipped share is highest (24%) for collar, bear put spread and long strangle.** They need a strike further out of the money, and such a leg is more often missing from the strike grid or priced below the minimum premium.

&#91;image: Win rate, median and mean return on risk by strategy\]

## 5. Results by underlying and by year

**By underlying.** The chart below shows the mean return on risk for every underlying and strategy. Pooled over all nine strategies, the mean return on risk per underlying runs from 23% (خساپا) and 29% (خودرو) at the bottom to 62% (فملی and دزاگرس) and 71% (فزر) at the top. The differences between underlyings are smaller than the differences between strategies, and they are not a safe way to choose an underlying: weekly trades on one underlying overlap heavily, and three years is a single market cycle. The order of the strategies is much more stable: it is close to the same on every underlying (average rank correlation 0.84 with the overall order). Long strangle, protective put and long straddle are in the top five on all 24 underlyings, and bull call spread and covered call are in the bottom four on 23 of 24.

&#91;image: Mean return on risk by underlying and strategy\]

**By year (mean return on risk, baseline).**

| Strategy | 2024 | 2025 | 2026 (to Nov) |
| --- | --- | --- | --- |
| long-call | 26% | 56% | 94% |
| bull-call-spread | -11% | 5% | 14% |
| covered-call | -1% | -0% | 1% |
| protective-put | 67% | 107% | 129% |
| collar | 22% | 34% | 45% |
| long-put | 64% | 46% | 17% |
| bear-put-spread | 32% | 28% | 7% |
| long-straddle | 54% | 68% | 91% |
| long-strangle | 65% | 107% | 139% |

The direction of each strategy follows the market. Call-side and share strategies improve from 2024 to 2026 as the 20-day return of the underlying rises (+2.9%, +3.2%, +7.5%). Put-side strategies fall. Straddle and strangle improve as volatility rises: the median HV20 goes from 26% to 29% to 35%. Part of the 2024 results also comes from a different trading regime: the daily price band was wide until August 2024, then 3%, and since June 2025 4%.

| Year | HV20 at entry | HV over the next 20 days | Ratio | Mean 20-day return | Share of 20-day windows up |
| --- | --- | --- | --- | --- | --- |
| 2024 | 26% | 27% | 1.02 | 2.9% | 58% |
| 2025 | 29% | 29% | 1.01 | 3.2% | 56% |
| 2026 | 35% | 34% | 0.98 | 7.5% | 60% |

HV20 is on average a fair forecast of the next 20 days' volatility (ratio about 1.0). So the baseline gains of buying options do **not** come from the model underestimating future volatility. They come from the drift of the underlying: the mean 20-day log return is +4.1%, about 49% a year, while the model assumes 30% (the 30% yearly rate), and from the model price being too low for what the market charges (next section).

&#91;image: Volatility over the next 20 days against HV20 at entry, and the 20-day return\]

## 6. Is the model price realistic?

The backtest prices every option with Black-Scholes at HV20. The 82 days of real option history let us check this directly. For each real traded row (volume above zero) up to 2026-11-07, the model price was computed from the underlying price of that day, the contract's strike and expiry, and HV20 of that day. 8,131 rows were compared.

**Result: the market charges clearly more.** The market price is higher than the model price in 89% of rows, and the median ratio is 1.33. For at-the-money options (strike within 5% of the price; 1,115 rows) the median market implied volatility is 77% and the median HV20 is 34%. In 94% of these rows implied volatility is above HV20.

| Type | Strike / price | Rows | Market / model (median) | Market IV / HV20 (median) |
| --- | --- | --- | --- | --- |
| call | deep in the money (below 85%) | 3,459 | 1.08 | 3.33 |
| call | 85% to 95% | 988 | 1.33 | 2.35 |
| call | at the money (95% to 105%) | 936 | 1.83 | 2.30 |
| call | 105% to 115% | 771 | 2.90 | 2.28 |
| call | above 115% | 1,079 | 11.90 | 2.42 |
| put | below 85% (far out of the money) | 223 | 17.03 | 1.88 |
| put | 85% to 95% | 228 | 4.17 | 1.80 |
| put | at the money (95% to 105%) | 184 | 2.13 | 1.80 |
| put | 105% to 115% | 124 | 1.43 | 1.83 |
| put | above 115% (deep in the money) | 139 | 1.12 | 2.90 |

&#91;image: Real traded price against the Black-Scholes price, and implied volatility against HV20\]

**Three readings**

- **Deep in-the-money options match the model** (ratio 1.08 and 1.12), because their price is mostly intrinsic value. The gap is all in the time value of options that are at or out of the money.
- **The gap grows with distance from the money.** Far out-of-the-money calls cost 12 times the model price and far out-of-the-money puts 17 times. HV20 with a normal distribution gives these options almost no value; the market does not. This is the usual volatility smile, and the backtest cannot reproduce it.
- **The gap also grows with time to expiry:** implied volatility over HV20 is 1.8 for 15 to 30 days, 2.2 for 31 to 60 days and 2.5 for more than 60 days (at-the-money calls).

**How far to trust this check.** It covers only the last four months, when volatility was already high (median HV20 35%). Rows with a trade are 26% of all rows; prices of far out-of-the-money options are tiny, so one price tick moves the ratio a lot; and the market implied volatility uses the same 30% rate as the model. The size of the gap in 2024 and 2025 is unknown. The direction is not in doubt: a backtest at HV20 prices buys options cheaper than the market sold them in 2026.

## 7. Sensitivity: entry time, exit rule and option price level

Each cell shows **win rate / mean return on risk**. The first column is the baseline.

| Strategy | 30d, hold | 14d, hold | 7d, hold | 30d, target/stop 50% | 1.5x HV20 | 2.2x HV20 |
| --- | --- | --- | --- | --- | --- | --- |
| long-call | 41% / 56% | 39% / 45% | 37% / 33% | 46% / 3% | 38% / 27% | 33% / -2% |
| bull-call-spread | 45% / 3% | 44% / 12% | 41% / 12% | 48% / -1% | 45% / -0% | 45% / -0% |
| covered-call | 56% / -0% | 51% / -1% | 45% / -1% | 56% / -0% | 62% / 1% | 69% / 4% |
| protective-put | 44% / 100% | 39% / 62% | 33% / 29% | 41% / -30% | 41% / 64% | 36% / 21% |
| collar | 55% / 34% | 50% / 5% | 46% / -6% | 54% / -17% | 56% / 42% | 57% / 52% |
| long-put | 37% / 44% | 35% / 27% | 34% / 10% | 46% / 7% | 31% / -1% | 25% / -35% |
| bear-put-spread | 39% / 23% | 36% / 6% | 37% / -1% | 45% / 0% | 36% / -1% | 35% / -18% |
| long-straddle | 58% / 70% | 56% / 54% | 51% / 35% | 63% / 23% | 46% / 21% | 32% / -14% |
| long-strangle | 49% / 105% | 39% / 65% | 28% / 27% | 55% / 22% | 39% / 28% | 27% / -23% |

**What changes the picture**

- **Option price level is the biggest factor.** Raising the option price from 1.0 to 2.2 times HV20 moves long straddle from +70% to -14%, long strangle from +105% to -23% and long put from +44% to -35%. At 1.5 times long straddle (+21%) and long strangle (+28%) are still positive and long put is flat (-1%). Long call falls from +56% to -2%.
- **Strategies that sell options gain when options are expensive.** Covered call wins 69% of the time at 2.2 times HV20 (against 56%) and its mean return becomes +4%, the only clearly positive result that does not depend on buying cheap options. Collar improves from +34% to +52%, but its share leg also contributes.
- **Shorter holding periods help less.** Entering 14 or 7 days before expiry lowers the mean of every strategy except bull call spread (long strangle: 105%, 65%, 27%). The edge in the model comes from the longer horizon in which the underlying drifts.
- **A 50% target and stop raises the win rate** (long put 37% to 46%, long straddle 58% to 63%) **but cuts the mean** (long call 56% to 3%, protective put 100% to -30%), because it cuts the few large winners. The exit rule is checked at each daily close using model prices, which is a rough approximation of real intraday trading.
- **The bull call spread is insensitive to price level** (3%, 0%, 0%), because buying and selling options at similar prices cancels most of the level. The bear put spread is not: 23%, -1%, -18%.

&#91;image: Win rate and mean return on risk under six rule and price scenarios\]

## 8. Link to the features task: HV rank and trend at entry

The features table gives, for every symbol and day, values known at the close of that day (and checked by three no-look-ahead tests). Two of them were joined to each trade by symbol and entry date: HV rank over 120 days (position of today's HV20 in its last 120 active days, 0 to 100) and the 20-day return. The join covers 19,336 of the 26,381 baseline trades (73%); the rest fall in the first months of a symbol's history, where HV rank does not exist yet. The three HV rank groups were fixed in advance (0-33, 33-67, 67-100) and not tuned.

**HV rank at entry (win rate / mean return on risk)**

| Strategy | HV rank 0-33 | HV rank 33-67 | HV rank 67-100 |
| --- | --- | --- | --- |
| long-call | 52% / 112% | 48% / 86% | 46% / 73% |
| bull-call-spread | 55% / 23% | 53% / 18% | 51% / 15% |
| covered-call | 62% / -0% | 62% / 0% | 59% / 1% |
| protective-put | 55% / 153% | 51% / 141% | 49% / 123% |
| collar | 64% / 57% | 61% / 43% | 58% / 41% |
| long-put | 32% / 47% | 33% / 26% | 34% / 26% |
| bear-put-spread | 31% / 15% | 35% / 10% | 37% / 10% |
| long-straddle | 68% / 123% | 64% / 87% | 60% / 70% |
| long-strangle | 63% / 198% | 55% / 139% | 51% / 103% |

- **A low HV rank before entry goes with better results** for every strategy that buys options, and the long volatility strategies gain the most (long strangle +198% against +103%). It matches the idea that quiet periods are followed by larger moves and that options bought when volatility is low are cheap.
- **The effect is exaggerated by the pricing.** In the backtest the option price is HV20 itself, so a low HV rank means a cheap model option by construction. The real market prices at about twice HV20, so the filter has to be tested again on real prices before it is used as a rule.

&#91;image: Mean return on risk by HV rank group at entry\]

**Trend at entry (20-day return above or below zero), directional strategies**

| Strategy | Win rate, trend down | Win rate, trend up | Mean return, trend down | Mean return, trend up |
| --- | --- | --- | --- | --- |
| long-call | 45% | 49% | 71% | 95% |
| bull-call-spread | 50% | 54% | 10% | 23% |
| long-put | 35% | 32% | 35% | 28% |
| bear-put-spread | 39% | 33% | 22% | 4% |

Trades opened after a rising month do better for bullish strategies and worse for bearish ones, and the other way round after a falling month. This agrees with the EDA: daily returns of these stocks are positively serially correlated (median lag-1 autocorrelation 0.35), so a recent move tends to continue. The 20-day return is therefore a candidate direction signal for the strategy builder, again to be confirmed on real prices.

## 9. Limitations

1. **All prices are model prices.** No real option price exists for a past expiry (section 2), so the results describe strategies on Black-Scholes prices at HV20, not what a trader would have paid. The engine marks this on every run (`model_priced_share = 1.0`) and the front end should show it.
2. **The model price is too low.** Real prices are about 1.3 times the model at the median and 2.2 times in volatility terms (section 6). Results that depend on cheap options (long straddle, long strangle, long put, long call) are overstated; results that depend on selling options (covered call) are understated.
3. **No bid and ask in the history.** Trades are filled at the last or model price. Real fills cross a spread that is unknown; bid and ask exist for one day only (626 rows on 2026-11-24). Illiquid options (74% of option-days had no trade) could not be traded at the assumed price.
4. **The model is simple.** European exercise, a constant 30% rate, no smile, no skew, no dividends, HV20 as volatility. Far out-of-the-money options are priced 12 to 17 times too low.
5. **Synthetic calendar and strikes.** Weekly Wednesday expiries and a strike grid scaled from today's listed strikes are not the real expiry months and strikes of the past.
6. **Settlement at the closing price.** The engine uses the underlying's close on the expiry day; the real exchange settles on an average of the last days. Fees are placeholders, not a broker's tariff.
7. **One market cycle.** Three years of a rising market, with a changing price band and a regime of rising volatility. There is no out-of-sample test, and the 95% intervals only describe sampling noise inside this period.
8. **Return measures.** Return on risk is unstable when the amount at risk is small (protective put reached +8,269 times on one trade). The engine's `total_return` and `max_drawdown` come from adding overlapping trades without compounding and should not be read as one account's balance. The report therefore uses the median and a limited mean.
9. **Data quality.** The same stock exists under two spellings in the option data; six underlyings with short history were not used; the underlying prices are back-adjusted with information about later events, which does not change ratios and returns but does change price levels.
10. **Task 9 results were not in the repository.** The runs here were made with the task 8 engine on the task 7 dataset, with the settings written in the Method section. They can be replaced by the stored `backtest_runs` rows once task 9 is merged.

## 10. Recommendations and next steps

1. **Do not present a profit figure from the baseline.** Present the strategy comparison and the sensitivity to the option price level (sections 4 and 7). If one number is needed for the demo, use the 1.5 times HV20 scenario and say that it is model-priced.
2. **Price the backtest at market level.** Add a volatility input to the engine (a multiplier on HV20, or the implied volatility observed in `quote_snapshots`, ideally by strike and expiry) so that model prices follow the market. This changes the sign of five of the nine strategies.
3. **Switch to real prices as soon as expiries exist.** The daily snapshot is running and the first contracts expire on 2026-11-25. After four to eight weekly expiries, run `--expiries observed` and compare with the model-priced result. Put the model-priced and the real-priced result side by side in the results page (tasks 11 and 18).
4. **Fix the measures before storing runs (task 9).** Store the median and a limited mean next to `total_return`, store the pricing scenario in `params`, and keep `model_priced_share`.
5. **Normalise symbol names** (Arabic and Persian letters) in the collector and the database, so option history and price history join without a workaround.
6. **Test two entry filters from the features table on real prices:** HV rank (120 days) below 33 for strategies that buy volatility, and the sign of the 20-day return for directional strategies.
7. **Replace placeholders:** the broker's fee tariff, and the settlement rule if the exchange's averaging is known.

**Reproduce.** The folder `analytical-report/` of the repository contains the scripts and every table behind the numbers: `run_backtests.py` (all strategies on all underlyings and six pricing and rule scenarios, about a minute), `analyze.py` (tables and charts), `make_tables.py` (text tables) and the CSV files in `outputs/`. All inputs are in the repository on `main`: the engine `backtest/` (task 8), the demo dataset `backend/testdata/demo/` (task 7), the adjusted price file `data-pipeline/output/underlyings_daily.csv` and the feature table `EDA/outputs/features/features.parquet` (features task). The folder was rerun from a clean clone of `main` and gave identical tables.

**Sources:** project repository `TA-T1/Finova` (task documents `docs/tasks/task-07-*` and `task-08-*`, `docs/strategy-backtest.md`, `backtest/README.md`); the EDA outputs and the feature table of this project.
