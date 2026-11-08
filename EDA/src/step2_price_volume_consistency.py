"""Step 2 - logical consistency of prices, volumes, adjustments and buyer/seller flows.

Outputs (outputs/step2_consistency/):
  checks_summary.csv           one row per check: PASS / WARN / FAIL / INFO + detail
  ohlc_violations.csv          rows breaking an OHLC ordering rule (with relative size)
  vwap_violations.csv          rows where value/volume is outside [low, high] (tolerance VWAP_TOL)
  prev_final_continuity.csv    rows where prev_final != previous record's final_price, classified
  adj_event_review.csv         one row per adjustment event with the numbers needed for step 4
  flow_inconsistencies.csv     rows where individual+corporate flows do not add up to totals
  return_thresholds.csv        how many daily moves exceed 5/6/7/8/10/15/30 %
  return_extremes_by_symbol.csv
  large_moves.csv              |final/prev_final-1| > JUMP_THRESHOLD, with an explanation class
  return_distribution.png, large_moves_by_class.png
  summary.json
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

from common import ADJ_PRICE_COLS, RAW_PRICE_COLS, load_daily, out_dir, save_table

OUT = out_dir("step2_consistency")
VWAP_TOL = 0.001        # relative tolerance for value/volume vs [low, high]
ADJ_TOL = 1e-4          # relative tolerance for adj_* == raw * adj_factor
FLOW_ABS_TOL = 1.0      # absolute tolerance (units / Rial) for flow identities
JUMP_THRESHOLD = 0.06   # |final/prev_final - 1| above this is listed in large_moves.csv

checks: list[dict] = []


def add(check_id: str, name: str, status: str, n_issues: int | None, detail: str) -> None:
    checks.append({"id": check_id, "check": name, "status": status, "n_issues": n_issues, "detail": detail})


def main() -> None:
    df = load_daily()
    g = df.groupby("symbol", sort=False)
    df["prev_row_final"] = g["final_price"].shift()
    df["prev_row_halted"] = g["is_halted"].shift()
    df["prev_row_adj_final"] = g["adj_final"].shift()
    df["prev_row_adj_factor"] = g["adj_factor"].shift()
    active = df[~df["is_halted"]].copy()
    halted = df[df["is_halted"]].copy()

    # ================= A. price positivity and halted-row encoding ======================================
    nonpos = (active[RAW_PRICE_COLS + ["prev_final"]] <= 0).sum()
    nonpos = nonpos[nonpos > 0]
    add("S2.01", "active rows: all prices and prev_final > 0", "PASS" if nonpos.empty else "FAIL",
        int(nonpos.sum()), f"{nonpos.to_dict()}")
    zero_hl = int(((halted["high"] == 0) & (halted["low"] == 0)).sum())
    open_nonzero = int((halted["open"] != 0).sum())
    add("S2.02", "halted rows: high/low are 0 (not NaN), open is 0", "WARN", open_nonzero,
        f"high==low==0 on {zero_hl}/{len(halted)} halted rows; open != 0 on {open_nonzero} halted rows "
        f"(stale value). NEVER compute returns/ranges from open/high/low on halted rows")
    carry = (halted["final_price"] == halted["prev_final"]).mean()
    add("S2.03", "halted rows: last_price/final_price carried forward and > 0",
        "PASS" if (halted[["last_price", "final_price"]] > 0).all().all() else "FAIL",
        int((halted[["last_price", "final_price"]] <= 0).any(axis=1).sum()),
        f"final_price == prev_final on {100 * carry:.1f}% of halted rows")

    # ================= B. OHLC ordering ==============================================================
    rules = {
        "high<low": active["high"] < active["low"],
        "open>high": active["open"] > active["high"],
        "open<low": active["open"] < active["low"],
        "last>high": active["last_price"] > active["high"],
        "last<low": active["last_price"] < active["low"],
        "final>high": active["final_price"] > active["high"],
        "final<low": active["final_price"] < active["low"],
    }
    parts = []
    for rule, mask in rules.items():
        if not mask.any():
            continue
        sub = active[mask][["symbol", "date", "open", "high", "low", "last_price", "final_price",
                            "prev_final", "volume", "adj_event"]].copy()
        sub["rule"] = rule
        if rule.startswith("final>"):
            sub["rel_violation"] = sub["final_price"] / sub["high"] - 1
        elif rule.startswith("final<"):
            sub["rel_violation"] = sub["low"] / sub["final_price"] - 1
        elif rule == "open>high":
            sub["rel_violation"] = sub["open"] / sub["high"] - 1
        elif rule == "open<low":
            sub["rel_violation"] = sub["low"] / sub["open"] - 1
        else:
            sub["rel_violation"] = np.nan
        parts.append(sub)
    viol = pd.concat(parts) if parts else pd.DataFrame()
    save_table(viol, OUT / "ohlc_violations.csv")
    counts = {r: int(m.sum()) for r, m in rules.items()}
    hard = {r: n for r, n in counts.items() if not r.startswith("final")}
    hard_syms = sorted(viol[viol["rule"].isin(list(hard))]["symbol"].unique()) if len(viol) else []
    add("S2.04", "OHLC order: high>=low, open/last inside [low, high]",
        "PASS" if sum(hard.values()) == 0 else "FAIL", sum(hard.values()),
        f"{hard}; symbols affected: {hard_syms}; max overshoot "
        f"{100 * viol[viol['rule'].isin(['open>high', 'open<low'])]['rel_violation'].max():.1f}%")
    n_final = counts["final>high"] + counts["final<low"]
    fv = viol[viol["rule"].isin(["final>high", "final<low"])]
    add("S2.05", "final_price lies inside [low, high]", "WARN" if n_final else "PASS", n_final,
        f"{counts['final>high']} above high, {counts['final<low']} below low "
        f"({100 * n_final / len(active):.1f}% of active rows); max overshoot {100 * fv['rel_violation'].max():.2f}% "
        f"; {int(fv['symbol'].nunique())} symbols")

    # ================= C. value / volume (VWAP) =====================================================
    active["vwap"] = active["value"] / active["volume"]
    out_rng = (active["vwap"] < active["low"] * (1 - VWAP_TOL)) | (active["vwap"] > active["high"] * (1 + VWAP_TOL))
    save_table(active[out_rng][["symbol", "date", "low", "high", "vwap", "final_price", "volume", "value"]],
               OUT / "vwap_violations.csv")
    add("S2.06", f"value/volume (VWAP) lies inside [low, high] (tol {VWAP_TOL:.1%})",
        "PASS" if not out_rng.any() else "FAIL", int(out_rng.sum()), "")
    fin_vs_vwap = (active["final_price"] / active["vwap"] - 1).abs()
    in_rng_for_final_viol = (~out_rng[active.index.isin(fv.index.union(fv.index))]).mean()
    ofr = active[(active["final_price"] > active["high"]) | (active["final_price"] < active["low"])]
    add("S2.07", "final_price is (almost) the day's VWAP", "INFO", None,
        f"median |final/vwap-1| = {fin_vs_vwap.median():.2e}, p99 = {fin_vs_vwap.quantile(.99):.2%}; "
        f"on the {len(ofr)} rows where final is outside [low, high], VWAP is inside the range on "
        f"{100 * (~out_rng[ofr.index]).mean():.0f}% -> final_price is NOT a last-trade price")

    # ================= D. prev_final continuity ====================================================
    has_prev = df["prev_row_final"].notna()
    diff = (df["prev_final"] - df["prev_row_final"]).abs() > 1e-9
    cont = df[has_prev & diff].copy()
    cont["rel_diff"] = cont["prev_final"] / cont["prev_row_final"] - 1
    cont["class"] = np.where(cont["adj_event"], "explained_by_adj_event",
                             np.where(cont["rel_diff"].abs() < 0.01, "UNEXPLAINED_small(<1%)", "UNEXPLAINED_large"))
    save_table(cont[["symbol", "date", "prev_final", "prev_row_final", "rel_diff", "adj_event", "adj_ratio",
                     "is_halted", "class"]], OUT / "prev_final_continuity.csv")
    unexpl = cont[cont["class"].str.startswith("UNEXPLAINED")]
    add("S2.08", "prev_final == previous record's final_price (except on adj_event days)",
        "PASS" if unexpl.empty else "WARN", len(unexpl),
        f"{int(has_prev.sum())} comparable rows, {len(cont)} differ, {int(cont['adj_event'].sum())} explained by adj_event; "
        f"unexplained: " + "; ".join(f"{r.symbol} {r.date.date()} ({r.rel_diff:+.2%})" for r in unexpl.itertuples()))
    ev = df[df["adj_event"]]
    ratio_dev = (ev["adj_ratio"] - ev["prev_final"] / ev["prev_row_final"]).abs().max()
    add("S2.09", "adj_ratio == prev_final / previous final_price on event days",
        "PASS" if ratio_dev < 1e-5 else "FAIL", int(((ev["adj_ratio"] - ev["prev_final"] / ev["prev_row_final"]).abs() > 1e-5).sum()),
        f"max abs deviation {ratio_dev:.1e}")

    # ================= E. adjusted-price formulas =================================================
    worst = {}
    for raw_c, adj_c in zip(RAW_PRICE_COLS, ADJ_PRICE_COLS):
        m = active[adj_c] / (active[raw_c] * active["adj_factor"]) - 1
        worst[adj_c] = float(m.abs().max())
    add("S2.10", f"adj_* == raw * adj_factor on active rows (tol {ADJ_TOL:g})",
        "PASS" if max(worst.values()) < ADJ_TOL else "FAIL",
        int(sum((active[a] / (active[r] * active['adj_factor']) - 1).abs().gt(ADJ_TOL).sum()
                for r, a in zip(RAW_PRICE_COLS, ADJ_PRICE_COLS))),
        "max rel dev: " + ", ".join(f"{k}={v:.1e}" for k, v in worst.items()))
    h_adj = halted[["adj_open", "adj_high", "adj_low"]]
    add("S2.11", "halted rows: adj_open/high/low are 0 like the raw columns",
        "PASS" if (h_adj.sum(axis=1) == 0).all() else "WARN", int((h_adj.sum(axis=1) != 0).sum()), "")
    step = (df["adj_factor"] / df["prev_row_adj_factor"])
    moved_off_event = int(((step - 1).abs() > 1e-6)[~df["adj_event"] & df["prev_row_adj_factor"].notna()].sum())
    step_dev = (step * df["adj_ratio"] - 1)[df["adj_event"]].abs()
    add("S2.12", "adj_factor changes only on event days, by exactly 1/adj_ratio",
        "PASS" if moved_off_event == 0 and step_dev.max() < 1e-3 else "FAIL", moved_off_event,
        f"off-event changes={moved_off_event}; max |factor_step*ratio-1| on events = {step_dev.max():.1e}")
    last_f = df.groupby("symbol")["adj_factor"].last()
    add("S2.13", "adj_factor of the latest row is 1.0 (backward-adjusted series)",
        "PASS" if np.allclose(last_f, 1.0) else "WARN", int((~np.isclose(last_f, 1.0)).sum()),
        f"non-1 symbols: {list(last_f[~np.isclose(last_f, 1.0)].index)}")
    thr_ok = ((ev["adj_ratio"] < 0.9) == (ev["event_type"] == "capital_increase")).all()
    add("S2.14", "event_type follows the heuristic ratio < 0.9 -> capital_increase", "PASS" if thr_ok else "WARN",
        int(((ev["adj_ratio"] < 0.9) != (ev["event_type"] == "capital_increase")).sum()),
        f"capital_increase={int((ev['event_type'] == 'capital_increase').sum())}, "
        f"dividend_or_adjustment={int((ev['event_type'] == 'dividend_or_adjustment').sum())}; "
        f"{int((ev['adj_ratio'] > 1).sum())} events have ratio > 1 (price went UP at reference)")

    # adjusted-return continuity at event days (input for step 4)
    ev = df[df["adj_event"]].copy()
    ev["ret_vs_reference"] = ev["final_price"] / ev["prev_final"] - 1          # true move on the exchange basis
    ev["raw_ret_unadjusted"] = ev["final_price"] / ev["prev_row_final"] - 1    # what a naive close-to-close shows
    ev["adj_ret"] = ev["adj_final"] / ev["prev_row_adj_final"] - 1
    ev["adj_minus_reference"] = ev["adj_ret"] - ev["ret_vs_reference"]
    ev["factor_before"], ev["factor_after"] = ev["prev_row_adj_factor"], ev["adj_factor"]
    # events of the same symbol within 5 market rows of each other
    ev["row_pos"] = df.groupby("symbol").cumcount()[ev.index]
    ev["cluster_with_other_event"] = ev.groupby("symbol")["row_pos"].transform(
        lambda s: s.diff().abs().lt(6) | s.diff(-1).abs().lt(6))
    reasons = []
    for r in ev.itertuples():
        why = []
        if r.adj_ratio > 1:
            why.append("ratio>1")
        if r.cluster_with_other_event:
            why.append("clustered_events")
        if r.adj_ratio < 0.2:
            why.append("very_small_ratio")
        if abs(r.ret_vs_reference) > 0.10:
            why.append("big_move_on_event_day")
        reasons.append(",".join(why))
    ev["review_flags"] = reasons
    cols = ["symbol", "date", "event_type", "adj_ratio", "prev_final", "prev_row_final", "final_price",
            "ret_vs_reference", "raw_ret_unadjusted", "adj_ret", "adj_minus_reference",
            "factor_before", "factor_after", "prev_row_halted", "is_halted", "review_flags"]
    save_table(ev[cols], OUT / "adj_event_review.csv")
    max_dev = ev["adj_minus_reference"].abs().max()
    add("S2.15", "adjusted return on event days == final/prev_final - 1 (no artificial jump)",
        "PASS" if max_dev < 1e-3 else "WARN", int((ev["adj_minus_reference"].abs() > 1e-3).sum()),
        f"max deviation {max_dev:.1e}; naive close-to-close return on these days ranges "
        f"{ev['raw_ret_unadjusted'].min():+.1%} .. {ev['raw_ret_unadjusted'].max():+.1%} (must NOT be used); "
        f"{int((ev['review_flags'] != '').sum())}/{len(ev)} events carry a review flag (see adj_event_review.csv)")

    # ================= F. individual / corporate flows =============================================
    fl = active.copy()
    checks_f = {
        "buy_vol != volume": (fl["ind_buy_vol"] + fl["corp_buy_vol"] - fl["volume"]).abs() > FLOW_ABS_TOL,
        "sell_vol != volume": (fl["ind_sell_vol"] + fl["corp_sell_vol"] - fl["volume"]).abs() > FLOW_ABS_TOL,
        "buy_val != value": (fl["ind_buy_val"] + fl["corp_buy_val"] - fl["value"]).abs() > max(FLOW_ABS_TOL, 0) + fl["value"] * 1e-9,
        "sell_val != value": (fl["ind_sell_val"] + fl["corp_sell_val"] - fl["value"]).abs() > FLOW_ABS_TOL + fl["value"] * 1e-9,
        "ind_net != buy-sell": (fl["ind_net_value"] - (fl["ind_buy_val"] - fl["ind_sell_val"])).abs() > FLOW_ABS_TOL,
        "corp_net != buy-sell": (fl["corp_net_value"] - (fl["corp_buy_val"] - fl["corp_sell_val"])).abs() > FLOW_ABS_TOL,
        "ind_net+corp_net != 0": (fl["ind_net_value"] + fl["corp_net_value"]).abs() > FLOW_ABS_TOL + fl["value"] * 1e-9,
        "buyer count 0 but volume>0": (fl["ind_buy_count"] + fl["corp_buy_count"]) == 0,
        "seller count 0 but volume>0": (fl["ind_sell_count"] + fl["corp_sell_count"]) == 0,
    }
    frames = []
    for name, m in checks_f.items():
        if m.any():
            s = fl[m][["symbol", "date", "volume", "value", "ind_buy_vol", "corp_buy_vol", "ind_sell_vol",
                       "corp_sell_vol", "ind_buy_val", "corp_buy_val", "ind_sell_val", "corp_sell_val"]].copy()
            s["rule"] = name
            frames.append(s)
    flow_bad = pd.concat(frames) if frames else pd.DataFrame()
    save_table(flow_bad, OUT / "flow_inconsistencies.csv")
    nbad = {k: int(v.sum()) for k, v in checks_f.items() if v.any()}
    mv = checks_f["buy_vol != volume"]
    gap_buy = (fl["ind_buy_vol"] + fl["corp_buy_vol"] - fl["volume"])[mv]
    gap_sell = (fl["ind_sell_vol"] + fl["corp_sell_vol"] - fl["volume"])[mv]
    rel_gap = (gap_buy / fl.loc[mv, "volume"]).abs()
    add("S2.16", "individual + corporate flows add up to volume/value; net values match",
        "PASS" if not nbad else "WARN", sum(nbad.values()),
        "all identities hold" if not nbad else
        f"{nbad}; the {int(mv.sum())} affected rows ({100 * mv.mean():.2f}% of active rows) are off by the SAME amount on "
        f"buy and sell side ({bool(np.allclose(gap_buy, gap_sell))}); flows fall short of volume by median "
        f"{rel_gap.median():.3%}, max {rel_gap.max():.2%} of volume; symbols: "
        f"{fl.loc[mv, 'symbol'].value_counts().head(5).to_dict()}")
    zero_flow_halted = halted[halted["ind_buy_vol"].notna()]
    add("S2.17", "halted rows with flow columns = 0 instead of NaN", "INFO", len(zero_flow_halted),
        f"dates: {sorted(zero_flow_halted['date'].dt.strftime('%Y-%m-%d').unique())[:6]}...")

    # ================= G. daily moves ===============================================================
    active["ret"] = active["final_price"] / active["prev_final"] - 1   # exchange-reference basis, valid on event days too
    thr_rows = [{"threshold": t, "n_rows_above": int((active["ret"].abs() > t).sum()),
                 "n_non_event_rows_above": int(((active["ret"].abs() > t) & ~active["adj_event"]).sum())}
                for t in (0.05, 0.06, 0.07, 0.08, 0.10, 0.15, 0.30, 1.0)]
    save_table(pd.DataFrame(thr_rows), OUT / "return_thresholds.csv")
    ext = active.groupby("symbol")["ret"].agg(
        n_active_days="size", mean="mean", std="std", min="min", p01=lambda s: s.quantile(.01),
        p99=lambda s: s.quantile(.99), max="max").reset_index()
    ext["n_moves_above_threshold"] = ext["symbol"].map(
        active[active["ret"].abs() > JUMP_THRESHOLD].groupby("symbol").size()).fillna(0).astype(int)
    save_table(ext.round(5), OUT / "return_extremes_by_symbol.csv")

    # monthly band of "clean" moves (active, no event, previous record also active, no reference anomaly):
    # the allowed daily band is NOT constant over the 3 years, so one global threshold is misleading.
    clean = active[~active["adj_event"] & (active["prev_row_halted"] == False) & (active["ret"].abs() < 2)].copy()
    clean["month"] = clean["date"].dt.to_period("M").astype(str)
    band = clean.groupby("month")["ret"].agg(
        n_rows="size", max_abs=lambda s: s.abs().max(), p995_abs=lambda s: s.abs().quantile(.995),
        n_above_3pct=lambda s: int((s.abs() > 0.0305).sum()),
        n_above_4pct=lambda s: int((s.abs() > 0.0405).sum())).reset_index()
    save_table(band.round(5), OUT / "monthly_move_band.csv")
    wide_months = set(band.loc[band["p995_abs"] >= 0.045, "month"])

    big = active[active["ret"].abs() > JUMP_THRESHOLD].copy()
    day_share = (active["ret"].abs() > 0.05).groupby(active["date"]).mean()
    big["day_share_moving_5pct"] = big["date"].map(day_share)
    big["wide_band_month"] = big["date"].dt.to_period("M").astype(str).isin(wide_months)
    big["class"] = np.select(
        [big["ret"].abs() > 2.0, big["adj_event"], big["prev_row_halted"].fillna(False).astype(bool),
         big["day_share_moving_5pct"] >= 0.3, big["wide_band_month"]],
        ["reference_price_anomaly(>200%)", "adj_event_day", "reopen_after_halt", "market_wide_move",
         "wide_band_month"],
        default="UNEXPLAINED")
    save_table(big[["symbol", "date", "open", "high", "low", "final_price", "prev_final", "ret",
                    "adj_event", "prev_row_halted", "day_share_moving_5pct", "wide_band_month", "volume", "class"]],
               OUT / "large_moves.csv")
    m3 = band["max_abs"].le(0.0305).sum()
    m4 = band["max_abs"].between(0.0395, 0.0405).sum()
    add("S2.20", "allowed daily move band changes over time (do not use one global limit)", "INFO", None,
        f"months with max clean |ret| <= 3.0%: {int(m3)}; months with max = 4.0%: {int(m4)}; "
        f"months with wider moves: {len(band) - int(m3) - int(m4)} (see monthly_move_band.csv)")
    cls = big["class"].value_counts().to_dict()
    add("S2.18", f"daily moves above {JUMP_THRESHOLD:.0%} (final vs prev_final)", "WARN" if len(big) else "PASS", len(big),
        f"{cls}; typical band: median |ret|={active['ret'].abs().median():.2%}, p99={active['ret'].abs().quantile(.99):.2%}")
    anomalies = big[big["class"].str.startswith("reference")]
    add("S2.19", "reference-price anomalies (prev_final is a nominal/IPO price, e.g. 1000)", "WARN", len(anomalies),
        "; ".join(f"{r.symbol} {r.date.date()} prev_final={r.prev_final:.0f} final={r.final_price:.0f}"
                  for r in anomalies.itertuples()) + " -> drop these rows from return statistics")

    # ================= figures ====================================================================
    fig, ax = plt.subplots(1, 2, figsize=(14, 4.5))
    r = active.loc[~active["adj_event"], "ret"].clip(-0.12, 0.12)
    ax[0].hist(r, bins=120, color="#2a9d8f")
    ax[0].set_yscale("log")
    ax[0].set_title("Daily return final/prev_final - 1 (active, non-event days; clipped +-12%)")
    ax[0].set_xlabel("return")
    ax[0].set_ylabel("rows (log)")
    ax[1].hist((active["final_price"] / active["vwap"] - 1).clip(-0.02, 0.02), bins=120, color="#264653")
    ax[1].set_yscale("log")
    ax[1].set_title("final_price / (value/volume) - 1   (final price ~ VWAP)")
    fig.tight_layout()
    fig.savefig(OUT / "return_distribution.png", dpi=130)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(8, 4))
    pd.Series(cls).sort_values().plot.barh(ax=ax, color="#e76f51")
    ax.set_title(f"Moves above {JUMP_THRESHOLD:.0%} by explanation class")
    fig.tight_layout()
    fig.savefig(OUT / "large_moves_by_class.png", dpi=130)
    plt.close(fig)

    # ---- write ---------------------------------------------------------------------------------
    res = pd.DataFrame(checks)
    res = res.sort_values("id", key=lambda s: s.str.extract(r"S2\.(\d+)")[0].astype(int)).reset_index(drop=True)
    save_table(res, OUT / "checks_summary.csv")
    summary = {"checks": len(res), **res["status"].value_counts().to_dict(), "large_moves": cls}
    (OUT / "summary.json").write_text(json.dumps(summary, indent=2, ensure_ascii=False))
    pd.set_option("display.width", 250, "display.max_colwidth", 200)
    print(res[["id", "status", "n_issues", "detail"]].to_string(index=False))


if __name__ == "__main__":
    main()
