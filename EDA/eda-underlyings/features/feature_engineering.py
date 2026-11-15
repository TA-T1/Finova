"""Sprint 2, task 7 - feature engineering for the underlying stocks (model inputs, one row per symbol and trading day).

Pipeline:  build_panel()  ->  compute_features(panel)  ->  features table (parquet / csv / optional SQL table)

Look-ahead rule: every feature of day t uses ONLY rows with date <= t of the same symbol. `compute_features` is a pure
function of the panel and uses only trailing rolling / ewm windows (no shift(-k), no centred windows, no global
statistics). `lookahead_check.py` proves it by truncation and by corrupting the future.

Conventions (decisions of steps 3-6 and of the sprint review):
  * market-closure days are not in the panel; halted rows stay in the table but ALL features are NaN (never 0, never filled)
  * windows count ACTIVE trading rows of the symbol, not calendar days
  * prices = corrected backward-adjusted prices (step 4); the adjusted chain only rescales levels, so every feature
    below is scale-free (log returns, ratios, percentages) and equals what would have been computed live on day t
  * returns that jump over a long halt (> 10 days), over a missing record, or over an adjustment event on a reopen day
    are excluded from the volatility inputs (ret_excl)
  * volume features use traded VALUE (Rial), not share volume: share volume is not adjusted for capital increases and the
    adjusted volume of step 4 was estimated with future data
  * HV = std of daily log returns over 20 active rows, annualised with sqrt(252); HV rank = (HV - min) / (max - min) * 100
    over the trailing 250 (and 120) active rows, full window required (no partial ranks)
"""
from __future__ import annotations

import json

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
from common import load_daily, out_dir, save_table  # shared helpers of the EDA code

RSI_N = 14
ATR_N = 14
HV_N = 20
HV_MIN_OBS = 15
RANK_WINDOWS = (250, 120)
ANNUAL = 252

# name -> (group, lookback in active rows, definition)
FEATURES = {
    "ret_1": ("return", 1, "log(close_t / close_{t-1 active day})"),
    "ret_5": ("return", 5, "log(close_t / close 5 active days earlier)"),
    "ret_20": ("return", 20, "log(close_t / close 20 active days earlier)"),
    "rsi_14": ("technical", 15, "Wilder RSI, 14 active days, 0..100"),
    "close_sma20_ratio": ("technical", 20, "close / SMA20 - 1"),
    "close_sma60_ratio": ("technical", 60, "close / SMA60 - 1"),
    "ma_ratio_5_20": ("technical", 20, "SMA5 / SMA20 - 1"),
    "ma_ratio_20_60": ("technical", 60, "SMA20 / SMA60 - 1"),
    "ma_cross_5_20": ("technical", 21, "+1 SMA5 crossed above SMA20 today, -1 crossed below, 0 no cross"),
    "ma_cross_20_60": ("technical", 61, "+1 SMA20 crossed above SMA60 today, -1 crossed below, 0 no cross"),
    "atr_14_pct": ("technical", 15, "Wilder ATR(14) of adjusted high/low/close divided by close"),
    "value_ratio_20": ("volume", 20, "traded value today / mean traded value of the last 20 active days (incl. today)"),
    "tcount_ratio_20": ("volume", 20, "trade count today / mean trade count of the last 20 active days (incl. today)"),
    "ind_buy_share": ("flow", 1, "retail share of buy value (retail + institutional)"),
    "ind_net_ratio": ("flow", 1, "(retail buy value - retail sell value) / traded value"),
    "ind_net_ratio_5": ("flow", 5, "mean of ind_net_ratio over the last 5 active days (min 3)"),
    "ind_power_log": ("flow", 1, "log of the retail buyer power ratio (per-capita buy / per-capita sell)"),
    "hv_20": ("volatility", 20, "annualised std of daily log returns, 20 active days (min 15)"),
    "hv_rank_250": ("volatility", 270, "HV20 position in its trailing 250-active-day min..max range, 0..100"),
    "hv_rank_120": ("volatility", 140, "HV20 position in its trailing 120-active-day min..max range, 0..100"),
}
FLAGS = ["is_active", "is_reopen", "halt_len", "first_trade", "after_gap", "adj_event"]
KEYS = ["symbol", "ins_code", "date"]
PANEL_COLS = KEYS + ["is_active", "is_reopen", "halt_len", "first_trade", "after_gap", "adj_event", "close", "high", "low",
                     "value", "trade_count", "ind_buy_share", "ind_net_ratio", "ind_power_ratio"]


# ------------------------------------------------------------------ panel
def build_panel() -> pd.DataFrame:
    """Merge step 6a (calendar, flags, flow ratios) with the corrected adjusted prices of step 4 and raw high/low."""
    root = out_dir("features").parent
    f6 = pd.read_csv(root / "step6a_returns_features" / "features_daily.csv", encoding="utf-8-sig", parse_dates=["date"])
    adj = pd.read_csv(root / "step4_adjustments" / "adjusted_series.csv", encoding="utf-8-sig", parse_dates=["date"])
    raw = load_daily()[["symbol", "ins_code", "date", "high", "low", "trade_count"]]
    p = f6.merge(adj[["symbol", "date", "adj_factor_corrected", "adj_final_corrected", "value"]], on=["symbol", "date"],
                 how="left", validate="1:1").merge(raw, on=["symbol", "date"], how="left", validate="1:1")
    p["is_active"] = p["active"].astype(bool)
    p["close"] = p["adj_final_corrected"]
    p["high"] = p["high"] * p["adj_factor_corrected"]
    p["low"] = p["low"] * p["adj_factor_corrected"]
    for c in ("is_reopen", "first_trade", "after_gap", "adj_event"):
        p[c] = p[c].astype(bool)
    p["halt_len"] = p["halt_len"].fillna(0)
    inactive = ~p["is_active"]
    p.loc[inactive, ["close", "high", "low", "value", "trade_count"]] = np.nan
    p["ind_power_ratio"] = p["ind_power_ratio"].where(p["is_active"])
    return p[PANEL_COLS].sort_values(["symbol", "date"]).reset_index(drop=True)


# ------------------------------------------------------------------ features
def _wilder(s: pd.Series, n: int) -> pd.Series:
    return s.ewm(alpha=1.0 / n, adjust=False, min_periods=n).mean()


def _one_symbol(g: pd.DataFrame) -> pd.DataFrame:
    """Features for ONE symbol, computed on its ACTIVE rows only (g is sorted by date, all rows active)."""
    c = g["close"]
    out = pd.DataFrame(index=g.index)
    lr = np.log(c).diff()                                                   # log return vs previous active row
    out["ret_1"] = lr
    out["ret_5"] = np.log(c / c.shift(5))
    out["ret_20"] = np.log(c / c.shift(20))

    delta = c.diff()
    avg_gain, avg_loss = _wilder(delta.clip(lower=0), RSI_N), _wilder((-delta).clip(lower=0), RSI_N)
    rsi = 100 - 100 / (1 + avg_gain / avg_loss)
    out["rsi_14"] = rsi.where(avg_loss != 0, 100.0).where(avg_gain.notna())

    sma5, sma20, sma60 = (c.rolling(n, min_periods=n).mean() for n in (5, 20, 60))
    out["close_sma20_ratio"] = c / sma20 - 1
    out["close_sma60_ratio"] = c / sma60 - 1
    out["ma_ratio_5_20"] = sma5 / sma20 - 1
    out["ma_ratio_20_60"] = sma20 / sma60 - 1
    for name, fast, slow in (("ma_cross_5_20", sma5, sma20), ("ma_cross_20_60", sma20, sma60)):
        above = (fast > slow).astype(float).where(fast.notna() & slow.notna())
        out[name] = above.diff().where(above.notna() & above.shift().notna()).fillna(np.nan)   # +1 up-cross, -1 down-cross, 0 none

    prev_c = c.shift()
    tr = pd.concat([g["high"] - g["low"], (g["high"] - prev_c).abs(), (g["low"] - prev_c).abs()], axis=1).max(axis=1, skipna=False)
    out["atr_14_pct"] = _wilder(tr, ATR_N) / c

    out["value_ratio_20"] = g["value"] / g["value"].rolling(20, min_periods=10).mean()
    out["tcount_ratio_20"] = g["trade_count"] / g["trade_count"].rolling(20, min_periods=10).mean()

    out["ind_buy_share"] = g["ind_buy_share"]
    out["ind_net_ratio"] = g["ind_net_ratio"]
    out["ind_net_ratio_5"] = g["ind_net_ratio"].rolling(5, min_periods=3).mean()
    out["ind_power_log"] = np.log(g["ind_power_ratio"].where(g["ind_power_ratio"] > 0))

    excl = (g["halt_len"] > 10) | g["after_gap"] | (g["is_reopen"] & g["adj_event"])
    hv = lr.where(~excl).rolling(HV_N, min_periods=HV_MIN_OBS).std() * np.sqrt(ANNUAL)
    out["hv_20"] = hv
    for w in RANK_WINDOWS:
        lo, hi = hv.rolling(w, min_periods=w).min(), hv.rolling(w, min_periods=w).max()
        out[f"hv_rank_{w}"] = ((hv - lo) / (hi - lo) * 100).where(hi > lo)
    return out


def compute_features(panel: pd.DataFrame) -> pd.DataFrame:
    """Pure function: panel (one row per symbol-trading day) -> features table. Row t depends only on rows <= t."""
    panel = panel.sort_values(["symbol", "date"]).reset_index(drop=True)
    parts = []
    for _, g in panel.groupby("symbol", sort=False):
        a = g[g["is_active"]]
        parts.append(_one_symbol(a))
    feat = pd.concat(parts).reindex(panel.index) if parts else pd.DataFrame(index=panel.index)
    res = panel[KEYS + FLAGS].copy()
    for name in FEATURES:
        res[name] = feat[name] if name in feat else np.nan
    res["halt_len"] = res["halt_len"].astype(int)
    return res


# ------------------------------------------------------------------ outputs
def write_sql(features: pd.DataFrame, url: str, table: str = "features") -> int:
    """Load the table into any SQLAlchemy database, e.g. sqlite:///features.db or postgresql+psycopg2://user:pw@host/db."""
    from sqlalchemy import create_engine

    eng = create_engine(url)
    features.to_sql(table, eng, if_exists="replace", index=False, chunksize=5000)
    with eng.begin() as con:
        from sqlalchemy import text
        con.execute(text(f"CREATE UNIQUE INDEX IF NOT EXISTS ux_{table}_symbol_date ON {table} (symbol, date)"))
    return len(features)


def main(db_url: str | None = None) -> None:
    OUT = out_dir("features")
    panel = build_panel()
    feats = compute_features(panel)
    assert not feats.duplicated(["symbol", "date"]).any() and len(feats) == len(panel)
    feats["date"] = pd.to_datetime(feats["date"])
    feats.to_parquet(OUT / "features.parquet", index=False)
    save_table(feats.assign(**{n: feats[n].round(8) for n in FEATURES}), OUT / "features.csv")

    dic = pd.DataFrame([{"feature": k, "group": v[0], "lookback_active_days": v[1], "definition": v[2]} for k, v in FEATURES.items()])
    save_table(dic, OUT / "feature_dictionary.csv")

    names = list(FEATURES)
    act = feats[feats["is_active"]]
    cov = pd.DataFrame({
        "feature": names,
        "non_null_active_rows": [int(act[n].notna().sum()) for n in names],
        "pct_non_null_active": [round(100 * act[n].notna().mean(), 1) for n in names],
        "min": [act[n].min() for n in names], "median": [act[n].median() for n in names], "max": [act[n].max() for n in names],
    })
    save_table(cov.round(5), OUT / "feature_coverage.csv")
    by_sym = act.groupby("symbol")[names].apply(lambda d: d.notna().mean() * 100).round(1)
    save_table(by_sym.reset_index(), OUT / "feature_coverage_by_symbol.csv")

    inactive_nonnull = int(feats.loc[~feats["is_active"], names].notna().sum().sum())
    inf_cnt = int(np.isinf(feats[names].to_numpy(dtype=float)).sum())
    rsi_ok = bool(act["rsi_14"].dropna().between(0, 100).all())
    rk_ok = all(bool(act[f"hv_rank_{w}"].dropna().between(0, 100).all()) for w in RANK_WINDOWS)
    checks = [
        {"id": "S7.01", "check": "one row per symbol-day, key unique", "status": "PASS", "detail": f"{len(feats)} rows, {feats['symbol'].nunique()} symbols"},
        {"id": "S7.02", "check": "halted rows have no feature values", "status": "PASS" if inactive_nonnull == 0 else "FAIL", "detail": f"{inactive_nonnull} non-null cells on halted rows"},
        {"id": "S7.03", "check": "no inf values", "status": "PASS" if inf_cnt == 0 else "FAIL", "detail": f"{inf_cnt}"},
        {"id": "S7.04", "check": "RSI in [0,100], HV rank in [0,100]", "status": "PASS" if rsi_ok and rk_ok else "FAIL", "detail": f"rsi_ok={rsi_ok}, rank_ok={rk_ok}"},
        {"id": "S7.05", "check": "warm-up NaN is expected", "status": "INFO",
         "detail": "; ".join(f"{r.feature}={r.pct_non_null_active}%" for r in cov.itertuples() if r.pct_non_null_active < 95)},
    ]
    save_table(pd.DataFrame(checks), OUT / "checks_summary.csv")

    rows_sql = write_sql(feats, db_url) if db_url else None
    (OUT / "summary.json").write_text(json.dumps({"rows": len(feats), "active_rows": int(feats["is_active"].sum()),
                                                  "features": len(names), "sql_rows_written": rows_sql}, indent=2))

    # example chart: one liquid symbol
    sym = "اهرم"
    d = feats[(feats["symbol"] == sym)].merge(panel[["symbol", "date", "close"]], on=["symbol", "date"])
    d = d[d["is_active"]]
    fig, ax = plt.subplots(4, 1, figsize=(11, 9), sharex=True)
    ax[0].plot(d["date"], d["close"], lw=1, color="#1f4e79")
    ax[0].plot(d["date"], d["close"] / (1 + d["close_sma20_ratio"]), lw=0.8, color="#e76f51", label="SMA20")
    ax[0].plot(d["date"], d["close"] / (1 + d["close_sma60_ratio"]), lw=0.8, color="#2a9d8f", label="SMA60")
    ax[0].set_title("adjusted close with SMA20 / SMA60 (example symbol)")
    ax[0].legend(fontsize=8)
    ax[1].plot(d["date"], d["rsi_14"], lw=0.8, color="#6a3d9a")
    ax[1].axhline(70, color="grey", ls="--", lw=0.6)
    ax[1].axhline(30, color="grey", ls="--", lw=0.6)
    ax[1].set_title("RSI(14)")
    ax[2].plot(d["date"], d["hv_20"], lw=0.9, color="#b2182b")
    ax[2].set_title("HV20 (annualised)")
    ax[3].plot(d["date"], d["hv_rank_250"], lw=0.9, label="rank 250", color="#1f4e79")
    ax[3].plot(d["date"], d["hv_rank_120"], lw=0.9, label="rank 120", color="#e9c46a")
    ax[3].set_title("HV rank")
    ax[3].legend(fontsize=8)
    fig.tight_layout()
    fig.savefig(OUT / "example_symbol_features.png", dpi=120)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(9, 6))
    cov.set_index("feature")["pct_non_null_active"].sort_values().plot.barh(ax=ax, color="#2a9d8f")
    ax.set_xlabel("% of active rows with a value")
    ax.set_title("Feature coverage (warm-up rows are NaN)")
    fig.tight_layout()
    fig.savefig(OUT / "feature_coverage.png", dpi=120)
    plt.close(fig)

    pd.set_option("display.width", 220, "display.max_colwidth", 160)
    print(cov.to_string(index=False))
    print(pd.DataFrame(checks).to_string(index=False))


if __name__ == "__main__":
    main()
