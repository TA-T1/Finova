"""Step 6b - return distribution, price-limit days, drawdowns, stationarity and volatility regimes.

Inputs : step 6a features_daily, step 4 adjusted_series, raw daily file.
Outputs (outputs/step6b_distribution/):
  distribution_by_symbol.csv    mean/std/skew/excess kurtosis/quantiles and Jarque-Bera test of clean returns
  limit_days_by_symbol.csv      days closing at the daily price limit (periods with a fixed band only)
  drawdown_by_symbol.csv        max drawdown, peak/trough/recovery dates, longest underwater spell (adjusted price)
  stationarity_by_symbol.csv    ADF test on log adjusted price and on clean returns
  acf_pacf_by_symbol.csv        ACF and PACF of clean returns, lags 1-10
  volatility_by_quarter.csv     per-symbol daily std by quarter: median and quartiles across symbols
  return_qq_pooled.png, acf_pacf_median.png, volatility_by_quarter.png, drawdown_by_symbol.png
  checks_summary.csv, summary.json
Price bands (step 2): up to 2024-08 wide/mixed, 2024-09..2025-06 +-3%, from 2025-07 +-4%. Limit days are only counted
in the two fixed-band periods, with tolerance LIMIT_TOL on last_price/prev_final - 1.
"""
from __future__ import annotations

import json

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from scipy import stats
from statsmodels.tsa.stattools import acf as sm_acf
from statsmodels.tsa.stattools import adfuller, pacf as sm_pacf

from common import fa_label, load_daily, out_dir, save_table

LIMIT_TOL = 0.001
MIN_N = 60
BANDS = [("P2 2024-09..2025-06", "2024-09-01", "2025-07-01", 0.03), ("P3 from 2025-07", "2025-07-01", "2026-12-31", 0.04)]


def main() -> None:
    OUT = out_dir("step6b_distribution")
    S4, S6A = OUT.parent / "step4_adjustments", OUT.parent / "step6a_returns_features"
    checks: list[dict] = []

    def add(i, name, status, n, detail):
        checks.append({"id": i, "check": name, "status": status, "n_issues": n, "detail": detail})

    F = pd.read_csv(S6A / "features_daily.csv", encoding="utf-8-sig", parse_dates=["date"]).sort_values(["symbol", "date"])
    adj = pd.read_csv(S4 / "adjusted_series.csv", encoding="utf-8-sig", parse_dates=["date"])
    raw = load_daily()[["symbol", "date", "last_price", "prev_final", "is_halted"]]
    syms = sorted(F["symbol"].unique())

    # ---------------- distribution + Jarque-Bera
    rows = []
    for sym, g in F.groupby("symbol"):
        r = g["ret_clean"].dropna().to_numpy()
        if len(r) < 30:
            continue
        jb = stats.jarque_bera(r)
        q = np.quantile(r, [.01, .05, .5, .95, .99])
        rows.append({"symbol": sym, "n": len(r), "mean": r.mean(), "std": r.std(ddof=1), "skew": stats.skew(r),
                     "excess_kurtosis": stats.kurtosis(r), "min": r.min(), "p01": q[0], "p05": q[1], "median": q[2],
                     "p95": q[3], "p99": q[4], "max": r.max(), "jb_stat": jb.statistic, "jb_p": jb.pvalue,
                     "normal_rejected_5pct": bool(jb.pvalue < 0.05)})
    D = pd.DataFrame(rows)
    save_table(D.round(6), OUT / "distribution_by_symbol.csv")
    add("S6B.01", "Jarque-Bera normality test on clean returns", "INFO", int(D["normal_rejected_5pct"].sum()),
        f"normality rejected for {int(D['normal_rejected_5pct'].sum())}/{len(D)} symbols (n>=30); median skew {D['skew'].median():+.2f}, "
        f"median excess kurtosis {D['excess_kurtosis'].median():.2f}, max {D['excess_kurtosis'].max():.1f} ({D.loc[D['excess_kurtosis'].idxmax(), 'symbol']})")
    pos = int((D["skew"] > 0).sum())
    add("S6B.02", "skewness sign across symbols", "INFO", None, f"{pos}/{len(D)} symbols right-skewed, {len(D) - pos} left-skewed")

    # ---------------- limit days (last price vs exchange reference, fixed-band periods)
    X = raw[~raw["is_halted"]].merge(F[["symbol", "date", "limit_locked", "is_reopen"]], on=["symbol", "date"], how="left")
    X["r"] = X["last_price"] / X["prev_final"] - 1
    lrows = []
    for sym, g in X.groupby("symbol"):
        rec = {"symbol": sym}
        for name, a, b, band in BANDS:
            x = g[(g["date"] >= a) & (g["date"] < b) & (g["r"].abs() < 2)]
            up = (x["r"] >= band - LIMIT_TOL) & (x["r"] <= band + LIMIT_TOL)
            dn = (x["r"] <= -band + LIMIT_TOL) & (x["r"] >= -band - LIMIT_TOL)
            k = name.split()[0]
            rec.update({f"{k}_days": len(x), f"{k}_limit_up": int(up.sum()), f"{k}_limit_down": int(dn.sum()),
                        f"{k}_limit_share": (up.sum() + dn.sum()) / len(x) if len(x) else np.nan,
                        f"{k}_locked_at_limit": int(((up | dn) & x["limit_locked"].fillna(False).astype(bool)).sum())})
        lrows.append(rec)
    L = pd.DataFrame(lrows)
    save_table(L.round(5), OUT / "limit_days_by_symbol.csv")
    tot = {k: int(L[f"{k}_days"].sum()) for k in ("P2", "P3")}
    share = {k: (L[f"{k}_limit_up"].sum() + L[f"{k}_limit_down"].sum()) / tot[k] for k in tot}
    up_all = sum(L[f"{k}_limit_up"].sum() for k in tot)
    dn_all = sum(L[f"{k}_limit_down"].sum() for k in tot)
    add("S6B.03", "days closing at the daily price limit (last_price vs reference)", "INFO", int(up_all + dn_all),
        f"P2 (+-3%): {share['P2']:.1%} of {tot['P2']} symbol-days; P3 (+-4%): {share['P3']:.1%} of {tot['P3']}; limit-up {int(up_all)} vs limit-down {int(dn_all)}; "
        f"highest share: {L.assign(s=(L['P2_limit_up'] + L['P2_limit_down'] + L['P3_limit_up'] + L['P3_limit_down']) / (L['P2_days'] + L['P3_days'])).nlargest(3, 's')['symbol'].tolist()}")

    # ---------------- drawdown on corrected adjusted price
    px = adj[~adj["is_halted"] & adj["adj_final_corrected"].notna() & ~adj["price_anomaly"]].sort_values(["symbol", "date"])
    drows = []
    curves = {}
    for sym, g in px.groupby("symbol"):
        p = g.set_index("date")["adj_final_corrected"]
        dd = p / p.cummax() - 1
        trough = dd.idxmin()
        peak_date = p[:trough].idxmax()
        after = p[trough:]
        rec_idx = after[after >= p[peak_date]].index
        under = (dd < 0).astype(int)
        runs = under.groupby((under != under.shift()).cumsum()).sum().where(lambda s: s > 0)
        drows.append({"symbol": sym, "n_days": len(p), "max_drawdown": dd.min(), "peak_date": peak_date.date(),
                      "trough_date": trough.date(), "recovery_date": rec_idx[0].date() if len(rec_idx) else None,
                      "current_drawdown": dd.iloc[-1], "longest_underwater_days": int(runs.max()) if runs.notna().any() else 0})
        curves[sym] = dd
    DD = pd.DataFrame(drows)
    save_table(DD.round(5), OUT / "drawdown_by_symbol.csv")
    worst = DD.nsmallest(3, "max_drawdown")
    add("S6B.04", "maximum drawdown of the adjusted price", "INFO", None,
        f"median {DD['max_drawdown'].median():.1%}; worst: " + ", ".join(f"{r.symbol} {r.max_drawdown:.1%}" for r in worst.itertuples())
        + f"; {int(DD['recovery_date'].isna().sum())}/{len(DD)} symbols never recovered their peak")

    # ---------------- stationarity (ADF) and ACF/PACF
    srows, arows = [], []
    for sym in syms:
        r = F.loc[F["symbol"] == sym, "ret_clean"].dropna().to_numpy()
        lp = np.log(px.loc[px["symbol"] == sym, "adj_final_corrected"].to_numpy())
        rec = {"symbol": sym, "n_price": len(lp), "n_ret": len(r)}
        if len(lp) >= MIN_N:
            a = adfuller(lp, autolag="AIC")
            rec.update({"adf_level_stat": a[0], "adf_level_p": a[1], "adf_level_lags": a[2]})
        if len(r) >= MIN_N:
            a = adfuller(r, autolag="AIC")
            rec.update({"adf_ret_stat": a[0], "adf_ret_p": a[1], "adf_ret_lags": a[2]})
            ac, pc = sm_acf(r, nlags=10, fft=False), sm_pacf(r, nlags=10, method="ols")
            arows.append({"symbol": sym, "n": len(r), **{f"acf_{k}": ac[k] for k in range(1, 11)}, **{f"pacf_{k}": pc[k] for k in range(1, 11)}})
        srows.append(rec)
    S = pd.DataFrame(srows)
    A = pd.DataFrame(arows)
    save_table(S.round(6), OUT / "stationarity_by_symbol.csv")
    save_table(A.round(6), OUT / "acf_pacf_by_symbol.csv")
    nl, nr = S["adf_level_p"].notna().sum(), S["adf_ret_p"].notna().sum()
    add("S6B.05", "ADF test (5%): log adjusted price is non-stationary, returns are stationary", "INFO", None,
        f"price: unit root not rejected for {int((S['adf_level_p'] > 0.05).sum())}/{nl} symbols; returns: stationary for {int((S['adf_ret_p'] < 0.05).sum())}/{nr} symbols")
    add("S6B.06", "partial autocorrelation of clean returns (median over symbols)", "INFO", None,
        "; ".join(f"lag{k}: {A[f'pacf_{k}'].median():+.3f}" for k in range(1, 6)))

    # ---------------- volatility by quarter
    G = F.dropna(subset=["ret_clean"]).copy()
    G["quarter"] = G["date"].dt.to_period("Q").astype(str)
    sd = G.groupby(["quarter", "symbol"])["ret_clean"].agg(["std", "size"]).reset_index()
    sd = sd[sd["size"] >= 20]
    V = sd.groupby("quarter")["std"].agg(median="median", q25=lambda s: s.quantile(.25), q75=lambda s: s.quantile(.75), n_symbols="size").reset_index()
    save_table(V.round(5), OUT / "volatility_by_quarter.csv")
    add("S6B.07", "cross-symbol median of quarterly daily volatility", "INFO", None,
        f"min {V['median'].min():.4f} ({V.loc[V['median'].idxmin(), 'quarter']}), max {V['median'].max():.4f} ({V.loc[V['median'].idxmax(), 'quarter']})")

    # ---------------- charts
    z = G.groupby("symbol")["ret_clean"].transform(lambda s: (s - s.mean()) / s.std(ddof=1)).dropna().to_numpy()
    fig, ax = plt.subplots(1, 2, figsize=(11, 4.5))
    (osm, osr), (slope, icpt, _) = stats.probplot(z, dist="norm")
    ax[0].scatter(osm, osr, s=3, color="#1f4e79")
    ax[0].plot(osm, slope * osm + icpt, color="#b2182b", lw=1)
    ax[0].set_title("Pooled standardized clean returns: normal QQ")
    ax[0].set_xlabel("normal quantiles")
    ax[0].set_ylabel("sample quantiles")
    ax[1].hist(z.clip(-6, 6), bins=120, density=True, color="#9ecae1")
    xs = np.linspace(-6, 6, 400)
    ax[1].plot(xs, stats.norm.pdf(xs), color="#b2182b")
    ax[1].set_yscale("log")
    ax[1].set_ylim(1e-4, 1)
    ax[1].set_title("Histogram vs normal density (log y)")
    fig.tight_layout()
    fig.savefig(OUT / "return_qq_pooled.png", dpi=130)
    plt.close(fig)

    fig, ax = plt.subplots(1, 2, figsize=(11, 4))
    ks = np.arange(1, 11)
    band = 1.96 / np.sqrt(A["n"].median())
    for a, pre, title in ((ax[0], "acf", "Median ACF of clean returns"), (ax[1], "pacf", "Median PACF of clean returns")):
        a.bar(ks, [A[f"{pre}_{k}"].median() for k in ks], color="#2166ac")
        a.axhline(band, color="#b2182b", ls="--", lw=0.8)
        a.axhline(-band, color="#b2182b", ls="--", lw=0.8)
        a.set_title(title)
        a.set_xlabel("lag (trading days)")
    fig.tight_layout()
    fig.savefig(OUT / "acf_pacf_median.png", dpi=130)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(11, 4))
    x = np.arange(len(V))
    ax.fill_between(x, V["q25"], V["q75"], color="#9ecae1", alpha=0.6, label="interquartile range")
    ax.plot(x, V["median"], color="#1f4e79", marker="o", ms=3, label="median")
    ax.set_xticks(x)
    ax.set_xticklabels(V["quarter"], rotation=60, fontsize=7)
    ax.set_ylabel("daily std of clean returns")
    ax.set_title("Volatility by quarter across symbols")
    ax.legend(fontsize=8)
    fig.tight_layout()
    fig.savefig(OUT / "volatility_by_quarter.png", dpi=130)
    plt.close(fig)

    order = DD.sort_values("max_drawdown")
    fig, ax = plt.subplots(figsize=(8, 8))
    ax.barh(range(len(order)), order["max_drawdown"], color="#b2182b")
    ax.barh(range(len(order)), order["current_drawdown"], color="#1f4e79", height=0.4)
    ax.set_yticks(range(len(order)))
    ax.set_yticklabels([fa_label(s) for s in order["symbol"]], fontsize=8)
    ax.set_title("Max drawdown (red) and current drawdown (blue), adjusted price")
    fig.tight_layout()
    fig.savefig(OUT / "drawdown_by_symbol.png", dpi=130)
    plt.close(fig)

    res = pd.DataFrame(checks)
    save_table(res, OUT / "checks_summary.csv")
    (OUT / "summary.json").write_text(json.dumps({"symbols": len(D), "jb_rejected": int(D["normal_rejected_5pct"].sum()),
                                                 **res["status"].value_counts().to_dict()}, indent=2))
    pd.set_option("display.width", 250, "display.max_colwidth", 220)
    print(res[["id", "status", "n_issues", "detail"]].to_string(index=False))


if __name__ == "__main__":
    main()
