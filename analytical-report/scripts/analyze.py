"""Analysis tables and charts for the analytical report (reads outputs/ made by run_backtests.py)."""
from __future__ import annotations

import json
import math
import sys
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from common import FEATURES, FIG, OUT, fa, load_market
from finova_backtest.pricing import bs_price, hv20

try:
    import arabic_reshaper
    from bidi.algorithm import get_display

    def P(s):
        return get_display(arabic_reshaper.reshape(str(s)))
except Exception:  # pragma: no cover
    def P(s):
        return str(s)

CLIP = (-1.0, 5.0)
plt.rcParams.update({"figure.dpi": 130, "axes.spines.top": False, "axes.spines.right": False, "font.size": 9})
BLUE, ORANGE, GREEN, RED, GREY = "#1f4e79", "#e76f51", "#2a9d8f", "#b2182b", "#8d99ae"
TPL_ORDER = ["long-call", "bull-call-spread", "covered-call", "protective-put", "collar", "long-put", "bear-put-spread", "long-straddle", "long-strangle"]


def save(name: str, df: pd.DataFrame):
    df.to_csv(OUT / name, index=False, encoding="utf-8-sig")


def cluster_boot(df: pd.DataFrame, col: str, by: str = "expiry", n: int = 1500, seed: int = 0):
    """95% CI of the mean of `col`, resampling whole expiry dates (trades of one date are correlated)."""
    rng = np.random.default_rng(seed)
    g = df.groupby(by)[col].agg(["sum", "count"])
    s, c = g["sum"].to_numpy(), g["count"].to_numpy()
    idx = rng.integers(0, len(g), (n, len(g)))
    means = s[idx].sum(1) / c[idx].sum(1)
    return np.percentile(means, [2.5, 97.5])


def main() -> None:
    runs = pd.read_csv(OUT / "runs.csv")
    tr = pd.read_csv(OUT / "trades.csv", parse_dates=["expiry", "entry_date", "exit_date"])
    sk = pd.read_csv(OUT / "skipped.csv")
    tr["ror_c"] = tr.return_on_risk.clip(*CLIP)
    tr["win"] = tr.pnl > 0
    tr["year"] = tr.expiry.dt.year
    base = tr[tr.scenario == "base_e30"].copy()

    # ---------------------------------------------------------------- 1. data overview (demo dataset)
    m, snaps = load_market()
    q = pd.DataFrame(snaps)
    for c in ("strike", "close_price", "last_price", "bid_price", "ask_price", "volume", "open_interest", "underlying_price"):
        q[c] = pd.to_numeric(q[c], errors="coerce")
    q["trade_date"] = pd.to_datetime(q["trade_date"])
    q["expiry"] = pd.to_datetime(q["expiry"])
    ov = {
        "option_rows": len(q), "contracts": q.contract_symbol.nunique(), "underlyings_with_options": q.underlying_symbol.nunique(),
        "first_day": str(q.trade_date.min().date()), "last_day": str(q.trade_date.max().date()), "trading_days": q.trade_date.nunique(),
        "rows_with_volume": int((q.volume > 0).sum()), "share_rows_with_volume": round(float((q.volume > 0).mean()), 3),
        "rows_with_bid_ask": int(((q.bid_price > 0) & (q.ask_price > 0)).sum()), "rows_with_open_interest": int((q.open_interest > 0).sum()),
        "bid_ask_days": sorted(q.loc[(q.bid_price > 0) & (q.ask_price > 0), "trade_date"].dt.strftime("%Y-%m-%d").unique().tolist()),
        "earliest_expiry": str(q.expiry.min().date()), "latest_expiry": str(q.expiry.max().date()),
        "expiries_already_passed": int((q.expiry <= q.trade_date.max()).sum()),
        "rows_per_contract_median": float(q.groupby("contract_symbol").size().median()),
        "price_history_days_3y": int(len(m.trading_days("خودرو"))),
    }
    per_u = (q.groupby("underlying_symbol").agg(contracts=("contract_symbol", "nunique"), rows=("trade_date", "size"), traded_value_rial=("value", lambda x: pd.to_numeric(x).sum()),
                                                 traded_rows=("volume", lambda x: int((x > 0).sum()))).reset_index().sort_values("traded_value_rial", ascending=False))
    per_u["price_history_days"] = per_u.underlying_symbol.map(lambda u: len(m.trading_days(u)))
    per_u["in_backtest"] = per_u.price_history_days >= 400
    save("data_by_underlying.csv", per_u)
    json.dump(ov, open(OUT / "data_overview.json", "w"), ensure_ascii=False, indent=1)

    # ---------------------------------------------------------------- 2. model price vs market price (real rows, Jul-Nov 2026)
    last_px = date(2026, 11, 7)  # last day of the adjusted price file
    r = q[(q.volume > 0) & (q.last_price > 0) & (q.underlying_price > 0) & (q.trade_date.dt.date <= last_px)].copy()
    rows = []
    for x in r.itertuples():
        d = x.trade_date.date()
        T = (x.expiry.date() - d).days / 365
        if T <= 0:
            continue
        fin = m.finals_until(x.underlying_symbol, d, 21)
        sig = hv20(fin) if len(fin) >= 3 else None
        if not sig:
            continue
        mp = bs_price(x.option_type, x.underlying_price, x.strike, T, sig)
        rows.append((x.trade_date, x.underlying_symbol, x.option_type, x.strike, x.underlying_price, (x.expiry.date() - d).days, sig, x.last_price, mp))
    mv = pd.DataFrame(rows, columns=["date", "underlying", "type", "strike", "spot", "dte", "hv20", "market", "model"])
    mv["moneyness"] = mv.strike / mv.spot
    mv["ratio"] = mv.market / mv.model
    mv["intrinsic"] = np.where(mv.type == "call", (mv.spot - mv.strike).clip(lower=0), (mv.strike - mv.spot).clip(lower=0))
    mv["time_value"] = (mv.market - mv.intrinsic)
    mv = mv[(mv.model > 1) & (mv.market > 1)]
    mv["bucket"] = pd.cut(mv.moneyness, [0, .85, .95, 1.05, 1.15, 10], labels=["deep ITM call / OTM put (K<85%)", "K 85-95%", "ATM (K 95-105%)", "K 105-115%", "K>115%"])
    mv["dte_bucket"] = pd.cut(mv.dte, [0, 14, 30, 60, 400], labels=["<=14d", "15-30d", "31-60d", ">60d"])
    # implied vol from market price (bisection)
    def iv(row):
        lo, hi = 0.01, 4.0
        if row.market <= row.intrinsic + 1e-9:
            return np.nan
        for _ in range(50):
            mid = (lo + hi) / 2
            if bs_price(row.type, row.spot, row.strike, row.dte / 365, mid) > row.market:
                hi = mid
            else:
                lo = mid
        return (lo + hi) / 2
    mv["iv"] = mv.apply(iv, axis=1)
    mv["iv_hv"] = mv.iv / mv.hv20
    mv.to_csv(OUT / "market_vs_model_rows.csv", index=False, encoding="utf-8-sig")
    mm = mv[mv.type.isin(["call", "put"])]
    tab = mm.groupby(["type", "bucket"], observed=True).agg(n=("ratio", "size"), median_ratio=("ratio", "median"), p25=("ratio", lambda x: x.quantile(.25)), p75=("ratio", lambda x: x.quantile(.75)), median_iv_over_hv=("iv_hv", "median")).reset_index()
    save("market_vs_model_by_moneyness.csv", tab)
    tab2 = mm[(mm.moneyness > .95) & (mm.moneyness < 1.05)].groupby(["type", "dte_bucket"], observed=True).agg(n=("ratio", "size"), median_ratio=("ratio", "median"), median_iv=("iv", "median"), median_hv20=("hv20", "median"), median_iv_over_hv=("iv_hv", "median")).reset_index()
    save("market_vs_model_atm_by_dte.csv", tab2)
    atm = mm[(mm.moneyness > .95) & (mm.moneyness < 1.05) & mm.iv.notna()]
    mv_sum = {"rows_compared": int(len(mm)), "median_market_over_model": float(mm.ratio.median()), "atm_rows": int(len(atm)), "atm_median_iv": float(atm.iv.median()), "atm_median_hv20": float(atm.hv20.median()),
              "atm_median_iv_over_hv": float(atm.iv_hv.median()), "share_market_above_model": float((mm.ratio > 1).mean()), "share_atm_iv_above_hv": float((atm.iv_hv > 1).mean())}
    json.dump(mv_sum, open(OUT / "market_vs_model_summary.json", "w"), indent=1)

    fig, ax = plt.subplots(1, 2, figsize=(10, 3.8))
    a = tab.pivot(index="bucket", columns="type", values="median_ratio")
    a.plot.bar(ax=ax[0], color=[BLUE, ORANGE], width=.75)
    ax[0].axhline(1, color="k", lw=.8, ls="--")
    ax[0].set_ylabel("market price / model price (median)")
    ax[0].set_xlabel("strike / underlying price")
    ax[0].set_title("Real traded price vs Black-Scholes at HV20")
    ax[0].tick_params(axis="x", rotation=25)
    for lab in ax[0].get_xticklabels():
        lab.set_ha("right")
    ax[1].hist(atm.iv_hv.clip(0, 4), bins=40, color=BLUE)
    ax[1].axvline(1, color="k", lw=.8, ls="--")
    ax[1].set_xlabel("implied volatility / HV20 (ATM rows)")
    ax[1].set_title("Market volatility vs the backtest's volatility")
    fig.tight_layout()
    fig.savefig(FIG / "market_vs_model.png")
    plt.close(fig)

    # ---------------------------------------------------------------- 3. realised vs model volatility and drift (why long options win in the model)
    rows = []
    for u in sorted(base.underlying.unique()):
        days = m.trading_days(u)
        fin = {d: p for d, p in zip(days, m._final[u])}
        for d in days[60::5]:
            i = days.index(d)
            if i + 20 >= len(days):
                break
            sig0 = hv20(m.finals_until(u, d, 21))
            fut = m.finals_until(u, days[i + 20], 21)
            sig1 = hv20(fut)
            if sig0 and sig1:
                rows.append((u, d, sig0, sig1, math.log(fin[days[i + 20]] / fin[d])))
    vv = pd.DataFrame(rows, columns=["underlying", "date", "hv_entry", "hv_forward", "ret_20d"])
    vv["year"] = pd.to_datetime(vv.date).dt.year
    vv["ratio"] = vv.hv_forward / vv.hv_entry
    t3 = vv.groupby("year").agg(n=("ratio", "size"), median_hv_entry=("hv_entry", "median"), median_hv_forward=("hv_forward", "median"), median_ratio=("ratio", "median"), mean_20d_log_return=("ret_20d", "mean"), share_20d_up=("ret_20d", lambda x: (x > 0).mean())).reset_index()
    save("realised_vs_model_vol_by_year.csv", t3)
    drift_all = {"mean_20d_log_return": float(vv.ret_20d.mean()), "annualised_drift_pct": float(vv.ret_20d.mean() * 12 * 100), "model_rate_pct": 30.0, "median_forward_over_entry_vol": float(vv.ratio.median())}
    json.dump(drift_all, open(OUT / "drift_summary.json", "w"), indent=1)

    # ---------------------------------------------------------------- 4. robust results per template (baseline)
    def summ(d: pd.DataFrame):
        lo, hi = cluster_boot(d, "ror_c")
        return pd.Series({"trades": len(d), "win_rate": d.win.mean(), "median_ror": d.return_on_risk.median(), "mean_ror_clipped": d.ror_c.mean(), "ci_low": lo, "ci_high": hi,
                          "avg_pnl_rial": d.pnl.mean(), "profit_factor": d.pnl[d.pnl > 0].sum() / -d.pnl[d.pnl < 0].sum() if (d.pnl < 0).any() else np.nan,
                          "fees_over_gross": d.fees.sum() / max(abs(d.gross_pnl).sum(), 1), "share_of_gain_from_top5pct": d.ror_c.sort_values(ascending=False).head(max(int(len(d) * .05), 1)).sum() / d.ror_c[d.ror_c > 0].sum()})
    by_t = base.groupby("template").apply(summ, include_groups=False).reindex(TPL_ORDER).reset_index()
    runs_b = runs[runs.scenario == "base_e30"]
    sk_b = sk[sk.scenario == "base_e30"]
    by_t["skipped_share"] = by_t.template.map(lambda t: runs_b[runs_b.template == t].skipped.sum() / (runs_b[runs_b.template == t].skipped.sum() + runs_b[runs_b.template == t].trades.sum()))
    by_t["engine_total_return_median_over_underlyings"] = by_t.template.map(runs_b.groupby("template").total_return.median())
    by_t["engine_max_drawdown_median"] = by_t.template.map(runs_b.groupby("template").max_drawdown.median())
    save("results_by_template.csv", by_t)

    b22 = tr[tr.scenario == "vol_x2.2"]
    by_t22 = b22.groupby("template").apply(summ, include_groups=False).reindex(TPL_ORDER).reset_index()
    save("results_by_template_vol_x2.2.csv", by_t22)

    by_u = base.groupby("underlying").agg(trades=("pnl", "size"), win_rate=("win", "mean"), mean_ror_clipped=("ror_c", "mean"), median_ror=("return_on_risk", "median")).reset_index().sort_values("mean_ror_clipped", ascending=False)
    save("results_by_underlying.csv", by_u)
    hm = base.pivot_table(index="underlying", columns="template", values="ror_c", aggfunc="mean")[TPL_ORDER]
    hm = hm.loc[hm.mean(axis=1).sort_values(ascending=False).index]
    hm.reset_index().to_csv(OUT / "heatmap_mean_ror_clipped.csv", index=False, encoding="utf-8-sig")
    wr = base.pivot_table(index="underlying", columns="template", values="win", aggfunc="mean")[TPL_ORDER].loc[hm.index]
    wr.reset_index().to_csv(OUT / "heatmap_win_rate.csv", index=False, encoding="utf-8-sig")
    by_year = base.groupby(["template", "year"]).agg(trades=("pnl", "size"), win_rate=("win", "mean"), mean_ror_clipped=("ror_c", "mean")).reset_index()
    save("results_by_template_year.csv", by_year)

    # costs: fees relative to premium
    cost = base.groupby("template").apply(lambda d: pd.Series({"avg_fees_rial": d.fees.mean(), "avg_gross_pnl_rial": d.gross_pnl.mean(), "avg_net_premium_rial": d.net_premium.abs().mean(), "fees_pct_of_capital_at_risk": (d.fees / d.capital_at_risk).median() * 100}), include_groups=False).reindex(TPL_ORDER).reset_index()
    save("costs_by_template.csv", cost)

    # ---------------------------------------------------------------- 5. sensitivity: entry time and exit rule
    sens = tr.groupby(["scenario", "template"]).apply(lambda d: pd.Series({"trades": len(d), "win_rate": d.win.mean(), "mean_ror_clipped": d.ror_c.mean(), "median_ror": d.return_on_risk.median()}), include_groups=False).reset_index()
    save("sensitivity.csv", sens)

    # ---------------------------------------------------------------- 6. link to task 7: HV rank at entry
    f = pd.read_parquet(FEATURES)
    f["underlying"] = f.symbol.map(fa)
    f["date"] = pd.to_datetime(f.date)
    fe = f[["underlying", "date", "hv_rank_250", "hv_rank_120", "hv_20", "ret_20", "rsi_14"]].rename(columns={"date": "entry_date"})
    bj = base.merge(fe, on=["underlying", "entry_date"], how="left")
    cov = float(bj.hv_rank_120.notna().mean())
    bj = bj[bj.hv_rank_120.notna()].copy()
    bj["hv_bucket"] = pd.cut(bj.hv_rank_120, [-0.1, 33.3, 66.7, 100.1], labels=["low (0-33)", "mid (33-67)", "high (67-100)"])
    t6 = bj.groupby(["template", "hv_bucket"], observed=True).agg(trades=("pnl", "size"), win_rate=("win", "mean"), mean_ror_clipped=("ror_c", "mean"), median_ror=("return_on_risk", "median")).reset_index()
    save("results_by_hv_rank_at_entry.csv", t6)
    json.dump({"share_of_baseline_trades_with_hv_rank_120": cov, "trades_used": int(len(bj))}, open(OUT / "hv_rank_link_summary.json", "w"))
    # momentum filter (ret_20 at entry) for directional templates
    bj["trend"] = np.where(bj.ret_20 > 0, "up (ret20>0)", "down (ret20<=0)")
    t7 = bj[bj.ret_20.notna()].groupby(["template", "trend"]).agg(trades=("pnl", "size"), win_rate=("win", "mean"), mean_ror_clipped=("ror_c", "mean")).reset_index()
    save("results_by_trend_at_entry.csv", t7)

    # ---------------------------------------------------------------- 7. skip reasons
    def reason_class(s: str) -> str:
        if "jumped" in s:
            return "price jump (capital action) while open"
        if "no trading day" in s:
            return "no trading day near entry date"
        if "minimum premium" in s:
            return "a leg priced below minimum premium"
        if "no price for" in s:
            return "no price for a leg (strike outside the grid)"
        return "other"
    sk_b = sk_b.assign(kind=sk_b.reason.map(reason_class))
    save("skip_reasons.csv", sk_b.groupby("kind").size().rename("count").reset_index().sort_values("count", ascending=False))

    # ---------------------------------------------------------------- charts
    # (a) template summary
    fig, ax = plt.subplots(1, 3, figsize=(11, 3.8), sharey=True)
    y = np.arange(len(by_t))
    ax[0].barh(y, by_t.win_rate * 100, color=BLUE)
    ax[0].set_title("Win rate (%)")
    ax[0].axvline(50, color="k", lw=.7, ls="--")
    ax[1].barh(y, by_t.median_ror * 100, color=[GREEN if v > 0 else RED for v in by_t.median_ror])
    ax[1].set_title("Median return on risk per trade (%)")
    ax[1].axvline(0, color="k", lw=.7)
    ax[2].barh(y, by_t.mean_ror_clipped * 100, xerr=[(by_t.mean_ror_clipped - by_t.ci_low) * 100, (by_t.ci_high - by_t.mean_ror_clipped) * 100], color=[GREEN if v > 0 else RED for v in by_t.mean_ror_clipped], ecolor="k", capsize=2)
    ax[2].set_title("Mean return on risk (%), 95% CI")
    ax[2].axvline(0, color="k", lw=.7)
    ax[0].set_yticks(y, by_t.template)
    ax[0].invert_yaxis()
    fig.tight_layout()
    fig.savefig(FIG / "results_by_template.png")
    plt.close(fig)

    # (b) heatmap template x underlying
    fig, ax = plt.subplots(figsize=(8.5, 8))
    im = ax.imshow(hm.values * 100, cmap="RdYlGn", vmin=-100, vmax=300, aspect="auto")
    ax.set_xticks(range(len(hm.columns)), hm.columns, rotation=35, ha="right")
    ax.set_yticks(range(len(hm.index)), [P(i) for i in hm.index])
    for i in range(hm.shape[0]):
        for j in range(hm.shape[1]):
            ax.text(j, i, f"{hm.values[i, j] * 100:.0f}", ha="center", va="center", fontsize=6.5)
    ax.set_title("Mean return on risk per trade (%, clipped), by underlying and template")
    fig.colorbar(im, ax=ax, shrink=.6)
    fig.tight_layout()
    fig.savefig(FIG / "heatmap_template_underlying.png")
    plt.close(fig)

    # (c) win rate by year per template
    fig, ax = plt.subplots(figsize=(8, 3.8))
    for t, c in zip(["long-call", "long-put", "long-straddle", "covered-call"], [GREEN, RED, BLUE, ORANGE]):
        d = by_year[by_year.template == t]
        ax.plot(d.year.astype(str), d.mean_ror_clipped * 100, marker="o", label=t, color=c)
    ax.axhline(0, color="k", lw=.7)
    ax.set_ylabel("mean return on risk (%, clipped)")
    ax.set_title("Same strategies, different years (model-priced)")
    ax.legend(frameon=False)
    fig.tight_layout()
    fig.savefig(FIG / "by_year.png")
    plt.close(fig)

    # (d) realised vs model vol/ drift
    fig, ax = plt.subplots(1, 2, figsize=(10, 3.6))
    ax[0].hist(vv.ratio.clip(0, 4), bins=50, color=BLUE)
    ax[0].axvline(1, color="k", ls="--", lw=.8)
    ax[0].set_title("Volatility over the next 20 days / HV20 at entry")
    ax[1].hist(vv.ret_20d.clip(-.4, .6) * 100, bins=60, color=ORANGE)
    ax[1].axvline(0, color="k", lw=.7)
    ax[1].axvline(30 / 12, color="k", ls="--", lw=.8)
    ax[1].set_title("20-day log return (%); dashed = model drift 30%/yr")
    fig.tight_layout()
    fig.savefig(FIG / "realised_vs_model.png")
    plt.close(fig)

    # (e) HV rank link
    fig, ax = plt.subplots(figsize=(8, 3.8))
    p = t6.pivot(index="template", columns="hv_bucket", values="mean_ror_clipped").reindex(TPL_ORDER) * 100
    p.plot.bar(ax=ax, color=[BLUE, GREY, ORANGE], width=.8)
    ax.axhline(0, color="k", lw=.7)
    ax.set_ylabel("mean return on risk (%, clipped)")
    ax.set_xlabel("")
    ax.set_title("Result by HV rank (120d) on the entry day")
    ax.tick_params(axis="x", rotation=25)
    for lab in ax.get_xticklabels():
        lab.set_ha("right")
    ax.legend(title="HV rank", frameon=False)
    fig.tight_layout()
    fig.savefig(FIG / "by_hv_rank.png")
    plt.close(fig)

    # (f) sensitivity
    fig, ax = plt.subplots(1, 2, figsize=(12, 5.2), sharey=False)
    for k, (col, tt) in enumerate([("win_rate", "Win rate (%)"), ("mean_ror_clipped", "Mean return on risk (%, clipped)")]):
        p = sens.pivot(index="template", columns="scenario", values=col).reindex(TPL_ORDER)[["base_e30", "e14", "e7", "target_stop_e30", "vol_x1.5", "vol_x2.2"]] * 100
        p.columns = ["30d, hold", "14d, hold", "7d, hold", "30d, target/stop 50%", "priced at 1.5x HV20", "priced at 2.2x HV20"]
        p.plot.bar(ax=ax[k], width=.85)
        ax[k].set_title(tt)
        ax[k].tick_params(axis="x", rotation=40)
        for lab in ax[k].get_xticklabels():
            lab.set_ha("right")
        ax[k].legend(fontsize=7, frameon=False, loc="upper center", bbox_to_anchor=(.5, -.42), ncol=3)
        ax[k].axhline(0, color="k", lw=.7)
    fig.tight_layout()
    fig.savefig(FIG / "sensitivity.png")
    plt.close(fig)

    # (g) example equity curves (engine's own definition, no compounding)
    curves = json.load(open(OUT / "equity_curves_base.json", encoding="utf-8"))
    fig, ax = plt.subplots(1, 2, figsize=(10, 3.6))
    for u, c in zip(["اهرم", "خودرو", "شستا"], [BLUE, ORANGE, GREEN]):
        for k, tpl in enumerate(["long-straddle", "covered-call"]):
            pts = curves.get(f"{u}|{tpl}")
            if pts:
                ax[k].plot(pd.to_datetime([p["date"] for p in pts]), [p["equity"] for p in pts], label=P(u), color=c, lw=1)
    ax[0].set_title("Equity curve: long-straddle")
    ax[1].set_title("Equity curve: covered-call")
    for a_ in ax:
        a_.legend(frameon=False)
        a_.axhline(1, color="k", lw=.6)
    fig.tight_layout()
    fig.savefig(FIG / "equity_examples.png")
    plt.close(fig)
    print("done")
    print(json.dumps(ov, ensure_ascii=False)[:600]); print(mv_sum); print(drift_all)


if __name__ == "__main__":
    main()
