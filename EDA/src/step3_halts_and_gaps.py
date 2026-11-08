"""Step 3 - halted days and gaps: what they are, how they relate to corporate actions, and how much usable data is left.

Outputs (outputs/step3_halts_gaps/):
  trading_status.csv          symbol x market-day status: ACTIVE / HALT_SYMBOL / MARKET_CLOSED / NO_RECORD
  halt_runs.csv               every consecutive run of halted records, with composition, event link and reopening stats
  halt_run_summary_by_symbol.csv
  reopening_stats.csv         reopen-day return / volume vs ordinary active days
  usable_data_by_symbol.csv   active days, longest active streak, share of fully-active 20/60-day windows
  gap_context.csv             the market days where a listed symbol has no record, with neighbouring status
  checks_summary.csv, halt_run_lengths.png, halted_rows_by_month.png, summary.json
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

from common import load_daily, market_calendar, market_closure_days, out_dir, save_table

OUT = out_dir("step3_halts_gaps")
checks: list[dict] = []


def add(check_id: str, name: str, status: str, n: int | None, detail: str) -> None:
    checks.append({"id": check_id, "check": name, "status": status, "n_issues": n, "detail": detail})


def length_bin(n: int) -> str:
    return "1" if n == 1 else "2-3" if n <= 3 else "4-10" if n <= 10 else ">10"


def main() -> None:
    df = load_daily()
    calendar = market_calendar(df)
    pos = pd.Series(np.arange(len(calendar)), index=calendar)
    closure = set(market_closure_days(df))
    df["in_closure"] = df["date"].isin(closure)

    # per-row volume baseline: median of the previous 20 ACTIVE volumes of the same symbol
    act = df[~df["is_halted"]].copy()
    act["vol_med20"] = act.groupby("symbol")["volume"].transform(lambda s: s.shift(1).rolling(20, min_periods=10).median())
    act["vol_ratio"] = act["volume"] / act["vol_med20"]
    act["ret"] = act["final_price"] / act["prev_final"] - 1
    act["open_gap"] = act["open"] / act["prev_final"] - 1
    act_idx = act.set_index(["symbol", "date"])

    # ============ trading_status grid ============================================================
    parts = []
    for sym, g in df.groupby("symbol", sort=True):
        s = pd.DataFrame({"date": calendar[calendar >= g["date"].min()]})
        s["symbol"] = sym
        halted_map = g.set_index("date")["is_halted"]
        has_rec = s["date"].isin(halted_map.index)
        halted = s["date"].map(halted_map).fillna(False).astype(bool)
        s["status"] = np.select([s["date"].isin(closure), ~has_rec, halted],
                                ["MARKET_CLOSED", "NO_RECORD", "HALT_SYMBOL"], default="ACTIVE")
        parts.append(s[["symbol", "date", "status"]])
    status = pd.concat(parts, ignore_index=True)
    save_table(status, OUT / "trading_status.csv")
    st_counts = status["status"].value_counts().to_dict()

    # ============ halt runs ======================================================================
    runs = []
    for sym, g in df.groupby("symbol", sort=True):
        g = g.reset_index(drop=True)
        run_id = (g["is_halted"] != g["is_halted"].shift()).cumsum()
        first_date = g["date"].iloc[0]
        for _, r in g[g["is_halted"]].groupby(run_id[g["is_halted"]]):
            i0, i1 = r.index[0], r.index[-1]
            n_closure = int(r["in_closure"].sum())
            nxt = g.iloc[i1 + 1] if i1 + 1 < len(g) else None  # first active record after the run
            rec = {
                "symbol": sym, "start": r["date"].iloc[0].date(), "end": r["date"].iloc[-1].date(),
                "n_records": len(r), "n_closure_days": n_closure, "n_symbol_days": len(r) - n_closure,
                "span_market_days": int(pos[r["date"].iloc[-1]] - pos[r["date"].iloc[0]] + 1),
                "starts_at_first_record": bool(r["date"].iloc[0] == first_date),
                "ends_at_last_record": nxt is None,
                "event_in_run": bool(r["adj_event"].any()),
                "event_on_reopen_day": bool(nxt is not None and nxt["adj_event"]),
            }
            rec["kind"] = ("market_wide_only" if rec["n_symbol_days"] == 0
                           else "symbol_only" if n_closure == 0 else "mixed")
            if nxt is not None:
                a = act_idx.loc[(sym, nxt["date"])]
                rec.update({"reopen_date": nxt["date"].date(), "reopen_ret": a["ret"], "reopen_open_gap": a["open_gap"],
                            "reopen_vol_ratio": a["vol_ratio"]})
            runs.append(rec)
    runs = pd.DataFrame(runs)
    runs["len_bin_symbol_days"] = runs["n_symbol_days"].map(lambda n: length_bin(n) if n > 0 else "0 (closure only)")
    save_table(runs, OUT / "halt_runs.csv")

    total_halted = int(df["is_halted"].sum())
    add("S3.01", "halt runs partition all halted rows", "PASS" if runs["n_records"].sum() == total_halted else "FAIL",
        int(runs["n_records"].sum() - total_halted),
        f"{len(runs)} runs cover {int(runs['n_records'].sum())} of {total_halted} halted rows; "
        f"kinds: {runs['kind'].value_counts().to_dict()}")

    sym_runs = runs[runs["kind"] != "market_wide_only"]
    long_runs = sym_runs[sym_runs["n_symbol_days"] > 10]
    add("S3.02", "symbol-specific halt runs: length distribution", "INFO", len(sym_runs),
        f"{sym_runs['len_bin_symbol_days'].value_counts().to_dict()}; runs >10 symbol-days = {len(long_runs)} "
        f"holding {int(long_runs['n_symbol_days'].sum())} of {int(sym_runs['n_symbol_days'].sum())} symbol-halt days "
        f"({100 * long_runs['n_symbol_days'].sum() / sym_runs['n_symbol_days'].sum():.0f}%)")

    per_sym = runs.groupby("symbol").agg(
        n_runs=("n_records", "size"), halted_records=("n_records", "sum"),
        symbol_halt_days=("n_symbol_days", "sum"), closure_days=("n_closure_days", "sum"),
        single_day_runs=("n_symbol_days", lambda s: int((s == 1).sum())),
        longest_symbol_run=("n_symbol_days", "max"), median_run_records=("n_records", "median")).reset_index()
    save_table(per_sym.sort_values("symbol_halt_days", ascending=False), OUT / "halt_run_summary_by_symbol.csv")

    # ============ link to adjustment events ====================================================
    ev = df[df["adj_event"]]
    in_halt = int(ev["is_halted"].sum())
    ev_reopen = int(runs["event_on_reopen_day"].sum())
    add("S3.03", "every adjustment event sits on a halted day or on the reopen day",
        "PASS" if in_halt + ev_reopen == len(ev) else "WARN", len(ev) - in_halt - ev_reopen,
        f"{len(ev)} events: {in_halt} on halted rows, {ev_reopen} on the first active day after a halt, "
        f"{len(ev) - in_halt - ev_reopen} elsewhere")
    n_with_event = int((sym_runs["event_in_run"] | sym_runs["event_on_reopen_day"]).sum())
    add("S3.04", "share of symbol-specific halt runs tied to a corporate action", "INFO", n_with_event,
        f"{n_with_event}/{len(sym_runs)} runs ({100 * n_with_event / len(sym_runs):.0f}%); among runs >3 symbol-days: "
        f"{int((sym_runs[sym_runs['n_symbol_days'] > 3]['event_in_run'] | sym_runs[sym_runs['n_symbol_days'] > 3]['event_on_reopen_day']).sum())}"
        f"/{int((sym_runs['n_symbol_days'] > 3).sum())}")

    # ============ reopening behaviour ==========================================================
    ro = runs.dropna(subset=["reopen_ret"]).copy()
    ro["abs_ret"] = ro["reopen_ret"].abs()
    ro["group"] = np.where(ro["kind"] == "market_wide_only", "market_closure", "symbol_halt")
    prev_active = act.groupby("symbol")["is_halted"].transform(lambda s: s)  # placeholder (all False)
    # baseline: active days whose previous record was also active, no event
    df["prev_halted"] = df.groupby("symbol")["is_halted"].shift()
    base_idx = df[(~df["is_halted"]) & (df["prev_halted"] == False) & (~df["adj_event"])].set_index(["symbol", "date"]).index
    base = act_idx.loc[act_idx.index.intersection(base_idx)]
    base = base[base["ret"].abs() < 2]
    rows = [{"group": "baseline_active_after_active", "len_bin": "all", "n": len(base),
             "median_abs_ret": base["ret"].abs().median(), "p95_abs_ret": base["ret"].abs().quantile(.95),
             "share_abs_ret_over_3pct": (base["ret"].abs() > 0.03).mean(),
             "median_open_gap_abs": base["open_gap"].abs().median(), "median_vol_ratio": base["vol_ratio"].median()}]
    for (grp, b), g in ro.groupby(["group", "len_bin_symbol_days"]):
        g = g[g["abs_ret"] < 2]
        rows.append({"group": grp, "len_bin": b, "n": len(g), "median_abs_ret": g["abs_ret"].median(),
                     "p95_abs_ret": g["abs_ret"].quantile(.95), "share_abs_ret_over_3pct": (g["abs_ret"] > 0.03).mean(),
                     "median_open_gap_abs": g["reopen_open_gap"].abs().median(), "median_vol_ratio": g["reopen_vol_ratio"].median()})
    reopen = pd.DataFrame(rows).round(4)
    save_table(reopen, OUT / "reopening_stats.csv")
    short = ro[(ro["group"] == "symbol_halt") & (ro["n_symbol_days"] <= 3)]
    longr = ro[(ro["group"] == "symbol_halt") & (ro["n_symbol_days"] > 3)]
    add("S3.05", "reopen day after a halt behaves differently from an ordinary day", "WARN", len(ro),
        f"median |ret|: baseline {base['ret'].abs().median():.2%}, after short halt (<=3d) {short['abs_ret'].median():.2%}, "
        f"after long halt (>3d) {longr['abs_ret'].median():.2%}, after market closure "
        f"{ro[ro['group'] == 'market_closure']['abs_ret'].median():.2%}; volume vs 20-day median: "
        f"baseline {base['vol_ratio'].median():.2f}x, long halt {longr['reopen_vol_ratio'].median():.2f}x")

    # ============ usable data per symbol ========================================================
    use_rows = []
    for sym, s in status.groupby("symbol", sort=True):
        t = s[s["status"] != "MARKET_CLOSED"].reset_index(drop=True)   # tradable calendar of this symbol
        a = (t["status"] == "ACTIVE").to_numpy()
        streak = best = 0
        n_streaks = 0
        for v in a:
            if v:
                streak += 1
                if streak == 1:
                    n_streaks += 1
                best = max(best, streak)
            else:
                streak = 0
        row = {"symbol": sym, "first_record": s["date"].min().date(), "tradable_days": len(t),
               "active_days": int(a.sum()), "active_share": a.mean(),
               "first_active_date": t.loc[a, "date"].min().date() if a.any() else None,
               "longest_active_streak": best, "n_active_streaks": n_streaks}
        for w in (20, 60):
            if len(a) >= w:
                roll = pd.Series(a.astype(int)).rolling(w).sum().dropna()
                row[f"windows_{w}"] = len(roll)
                row[f"full_windows_{w}_share"] = (roll == w).mean()
                row[f"windows_{w}_ge80_share"] = (roll >= 0.8 * w).mean()
            else:
                row[f"windows_{w}"], row[f"full_windows_{w}_share"], row[f"windows_{w}_ge80_share"] = 0, np.nan, np.nan
        use_rows.append(row)
    use = pd.DataFrame(use_rows).round(4)
    save_table(use, OUT / "usable_data_by_symbol.csv")
    low = use[use["active_share"] < 0.8]
    add("S3.06", "symbols with < 80% active days on their tradable calendar", "WARN" if len(low) else "PASS", len(low),
        "; ".join(f"{r.symbol} {r.active_share:.0%} (first active {r.first_active_date})" for r in low.itertuples()))
    short_hist = use[use["longest_active_streak"] < 60]
    add("S3.07", "symbols whose longest active streak is shorter than 60 days", "WARN" if len(short_hist) else "PASS",
        len(short_hist), "; ".join(f"{r.symbol}={r.longest_active_streak}" for r in short_hist.itertuples()))

    # ============ gaps ===========================================================================
    gaps = status[status["status"] == "NO_RECORD"].copy()
    ctx = []
    for r in gaps.itertuples():
        s = status[status["symbol"] == r.symbol].reset_index(drop=True)
        i = s.index[s["date"] == r.date][0]
        ctx.append({"symbol": r.symbol, "date": r.date.date(),
                    "prev_status": s.loc[i - 1, "status"] if i > 0 else None,
                    "next_status": s.loc[i + 1, "status"] if i + 1 < len(s) else None})
    ctx = pd.DataFrame(ctx)
    save_table(ctx, OUT / "gap_context.csv")
    add("S3.08", "missing records (NO_RECORD) outside market closures", "WARN" if len(ctx) else "PASS", len(ctx),
        f"{ctx['date'].astype(str).value_counts().to_dict()}; neighbours: prev={ctx['prev_status'].value_counts().to_dict()}, "
        f"next={ctx['next_status'].value_counts().to_dict()}")
    first_act = use.set_index("symbol")["first_active_date"]
    late = use[pd.to_datetime(use["first_active_date"]) - pd.to_datetime(use["first_record"]) > pd.Timedelta(days=30)]
    add("S3.09", "symbols whose first real trade comes >30 days after their first record", "WARN" if len(late) else "PASS",
        len(late), "; ".join(f"{r.symbol}: record {r.first_record} -> first trade {r.first_active_date}" for r in late.itertuples()))

    # ============ is volume adjusted for capital increases? ====================================
    vrows = []
    for r in df[df["adj_event"] & (df["adj_ratio"] < 0.9)].itertuples():
        a = df[(df["symbol"] == r.symbol) & (~df["is_halted"])]
        before, after = a[a["date"] < r.date].tail(20)["volume"], a[a["date"] >= r.date].head(20)["volume"]
        if len(before) >= 10 and len(after) >= 10:
            vrows.append({"symbol": r.symbol, "date": r.date.date(), "adj_ratio": r.adj_ratio,
                          "expected_volume_multiple": 1 / r.adj_ratio,
                          "observed_median_volume_multiple": after.median() / before.median(),
                          "median_value_multiple": a[a["date"] >= r.date].head(20)["value"].median()
                          / a[a["date"] < r.date].tail(20)["value"].median()})
    vol = pd.DataFrame(vrows)
    save_table(vol.round(4), OUT / "volume_vs_capital_increase.csv")
    rho = np.corrcoef(np.log(vol["expected_volume_multiple"]), np.log(vol["observed_median_volume_multiple"]))[0, 1]
    add("S3.10", "raw volume jumps after capital increases (volume is NOT adjusted like prices)", "WARN", len(vol),
        f"{len(vol)} capital-increase events: corr(log expected 1/ratio, log observed volume multiple) = {rho:.2f}; "
        f"median observed/expected = {(vol['observed_median_volume_multiple'] / vol['expected_volume_multiple']).median():.2f}; "
        f"median traded-value multiple = {vol['median_value_multiple'].median():.2f}")

    # ============ figures =======================================================================
    fig, ax = plt.subplots(1, 2, figsize=(13, 4.2))
    order = ["1", "2-3", "4-10", ">10"]
    sym_runs["len_bin_symbol_days"].value_counts().reindex(order).plot.bar(ax=ax[0], color="#2a9d8f", rot=0)
    ax[0].set_title("Symbol-specific halt runs by length (symbol-days)")
    ax[0].set_ylabel("# runs")
    sym_runs.groupby("len_bin_symbol_days")["n_symbol_days"].sum().reindex(order).plot.bar(ax=ax[1], color="#e76f51", rot=0)
    ax[1].set_title("Halted symbol-days contributed by each length class")
    ax[1].set_ylabel("symbol-days")
    fig.tight_layout()
    fig.savefig(OUT / "halt_run_lengths.png", dpi=130)
    plt.close(fig)

    mm = df[df["is_halted"]].assign(month=lambda d: d["date"].dt.to_period("M").astype(str))
    tab = mm.groupby(["month", "in_closure"]).size().unstack(fill_value=0).rename(
        columns={False: "symbol-specific halt", True: "market-wide closure"})
    fig, ax = plt.subplots(figsize=(13, 4.2))
    tab.plot.bar(stacked=True, ax=ax, color=["#2a9d8f", "#e76f51"][: tab.shape[1]], width=0.85)
    ax.set_title("Halted rows per month")
    ax.set_xlabel("")
    fig.tight_layout()
    fig.savefig(OUT / "halted_rows_by_month.png", dpi=130)
    plt.close(fig)

    res = pd.DataFrame(checks)
    save_table(res, OUT / "checks_summary.csv")
    summary = {"checks": len(res), **res["status"].value_counts().to_dict(), "status_cells": st_counts}
    (OUT / "summary.json").write_text(json.dumps(summary, indent=2, ensure_ascii=False))
    pd.set_option("display.width", 250, "display.max_colwidth", 400)
    print(res[["id", "status", "n_issues", "detail"]].to_string(index=False))
    print(reopen.to_string(index=False))


if __name__ == "__main__":
    main()
