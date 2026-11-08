"""Step 6c - time-series behaviour of returns and next-day predictive content of the features.

Input : outputs/step6a_returns_features/features_daily.csv
Outputs (outputs/step6c_dynamics/):
  autocorr_by_symbol.csv     ACF of ret / |ret| / ret^2 (lags 1-5), Ljung-Box, ARCH-LM per symbol
  weekday_effects.csv        mean return and mean |ret| by weekday (pooled, symbol-demeaned) with t-stats
  period_volatility.csv      daily std by symbol and price-band period
  predictive_ic.csv          per feature: pooled rank-IC with next-day return / |return| / range, t-stat over symbols
  vol_persistence.csv        |ret| next day by vol20 and value_rel20 quintile
  checks_summary.csv, summary.json, acf_pooled.png, predictive_ic.png
Only consecutive ACTIVE records (no halt, no reopen, no gap between t and t+1) are paired.
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
from scipy import stats

from common import out_dir, save_table


def main() -> None:

    OUT = out_dir("step6c_dynamics")
    IN = OUT.parent / "step6a_returns_features" / "features_daily.csv"
    checks: list[dict] = []


    def add(i, name, status, n, detail):
        checks.append({"id": i, "check": name, "status": status, "n_issues": n, "detail": detail})


    F = pd.read_csv(IN, encoding="utf-8-sig", parse_dates=["date"]).sort_values(["symbol", "date"]).reset_index(drop=True)
    F["ret_last_clean"] = F["ret_last"].where(F["ret_clean"].notna())   # same rows as ret_clean, last-trade basis
    F["abs_ret"] = F["ret_clean"].abs()
    F["ret2"] = F["ret_clean"] ** 2
    F["wd"] = F["date"].dt.dayofweek.map({5: "Sat", 6: "Sun", 0: "Mon", 1: "Tue", 2: "Wed", 3: "Thu", 4: "Fri"})

    # ---------------- consecutive pairs: row t and t+1 both active, t+1 not a reopen / gap
    g = F.groupby("symbol")
    for c in ["ret_clean", "abs_ret", "hl_range", "ind_net_ratio", "ret_last_clean"]:
        F[f"next_{c}"] = g[c].shift(-1)
    nxt_ok = g["active"].shift(-1).fillna(False).astype(bool) & ~g["is_reopen"].shift(-1).fillna(True).astype(bool) \
        & ~g["after_gap"].shift(-1).fillna(True).astype(bool)
    F["pair_ok"] = F["active"] & nxt_ok
    add("S6C.01", "consecutive active pairs available for lead-lag tests", "INFO", None,
        f"{int(F['pair_ok'].sum())} pairs of {int(F['active'].sum())} active rows")


    def acf(x: np.ndarray, k: int) -> float:
        a, b = x[:-k], x[k:]
        m = np.isfinite(a) & np.isfinite(b)
        return float(np.corrcoef(a[m], b[m])[0, 1]) if m.sum() > 30 else np.nan


    def ljung_box(x: np.ndarray, h: int = 10):
        x = x[np.isfinite(x)]
        n = len(x)
        if n < 60:
            return np.nan, np.nan
        xc = x - x.mean()
        den = (xc ** 2).sum()
        r = np.array([(xc[:-k] * xc[k:]).sum() / den for k in range(1, h + 1)])
        q = n * (n + 2) * (r ** 2 / (n - np.arange(1, h + 1))).sum()
        return float(q), float(1 - stats.chi2.cdf(q, h))


    def arch_lm(x: np.ndarray, p: int = 5):
        x = x[np.isfinite(x)]
        e2 = (x - x.mean()) ** 2
        if len(e2) < 80:
            return np.nan, np.nan
        Y = e2[p:]
        X = np.column_stack([np.ones(len(Y))] + [e2[p - k:-k] for k in range(1, p + 1)])
        beta = np.linalg.lstsq(X, Y, rcond=None)[0]
        r2 = 1 - ((Y - X @ beta) ** 2).sum() / ((Y - Y.mean()) ** 2).sum()
        lm = len(Y) * r2
        return float(lm), float(1 - stats.chi2.cdf(lm, p))


    rows = []
    for sym, d in F.groupby("symbol", sort=True):
        # treat the series as one block of consecutive CLEAN returns (NaN breaks pairs inside acf)
        r = d["ret_clean"].values
        ar = d["abs_ret"].values
        q, pq = ljung_box(r)
        qa, pqa = ljung_box(ar)
        lm, plm = arch_lm(r)
        rl = d["ret_last_clean"].values
        rec = dict(acf_last_1=acf(rl, 1), acf_last_2=acf(rl, 2), symbol=sym, n=int(np.isfinite(r).sum()), mean=np.nanmean(r), std=np.nanstd(r), skew=stats.skew(r[np.isfinite(r)]),
                   kurt=stats.kurtosis(r[np.isfinite(r)]), lb10_q=q, lb10_p=pq, lb10_abs_q=qa, lb10_abs_p=pqa, arch_lm5=lm, arch_p=plm)
        for k in range(1, 6):
            rec[f"acf_ret_{k}"] = acf(r, k)
            rec[f"acf_abs_{k}"] = acf(ar, k)
        rows.append(rec)
    A = pd.DataFrame(rows)
    save_table(A, OUT / "autocorr_by_symbol.csv")
    sig_ret = int((A["lb10_p"] < 0.05).sum())
    sig_abs = int((A["lb10_abs_p"] < 0.05).sum())
    sig_arch = int((A["arch_p"] < 0.05).sum())
    add("S6C.02", "serial correlation of returns (Ljung-Box lag 10, 5%)", "INFO", sig_ret,
        f"{sig_ret}/{len(A)} symbols reject no-autocorrelation; median acf1 = {A['acf_ret_1'].median():.3f}; "
        f"{int((A['acf_ret_1'].abs() > 0.1).sum())} symbols have |acf1|>0.1")
    add("S6C.03", "volatility clustering (Ljung-Box on |ret| and ARCH-LM(5), 5%)", "INFO", sig_abs,
        f"{sig_abs}/{len(A)} symbols reject for |ret|, {sig_arch}/{len(A)} for ARCH-LM; median acf1 of |ret| = {A['acf_abs_1'].median():.3f}")
    hi_kurt = A[A["kurt"] > 3]
    add("S6C.04", "fat tails: excess kurtosis > 3 on clean returns", "INFO", len(hi_kurt),
        f"{len(hi_kurt)}/{len(A)} symbols; median excess kurtosis {A['kurt'].median():.2f}; range {A['kurt'].min():.2f}..{A['kurt'].max():.2f}")

    d1, d2 = A["acf_ret_1"].median(), A["acf_last_1"].median()
    add("S6C.11", "lag-1 autocorrelation: final_price (VWAP-like) vs last_price basis", "WARN", None,
        f"median acf1 {d1:.3f} on final_price returns vs {d2:.3f} on last_price returns; "
        f"{int((A['acf_last_1'] < A['acf_ret_1']).sum())}/{len(A)} symbols lower on last_price")

    # ---------------- weekday effects (symbol-demeaned)
    W = F.dropna(subset=["ret_clean"]).copy()
    W["ret_dm"] = W["ret_clean"] - W.groupby("symbol")["ret_clean"].transform("mean")
    W["abs_dm"] = W["abs_ret"] / W.groupby("symbol")["abs_ret"].transform("mean")
    wrows = []
    for wd, d in W.groupby("wd"):
        t1 = stats.ttest_1samp(d["ret_dm"], 0)
        t2 = stats.ttest_1samp(d["abs_dm"], 1)
        wrows.append(dict(weekday=wd, n=len(d), mean_ret_dm=d["ret_dm"].mean(), t_ret=float(t1.statistic), rel_abs_ret=d["abs_dm"].mean(), t_abs=float(t2.statistic)))
    WD = pd.DataFrame(wrows).set_index("weekday").loc[["Sat", "Sun", "Mon", "Tue", "Wed"]].reset_index()
    save_table(WD, OUT / "weekday_effects.csv")
    add("S6C.05", "weekday effects (Sat..Wed), symbol-demeaned", "INFO", int((WD["t_ret"].abs() > 2).sum()),
        "; ".join(f"{r.weekday}: ret {r.mean_ret_dm:+.4f} (t={r.t_ret:.1f}), |ret| x{r.rel_abs_ret:.2f} (t={r.t_abs:.1f})" for r in WD.itertuples()))

    # ---------------- period volatility
    F["period"] = np.where(F["date"] < "2024-09-01", "P1 to 2024-08", np.where(F["date"] < "2025-07-01", "P2 2024-09..2025-06", "P3 from 2025-07"))
    PV = F.dropna(subset=["ret_clean"]).pivot_table(index="symbol", columns="period", values="ret_clean", aggfunc="std").reset_index()
    save_table(PV, OUT / "period_volatility.csv")
    med = PV.drop(columns="symbol").median()
    add("S6C.06", "daily volatility by price-band period (median across symbols)", "INFO", None, "; ".join(f"{k}: {v:.4f}" for k, v in med.items()))

    # ---------------- predictive content of features (pooled rank-IC; t-stat from per-symbol ICs)
    P = F[F["pair_ok"]].copy()
    feats = ["ret_clean", "abs_ret", "vol20", "vol60", "value_rel20", "tcount_rel20", "advol_rel20", "ind_net_ratio", "ind_buy_share",
             "log_ind_power", "hl_range", "range_rel20"]
    targets = {"next_ret_last": "next_ret_last_clean", "next_ret": "next_ret_clean", "next_abs_ret": "next_abs_ret", "next_range": "next_hl_range"}
    ic_rows = []
    for f in feats:
        for tname, tcol in targets.items():
            ics = []
            for sym, d in P.groupby("symbol"):
                x = d[[f, tcol]].dropna()
                if len(x) >= 60:
                    ics.append(stats.spearmanr(x[f], x[tcol])[0])
            ics = np.array(ics)
            pooled = P[[f, tcol]].dropna()
            pic = stats.spearmanr(pooled[f], pooled[tcol])[0] if len(pooled) > 100 else np.nan
            ic_rows.append(dict(feature=f, target=tname, n=len(pooled), pooled_ic=pic, mean_symbol_ic=ics.mean(),
                                t_over_symbols=ics.mean() / (ics.std(ddof=1) / np.sqrt(len(ics))), share_symbols_positive=(ics > 0).mean(), n_symbols=len(ics)))
    IC = pd.DataFrame(ic_rows)
    save_table(IC, OUT / "predictive_ic.csv")
    top = IC[IC["target"] == "next_ret"].reindex(IC[IC["target"] == "next_ret"]["mean_symbol_ic"].abs().sort_values(ascending=False).index).head(3)
    tl = IC[IC["target"] == "next_ret_last"].reindex(IC[IC["target"] == "next_ret_last"]["mean_symbol_ic"].abs().sort_values(ascending=False).index).head(4)
    add("S6C.12", "next-day return on LAST-price basis (no VWAP artefact): strongest features", "INFO", None,
        "; ".join(f"{r.feature}: IC {r.mean_symbol_ic:+.3f} (t={r.t_over_symbols:.1f})" for r in tl.itertuples()))
    add("S6C.07", "next-day RETURN (final_price basis): strongest feature rank-IC (mean over symbols)", "INFO", None,
        "; ".join(f"{r.feature}: IC {r.mean_symbol_ic:+.3f} (t={r.t_over_symbols:.1f})" for r in top.itertuples()))
    top2 = IC[IC["target"] == "next_abs_ret"].reindex(IC[IC["target"] == "next_abs_ret"]["mean_symbol_ic"].abs().sort_values(ascending=False).index).head(4)
    add("S6C.08", "next-day |RETURN| (volatility): strongest features", "INFO", None,
        "; ".join(f"{r.feature}: IC {r.mean_symbol_ic:+.3f} (t={r.t_over_symbols:.1f})" for r in top2.itertuples()))

    # quintile table: next |ret| by vol20 and value_rel20
    qrows = []
    for f in ["vol20", "value_rel20"]:
        d = P.dropna(subset=[f, "next_abs_ret"]).copy()
        d["q"] = d.groupby("symbol")[f].transform(lambda s: pd.qcut(s.rank(method="first"), 5, labels=False))
        for q, x in d.groupby("q"):
            qrows.append(dict(feature=f, quintile=int(q) + 1, n=len(x), mean_next_abs_ret=x["next_abs_ret"].mean(), mean_next_ret=x["next_ret_clean"].mean()))
    Q = pd.DataFrame(qrows)
    save_table(Q, OUT / "vol_persistence.csv")
    v = Q[Q["feature"] == "vol20"]
    add("S6C.09", "next-day |ret| by within-symbol vol20 quintile (low -> high)", "INFO", None,
        " -> ".join(f"{x:.4f}" for x in v["mean_next_abs_ret"]))

    # sanity: leakage - the target must come from t+1 only
    chk = F.groupby("symbol").apply(lambda d: (d["next_ret_clean"].iloc[:-1].values == d["ret_clean"].iloc[1:].values) | d["next_ret_clean"].iloc[:-1].isna().values, include_groups=False)
    ok_shift = all(bool(np.all(v)) for v in chk)
    add("S6C.10", "target alignment: next_ret equals the following row's return", "PASS" if ok_shift else "FAIL", 0 if ok_shift else 1, "")

    pd.DataFrame(checks).to_csv(OUT / "checks_summary.csv", index=False, encoding="utf-8-sig")
    json.dump(dict(symbols=len(A), lb_ret=sig_ret, lb_abs=sig_abs, arch=sig_arch), open(OUT / "summary.json", "w"))

    # charts
    fig, ax = plt.subplots(1, 2, figsize=(10, 4))
    ks = range(1, 6)
    ax[0].bar(ks, [A[f"acf_ret_{k}"].median() for k in ks], color="#2166ac")
    ax[0].set_title("Median ACF of returns across symbols")
    ax[1].bar(ks, [A[f"acf_abs_{k}"].median() for k in ks], color="#b2182b")
    ax[1].set_title("Median ACF of |returns| (volatility clustering)")
    for a in ax:
        a.set_xlabel("lag (trading days)")
    fig.tight_layout()
    fig.savefig(OUT / "acf_pooled.png", dpi=150)
    plt.close(fig)
    fig, ax = plt.subplots(figsize=(9, 5))
    piv = IC.pivot(index="feature", columns="target", values="mean_symbol_ic")
    piv.plot.barh(ax=ax)
    ax.axvline(0, color="k", lw=0.6)
    ax.set_xlabel("mean rank-IC over symbols")
    ax.set_title("Predictive content of features for the next trading day")
    fig.tight_layout()
    fig.savefig(OUT / "predictive_ic.png", dpi=150)
    plt.close(fig)
    print(pd.DataFrame(checks)[["id", "status", "detail"]].to_string(max_colwidth=260))


if __name__ == "__main__":
    main()
