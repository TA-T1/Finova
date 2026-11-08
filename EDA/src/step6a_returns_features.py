"""Step 6a - returns and volume / flow features on the corrected, trading-calendar panel.

Inputs : raw daily file, step 3 (trading_status), step 4 (adjusted_series, adj_event_final).
Outputs (outputs/step6a_returns_features/):
  features_daily.csv        one row per (symbol, trading-calendar record) with returns, flags, volume and flow features
  feature_summary.csv       per feature: count, share NaN, quantiles
  coverage_by_symbol.csv    usable rows per symbol / feature
  extreme_returns.csv       returns beyond the period-specific band
  leakage_check.csv         proof that rolling features only use past data
  checks_summary.csv, summary.json, return_hist.png, feature_coverage.png

Rules (policy of steps 3-4):
  * market-closure days are removed from the calendar; halted rows get NaN returns (never 0)
  * return = log(adj_final_corrected_t / adj_final_corrected of the previous ACTIVE row); the adjusted chain already
    contains every corporate action, so no event-day jump remains
  * flags: is_reopen (halt before the row), halt_len, first_trade, after_gap (missing record), reopen_share_increase
  * rolling features use shift(1) windows over ACTIVE rows only (min_periods), so no look-ahead
"""
from __future__ import annotations

import json
import sys
from pathlib import Path
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

from common import load_daily, market_closure_days, out_dir, save_table


def main() -> None:

    OUT = out_dir("step6a_returns_features")
    S4 = OUT.parent / "step4_adjustments"
    checks: list[dict] = []


    def add(i, name, status, n, detail):
        checks.append({"id": i, "check": name, "status": status, "n_issues": n, "detail": detail})


    df = load_daily()
    closure = set(market_closure_days(df))
    adj = pd.read_csv(S4 / "adjusted_series.csv", encoding="utf-8-sig", parse_dates=["date"])
    evf = pd.read_csv(S4 / "adj_event_final.csv", encoding="utf-8-sig", parse_dates=["date"])
    df = df.merge(adj[["symbol", "date", "adj_factor_corrected", "adj_final_corrected", "price_anomaly", "share_factor",
                       "adj_volume", "volume_adj_uncertain", "ind_buy_vol_adj", "ind_sell_vol_adj", "corp_buy_vol_adj",
                       "corp_sell_vol_adj"]], on=["symbol", "date"], how="left", validate="1:1")
    assert len(df) == 17368
    n_rows_raw = len(df)
    df["in_closure"] = df["date"].isin(closure)
    df = df[~df["in_closure"]].reset_index(drop=True)          # trading calendar only
    add("S6A.01", "market-closure days removed from the panel", "PASS", 0, f"{n_rows_raw - len(df)} rows removed, {len(df)} kept")

    df["adj_last_c"] = df["last_price"] * df["adj_factor_corrected"]
    df["active"] = ~df["is_halted"]
    df = df.sort_values(["symbol", "date"]).reset_index(drop=True)
    df["row"] = df.groupby("symbol").cumcount()

    # first real trade per symbol
    first_act = df[df["active"]].groupby("symbol")["row"].min()
    df["first_trade"] = df["row"] == df["symbol"].map(first_act)
    df["pre_listing"] = df["row"] < df["symbol"].map(first_act)

    # previous active row of the same symbol
    act_rows = df.loc[df["active"], ["symbol", "row", "date", "adj_final_corrected", "adj_last_c", "value"]].copy()
    act_rows["prev_row_active"] = act_rows.groupby("symbol")["row"].shift(1)
    act_rows["prev_date"] = act_rows.groupby("symbol")["date"].shift(1)
    act_rows["prev_adj_final"] = act_rows.groupby("symbol")["adj_final_corrected"].shift(1)
    act_rows["prev_adj_last"] = act_rows.groupby("symbol")["adj_last_c"].shift(1)
    df = df.merge(act_rows[["symbol", "row", "prev_row_active", "prev_date", "prev_adj_final", "prev_adj_last"]],
                  on=["symbol", "row"], how="left")
    n_between = df["row"] - df["prev_row_active"] - 1                 # records between the two active rows
    halted_between = np.where(df["active"], n_between, np.nan)
    df["halt_len"] = halted_between                                    # halted records (symbol-specific) before this row
    # calendar days missing without a record = NO_RECORD gaps
    cal = np.sort(df["date"].unique())
    cpos = pd.Series(np.arange(len(cal)), index=cal)
    df["cal_gap"] = np.where(df["active"] & df["prev_date"].notna(), df["date"].map(cpos) - df["prev_date"].map(cpos) - 1, np.nan)
    df["after_gap"] = (df["cal_gap"] - df["halt_len"]) > 0
    df["is_reopen"] = df["halt_len"] > 0

    valid = df["active"] & df["prev_adj_final"].notna() & ~df["first_trade"] & ~df["price_anomaly"]
    df["ret"] = np.where(valid, np.log(df["adj_final_corrected"] / df["prev_adj_final"]), np.nan)
    df["ret_last"] = np.where(valid, np.log(df["adj_last_c"] / df["prev_adj_last"]), np.nan)

    # share-increase reopen rows (first active row on/after the event)
    si = evf[evf["event_class"].str.startswith("SHARE_INCREASE")]
    df["reopen_share_increase"] = False
    for r in si.itertuples():
        m = (df["symbol"] == r.symbol) & (df["date"] >= r.date) & df["active"]
        idx = df.index[m]
        if len(idx):
            df.loc[idx[0], "reopen_share_increase"] = True
    df["ret_use"] = np.where((df["halt_len"] > 10) | df["reopen_share_increase"] | df["after_gap"], np.nan, df["ret"])
    df["ret_clean"] = df["ret_use"]          # alias: the series meant for distributions / volatility

    add("S6A.02", "returns are NaN on halted rows, pre-listing rows, first trade and price-anomaly rows", "PASS" if df.loc[~valid, "ret"].isna().all() else "FAIL",
        int(df.loc[~valid, "ret"].notna().sum()), f"{int(valid.sum())} valid returns of {int(df['active'].sum())} active rows; halted rows never get 0")

    # consistency with the exchange's own reopen return (non-event rows)
    exch = df["final_price"] / df["prev_final"] - 1
    ne = valid & ~df["adj_event"]
    dev = (np.exp(df.loc[ne, "ret"]) - 1 - exch[ne]).abs()
    add("S6A.03", "return equals final/prev_final-1 on ordinary rows (non-event, previous record active)",
        "PASS" if dev[df.loc[ne, "halt_len"] == 0].max() < 1e-4 else "WARN",
        int((dev[df.loc[ne, "halt_len"] == 0] > 1e-4).sum()),
        f"max deviation {dev[df.loc[ne, 'halt_len'] == 0].max():.1e} on {int((df.loc[ne, 'halt_len'] == 0).sum())} rows; "
        f"{int((df.loc[ne, 'halt_len'] > 0).sum())} reopen rows differ by up to {dev[df.loc[ne, 'halt_len'] > 0].max():.1e} "
        f"(prev_final of the reference vs last real close)")

    # period-specific move band (step 2): up to Aug-2024 ~10%, Sep-2024..Jun-2025 3%, from Jul-2025 4%
    cap = np.where(df["date"] < "2024-09-01", 0.10, np.where(df["date"] < "2025-07-01", 0.03, 0.04))
    df["band_cap"] = cap
    df["ret_extreme"] = df["ret_clean"].abs() > (np.log1p(cap) + 0.015)
    ext = df[df["ret_extreme"]][["symbol", "date", "ret_clean", "band_cap", "halt_len", "is_reopen", "adj_event"]]
    save_table(ext, OUT / "extreme_returns.csv")
    add("S6A.04", "clean returns beyond the period band (+1.5%)", "WARN" if len(ext) else "PASS", len(ext),
        f"{len(ext)} rows ({len(ext) / df['ret_clean'].notna().sum():.2%} of clean returns); {int(ext['is_reopen'].sum())} are reopen rows")

    # ---------------- range / volatility
    df["adj_high_c"] = df["high"] * df["adj_factor_corrected"]
    df["adj_low_c"] = df["low"] * df["adj_factor_corrected"]
    ok = df["active"] & (df["low"] > 0) & (df["high"] >= df["low"])
    df["hl_range"] = np.where(ok, np.log(df["adj_high_c"] / df["adj_low_c"]), np.nan)
    df["parkinson_var"] = df["hl_range"] ** 2 / (4 * np.log(2))
    df["intraday_ret"] = np.where(ok & (df["open"] > 0), np.log(df["last_price"] / df["open"]), np.nan)


    def roll_active(g: pd.DataFrame, col: str, fn: str, win: int, minp: int) -> pd.Series:
        """Rolling statistic over previous ACTIVE rows only (shifted by one active row -> no look-ahead); value placed on all rows."""
        s = g[col].where(g["active"])
        a = s.dropna()
        r = getattr(a.shift(1).rolling(win, min_periods=minp), fn)()
        return r.reindex(g.index)


    feat_cols = {}
    parts = []
    for sym, g in df.groupby("symbol", sort=True):
        g = g.copy()
        # past-only baselines (value known BEFORE today)
        g["value_med20"] = roll_active(g, "value", "median", 20, 10)
        g["tcount_med20"] = roll_active(g, "trade_count", "median", 20, 10)
        g["advol_med20"] = roll_active(g.assign(adv=np.where(g["volume_adj_uncertain"], np.nan, g["adj_volume"])), "adv", "median", 20, 10)
        g["vol20"] = roll_active(g, "ret_clean", "std", 20, 10)
        g["vol60"] = roll_active(g, "ret_clean", "std", 60, 30)
        g["park20"] = np.sqrt(roll_active(g, "parkinson_var", "mean", 20, 10))
        parts.append(g)
    df = pd.concat(parts).sort_values(["symbol", "date"]).reset_index(drop=True)

    # volume features
    df["log_value"] = np.where(df["active"], np.log(df["value"].where(df["value"] > 0)), np.nan)
    df["value_rel20"] = np.where(df["active"], df["value"] / df["value_med20"], np.nan)
    df["tcount_rel20"] = np.where(df["active"], df["trade_count"] / df["tcount_med20"], np.nan)
    adv = np.where(df["volume_adj_uncertain"], np.nan, df["adj_volume"])
    df["advol_rel20"] = np.where(df["active"], adv / df["advol_med20"], np.nan)
    df["avg_trade_value"] = np.where(df["active"] & (df["trade_count"] > 0), df["value"] / df["trade_count"], np.nan)
    df["log_value_rel20"] = np.log(df["value_rel20"])
    # flow features (ratios only - totals disagree by up to 1.9% for 63 rows, step 2)
    tot = df["ind_buy_val"] + df["corp_buy_val"]
    df["ind_buy_share"] = np.where(df["active"], df["ind_buy_val"] / tot, np.nan)                 # retail share of buying
    df["ind_sell_share"] = np.where(df["active"], df["ind_sell_val"] / (df["ind_sell_val"] + df["corp_sell_val"]), np.nan)
    df["ind_net_ratio"] = np.where(df["active"] & (df["value"] > 0), df["ind_net_value"] / df["value"], np.nan)
    bavg = df["ind_buy_val"] / df["ind_buy_count"].where(df["ind_buy_count"] > 0)
    savg = df["ind_sell_val"] / df["ind_sell_count"].where(df["ind_sell_count"] > 0)
    df["ind_power_ratio"] = np.where(df["active"], bavg / savg, np.nan)                            # buyer power (avg retail buy size / sell size)
    df["log_ind_power"] = np.log(df["ind_power_ratio"])
    for c in ["ind_buy_share", "ind_sell_share", "ind_net_ratio"]:
        df.loc[~df["ind_buy_val"].notna(), c] = np.nan

    # turnover-style relative range
    df["range_rel20"] = df["hl_range"] / df["park20"].where(df["park20"] > 0)
    # days locked at the daily price limit: range ~0 although the return is large -> range is censored
    df["limit_locked"] = df["active"] & (df["hl_range"] < 0.002)

    # ---------------- leakage check: today's value must not change the feature of today
    leak = []
    for sym in ["خودرو", "اخابر", "ذوب"]:
        g = df[df["symbol"] == sym].reset_index(drop=True)
        i = int(g.index[g["active"]][200])
        g2 = g.copy()
        g2.loc[i, "value"] *= 100
        g2.loc[i, "ret_clean"] = 0.5
        r1 = roll_active(g, "value", "median", 20, 10).iloc[i]
        r2 = roll_active(g2, "value", "median", 20, 10).iloc[i]
        v1 = roll_active(g, "ret_clean", "std", 20, 10).iloc[i]
        v2 = roll_active(g2, "ret_clean", "std", 20, 10).iloc[i]
        leak.append(dict(symbol=sym, row=i, median_unchanged=bool(np.isclose(r1, r2, equal_nan=True)), vol_unchanged=bool(np.isclose(v1, v2, equal_nan=True))))
    L = pd.DataFrame(leak)
    save_table(L, OUT / "leakage_check.csv")
    add("S6A.05", "no look-ahead: perturbing today's value/return leaves today's rolling baselines unchanged",
        "PASS" if L[["median_unchanged", "vol_unchanged"]].all().all() else "FAIL", int((~L[["median_unchanged", "vol_unchanged"]].all(axis=1)).sum()),
        f"tested on {len(L)} symbols")

    # ---------------- save
    keep = ["symbol", "date", "active", "pre_listing", "first_trade", "is_reopen", "halt_len", "after_gap", "adj_event", "reopen_share_increase",
            "price_anomaly", "volume_adj_uncertain", "ret", "ret_last", "ret_clean", "ret_extreme", "band_cap", "hl_range", "intraday_ret",
            "vol20", "vol60", "park20", "range_rel20", "log_value", "value_rel20", "log_value_rel20", "tcount_rel20", "advol_rel20",
            "avg_trade_value", "limit_locked", "ind_buy_share", "ind_sell_share", "ind_net_ratio", "ind_power_ratio", "log_ind_power"]
    F = df[keep].copy()
    save_table(F, OUT / "features_daily.csv")

    num = ["ret_clean", "hl_range", "vol20", "vol60", "log_value", "value_rel20", "tcount_rel20", "advol_rel20", "ind_buy_share", "ind_net_ratio", "log_ind_power", "range_rel20"]
    rows = []
    for c in num:
        x = F[c].dropna()
        q = x.quantile([.01, .05, .5, .95, .99])
        rows.append(dict(feature=c, n=len(x), share_nan_all_rows=1 - len(x) / len(F), mean=x.mean(), std=x.std(), p01=q[.01], p05=q[.05], p50=q[.5], p95=q[.95], p99=q[.99]))
    save_table(pd.DataFrame(rows), OUT / "feature_summary.csv")

    cov = F[F["active"]].groupby("symbol").agg(active_rows=("active", "size"), clean_ret=("ret_clean", "count"), vol20=("vol20", "count"),
                                               value_rel20=("value_rel20", "count"), advol_rel20=("advol_rel20", "count"),
                                               flows=("ind_buy_share", "count")).reset_index()
    cov["clean_ret_share"] = cov["clean_ret"] / cov["active_rows"]
    save_table(cov, OUT / "coverage_by_symbol.csv")
    add("S6A.06", "flow features exist only where real/legal columns exist", "INFO", None,
        f"ind_buy_share non-NaN on {F['ind_buy_share'].notna().sum()} of {int(F['active'].sum())} active rows")
    inf = int(np.isinf(F.select_dtypes('number')).sum().sum())
    add("S6A.07", "no infinite values in features", "PASS" if inf == 0 else "FAIL", inf, "")
    cr = F["ret_clean"].dropna()
    add("S6A.08", "clean return distribution", "INFO", len(cr),
        f"n={len(cr)}, std={cr.std():.4f}, p01={cr.quantile(.01):.3f}, p99={cr.quantile(.99):.3f}, kurtosis={cr.kurt():.1f}, skew={cr.skew():.2f}")
    dropped = F["ret"].notna() & F["ret_clean"].isna()
    add("S6A.09", "returns kept in 'ret' but removed from 'ret_clean'", "INFO", int(dropped.sum()),
        f"{int((F.loc[dropped,'halt_len'] > 10).sum())} after >10-day halts, {int(F.loc[dropped,'reopen_share_increase'].sum())} share-increase reopens, {int(F.loc[dropped,'after_gap'].sum())} after a missing record")
    vol_sym = F.groupby("symbol")["ret_clean"].std()
    add("S6A.10", "daily volatility by symbol (clean returns)", "INFO", None,
        f"min {vol_sym.min():.4f} ({vol_sym.idxmin()}), median {vol_sym.median():.4f}, max {vol_sym.max():.4f} ({vol_sym.idxmax()})")
    corr = F[["ret_clean", "hl_range", "value_rel20", "tcount_rel20", "ind_net_ratio"]].assign(abs_ret=F["ret_clean"].abs()).corr(method="spearman")
    add("S6A.11", "Spearman: |ret| vs hl_range / value_rel20 (sanity)", "INFO", None,
        f"|ret|~hl_range {corr.loc['abs_ret','hl_range']:.2f}; |ret|~value_rel20 {corr.loc['abs_ret','value_rel20']:.2f}; ret~ind_net_ratio {corr.loc['ret_clean','ind_net_ratio']:.2f}")
    save_table(corr.reset_index(), OUT / "feature_correlations.csv")
    xx = F.dropna(subset=["ret_clean", "hl_range"])
    lk = xx["limit_locked"]
    add("S6A.12", "price-limit locked days (range ~0) censor the range feature", "WARN", int(lk.sum()),
        f"{lk.mean():.1%} of rows with a return; mean |ret| locked {xx[lk]['ret_clean'].abs().mean():.3f} vs {xx[~lk]['ret_clean'].abs().mean():.3f} unlocked; "
        f"hence the negative |ret|~range correlation; use limit_locked together with range features")

    pd.DataFrame(checks).to_csv(OUT / "checks_summary.csv", index=False, encoding="utf-8-sig")
    json.dump(dict(rows=len(F), active=int(F["active"].sum()), clean_returns=int(len(cr)), extreme=len(ext)), open(OUT / "summary.json", "w"), indent=2)

    # charts
    fig, ax = plt.subplots(1, 2, figsize=(11, 4))
    ax[0].hist(cr.clip(-0.12, 0.12), bins=120, color="#2166ac")
    ax[0].set_title("Clean daily log returns (clipped at +/-12%)")
    ax[0].set_yscale("log")
    for k, (a, b) in enumerate([("2023-11-01", "2024-09-01"), ("2024-09-01", "2025-07-01"), ("2025-07-01", "2026-12-01")]):
        x = F[(F["date"] >= a) & (F["date"] < b)]["ret_clean"].dropna()
        ax[1].hist(x.clip(-0.12, 0.12), bins=80, alpha=0.5, label=f"{a[:7]}..{b[:7]} (n={len(x)})")
    ax[1].set_title("By price-band period")
    ax[1].legend(fontsize=7)
    fig.tight_layout()
    fig.savefig(OUT / "return_hist.png", dpi=150)
    plt.close(fig)
    fig, ax = plt.subplots(figsize=(9, 5))
    cv = cov.sort_values("clean_ret_share")
    ax.barh(range(len(cv)), cv["clean_ret_share"], color="#4d9221")
    ax.set_yticks(range(len(cv)))
    ax.set_yticklabels([f"S{i+1:02d}" for i in range(len(cv))], fontsize=7)
    ax.set_xlabel("share of active rows with a clean return")
    fig.tight_layout()
    fig.savefig(OUT / "feature_coverage.png", dpi=150)
    plt.close(fig)
    print(pd.DataFrame(checks)[["id", "status", "n_issues", "detail"]].to_string(max_colwidth=230))


if __name__ == "__main__":
    main()
