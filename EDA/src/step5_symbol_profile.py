"""Step 5 - symbol profile: coverage, liquidity, adjusted price performance and investor mix per symbol.

Inputs : raw daily file, step 3 (trading_status, usable_data_by_symbol), step 4 (adjusted_series, adj_event_final).
Outputs (outputs/step5_profile/):
  symbol_profile.csv            one row per symbol (coverage, halts, price, liquidity, investor mix, tier)
  liquidity_tiers.csv           tier definition and members
  median_daily_value.png        median daily traded value per symbol (log scale, coloured by tier)
  active_share_vs_liquidity.png active-day share against liquidity
  raw_vs_adjusted_price.png     raw final price vs corrected adjusted price, one panel per symbol
  checks_summary.csv, summary.json
Liquidity tiers are data-driven terciles of median daily traded value among symbols with >= MIN_ACTIVE active days.
Business groups (stock / fund / bank ...) are NOT inferred here - they need external reference data.
"""
from __future__ import annotations

import json

import matplotlib

matplotlib.use("Agg")
import matplotlib.dates as mdates
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

from common import fa_label, load_daily, out_dir, save_table

MIN_ACTIVE = 200
TIER_COLORS = {"T1_high": "#2a9d8f", "T2_mid": "#e9c46a", "T3_low": "#e76f51", "short_history": "#9aa0a6"}


def main() -> None:
    OUT = out_dir("step5_profile")
    S3, S4 = OUT.parent / "step3_halts_gaps", OUT.parent / "step4_adjustments"
    checks: list[dict] = []

    def add(i, name, status, n, detail):
        checks.append({"id": i, "check": name, "status": status, "n_issues": n, "detail": detail})

    df = load_daily()
    adj = pd.read_csv(S4 / "adjusted_series.csv", encoding="utf-8-sig", parse_dates=["date"])
    df = df.merge(adj[["symbol", "date", "adj_final_corrected", "price_anomaly"]], on=["symbol", "date"],
                  how="left", validate="1:1")
    status = pd.read_csv(S3 / "trading_status.csv", encoding="utf-8-sig", parse_dates=["date"])
    usable = pd.read_csv(S3 / "usable_data_by_symbol.csv", encoding="utf-8-sig").set_index("symbol")
    ev = pd.read_csv(S4 / "adj_event_final.csv", encoding="utf-8-sig")
    st = status.groupby("symbol")["status"].value_counts().unstack(fill_value=0)
    artificial = {"REVERSAL_CLUSTER", "LISTING_REFERENCE"}
    ev_valid = ev[~ev["event_class"].isin(artificial)].groupby("symbol").size()
    ev_all = df[df["adj_event"]].groupby("symbol").size()

    rows = []
    for sym, g in df.groupby("symbol", sort=True):
        act = g[~g["is_halted"]]
        px = act[act["adj_final_corrected"].notna() & ~act["price_anomaly"]]
        first, last = px.iloc[0], px.iloc[-1]
        s = st.loc[sym]
        tradable = int(s.drop(labels=["MARKET_CLOSED"], errors="ignore").sum())
        buy_tot = (act["ind_buy_val"] + act["corp_buy_val"]).sum()
        sell_tot = (act["ind_sell_val"] + act["corp_sell_val"]).sum()
        rows.append({
            "symbol": sym, "ins_code": int(g["ins_code"].iloc[0]),
            "first_record": g["date"].min().date(), "first_trade_date": act["date"].min().date(),
            "last_date": g["date"].max().date(), "n_records": len(g),
            "tradable_days": tradable, "active_days": int(s.get("ACTIVE", 0)),
            "symbol_halt_days": int(s.get("HALT_SYMBOL", 0)), "no_record_days": int(s.get("NO_RECORD", 0)),
            "symbol_halt_pct": round(100 * s.get("HALT_SYMBOL", 0) / tradable, 2),
            "active_share": round(s.get("ACTIVE", 0) / tradable, 4),
            "longest_active_streak": int(usable.loc[sym, "longest_active_streak"]),
            "n_adj_events_raw": int(ev_all.get(sym, 0)), "n_adj_events_valid": int(ev_valid.get(sym, 0)),
            "first_adj_price": first["adj_final_corrected"], "last_adj_price": last["adj_final_corrected"],
            "adj_total_return": last["adj_final_corrected"] / first["adj_final_corrected"] - 1,
            "adj_log_return": np.log(last["adj_final_corrected"] / first["adj_final_corrected"]),
            "median_final_price_raw": act["final_price"].median(), "last_final_price_raw": act["final_price"].iloc[-1],
            "median_value": act["value"].median(), "mean_value": act["value"].mean(), "p10_value": act["value"].quantile(.10),
            "value_cv": act["value"].std() / act["value"].mean(),
            "median_trade_count": act["trade_count"].median(), "mean_trade_count": act["trade_count"].mean(),
            "median_avg_trade_value": (act["value"] / act["trade_count"]).median(),
            "retail_buy_value_share": act["ind_buy_val"].sum() / buy_tot if buy_tot else np.nan,
            "retail_sell_value_share": act["ind_sell_val"].sum() / sell_tot if sell_tot else np.nan,
            "median_retail_buyers": act["ind_buy_count"].median(), "median_corp_buyers": act["corp_buy_count"].median(),
        })
    P = pd.DataFrame(rows)
    P["median_value_billion_rial"] = P["median_value"] / 1e9
    P["liquidity_rank"] = P["median_value"].rank(ascending=False, method="first").astype(int)
    eligible = P["active_days"] >= MIN_ACTIVE
    q1, q2 = P.loc[eligible, "median_value"].quantile([1 / 3, 2 / 3])
    P["liquidity_tier"] = np.select([~eligible, P["median_value"] >= q2, P["median_value"] >= q1],
                                    ["short_history", "T1_high", "T2_mid"], default="T3_low")
    P = P.sort_values("liquidity_rank").reset_index(drop=True)
    save_table(P.round(6), OUT / "symbol_profile.csv")
    tiers = P.groupby("liquidity_tier").agg(n_symbols=("symbol", "size"), min_median_value=("median_value", "min"),
                                           max_median_value=("median_value", "max"),
                                           symbols=("symbol", lambda s: " ".join(s))).reset_index()
    save_table(tiers, OUT / "liquidity_tiers.csv")

    # ---------------- checks (cross-validated against steps 1, 3, 4)
    add("S5.01", "one profile row per symbol", "PASS" if len(P) == df["symbol"].nunique() == 27 else "FAIL", 0, f"{len(P)} symbols")
    hb = pd.read_csv(OUT.parent / "step1_structure" / "halted_breakdown_by_symbol.csv", encoding="utf-8-sig").set_index("symbol")
    mism = int((P.set_index("symbol")["symbol_halt_days"] != hb["halted_symbol_specific"].reindex(P["symbol"]).values).sum())
    add("S5.02", "symbol_halt_days equals the step 1 symbol-specific halt count", "PASS" if mism == 0 else "FAIL", mism, "")
    act_ok = int((P["active_days"] != P["n_records"] - df.groupby("symbol")["is_halted"].sum().reindex(P["symbol"]).values).sum())
    add("S5.03", "active_days + halted records = records (closure days are halted records)", "INFO", act_ok,
        "difference = market-closure rows, which are excluded from tradable_days")
    add("S5.04", "adjusted prices positive and finite", "PASS" if (P[["first_adj_price", "last_adj_price"]] > 0).all().all() else "FAIL", 0, "")
    elig = P[P["active_days"] >= MIN_ACTIVE]
    top, bot = elig.iloc[0], elig.iloc[-1]
    add("S5.05", f"liquidity range (median daily traded value, symbols with >= {MIN_ACTIVE} active days)", "INFO", None,
        f"most liquid {top['symbol']}: {top['median_value_billion_rial']:.1f} bn Rial; least liquid "
        f"{bot['symbol']}: {bot['median_value_billion_rial']:.2f} bn Rial; ratio {top['median_value'] / bot['median_value']:.0f}x; "
        f"short-history symbols excluded: {P.loc[P['active_days'] < MIN_ACTIVE, 'symbol'].tolist()}")
    add("S5.06", "liquidity tiers (terciles among symbols with enough history)", "INFO", None,
        "; ".join(f"{r.liquidity_tier}={r.n_symbols}" for r in tiers.itertuples()))
    short = P[P["active_days"] < MIN_ACTIVE]
    add("S5.07", f"symbols with fewer than {MIN_ACTIVE} active days", "WARN" if len(short) else "PASS", len(short),
        "; ".join(f"{r.symbol}={r.active_days}" for r in short.itertuples()))
    add("S5.08", "retail share of buy value across symbols", "INFO", None,
        f"min {P['retail_buy_value_share'].min():.2f}, median {P['retail_buy_value_share'].median():.2f}, max {P['retail_buy_value_share'].max():.2f}")
    add("S5.09", "adjusted total return over each symbol's active period", "INFO", None,
        f"median {P['adj_total_return'].median():+.1%}; best {P.loc[P['adj_total_return'].idxmax(), 'symbol']} {P['adj_total_return'].max():+.1%}; "
        f"worst {P.loc[P['adj_total_return'].idxmin(), 'symbol']} {P['adj_total_return'].min():+.1%}")

    # ---------------- charts
    colors = P["liquidity_tier"].map(TIER_COLORS)
    fig, ax = plt.subplots(figsize=(9, 8))
    ax.barh(range(len(P)), P["median_value_billion_rial"], color=colors)
    ax.set_yticks(range(len(P)))
    ax.set_yticklabels([fa_label(s) for s in P["symbol"]], fontsize=8)
    ax.invert_yaxis()
    ax.set_xscale("log")
    ax.set_xlabel("median daily traded value (billion Rial, log scale)")
    ax.set_title("Liquidity by symbol (colour = tier)")
    ax.legend(handles=[plt.Rectangle((0, 0), 1, 1, color=c) for c in TIER_COLORS.values()], labels=list(TIER_COLORS), fontsize=8, loc="lower right")
    fig.tight_layout()
    fig.savefig(OUT / "median_daily_value.png", dpi=130)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(9, 6))
    ax.scatter(P["median_value_billion_rial"], P["active_share"], c=colors, s=40)
    for r in P.itertuples():
        ax.annotate(fa_label(r.symbol), (r.median_value_billion_rial, r.active_share), fontsize=7, xytext=(3, 3), textcoords="offset points")
    ax.set_xscale("log")
    ax.set_xlabel("median daily traded value (billion Rial, log scale)")
    ax.set_ylabel("active share of tradable days")
    ax.set_title("Active-day share vs liquidity")
    fig.tight_layout()
    fig.savefig(OUT / "active_share_vs_liquidity.png", dpi=130)
    plt.close(fig)

    syms = sorted(df["symbol"].unique())
    fig, axes = plt.subplots(6, 5, figsize=(18, 15), sharex=True)
    for ax, sym in zip(axes.ravel(), syms):
        g = df[(df["symbol"] == sym) & ~df["is_halted"]]
        ax.plot(g["date"], g["final_price"], color="#9aa0a6", lw=0.9, label="raw final")
        ax.plot(g["date"], g["adj_final_corrected"], color="#1f4e79", lw=0.9, label="adjusted")
        ax.set_yscale("log")
        ax.set_title(fa_label(sym), fontsize=9)
        ax.tick_params(labelsize=6)
    for ax in axes.ravel()[len(syms):]:
        ax.axis("off")
    for ax in axes.ravel()[:len(syms)]:
        ax.xaxis.set_major_locator(mdates.MonthLocator(bymonth=(1, 7)))
        ax.xaxis.set_major_formatter(mdates.DateFormatter("%y-%m"))
        ax.tick_params(axis="x", labelbottom=True, rotation=0)
    axes.ravel()[0].legend(fontsize=7)
    fig.suptitle("Raw final price vs corrected adjusted price (log scale, active days)")
    fig.tight_layout()
    fig.savefig(OUT / "raw_vs_adjusted_price.png", dpi=100)
    plt.close(fig)

    res = pd.DataFrame(checks)
    save_table(res, OUT / "checks_summary.csv")
    (OUT / "summary.json").write_text(json.dumps({"symbols": len(P), "tiers": P["liquidity_tier"].value_counts().to_dict(),
                                                 **res["status"].value_counts().to_dict()}, ensure_ascii=False, indent=2))
    pd.set_option("display.width", 250, "display.max_colwidth", 200)
    print(res[["id", "status", "n_issues", "detail"]].to_string(index=False))


if __name__ == "__main__":
    main()
