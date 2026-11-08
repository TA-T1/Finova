"""Step 0 - load the data, build a column dictionary and reconcile the three input files.

Outputs (outputs/step0_overview/):
  columns_dictionary.csv   dtype / null / min / max / group / (inferred) description per column
  symbol_overview.csv      per-symbol coverage, expected vs actual rows, halted share
  market_calendar.csv      observed market calendar (union of all symbol dates)
  reconciliation.csv       every number in quality_report.md and underlyings.csv re-computed from the daily file
  coverage_heatmap.png     symbols x market-days status map (record / halted / no record)
  summary.json             headline numbers
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
from matplotlib.colors import ListedColormap

from common import (REPORT_MD, fa_label, load_daily, load_meta, market_calendar, out_dir,
                    parse_md_table, save_table)

OUT = out_dir("step0_overview")

# ----------------------------------------------------------------------------------------------
# Column dictionary. Descriptions are INFERRED from names/values - confirm with the data owner.
# ----------------------------------------------------------------------------------------------
COLUMN_INFO = {
    "symbol": ("identity", "Ticker (Persian text, Arabic ي/ك variants present)"),
    "name": ("identity", "Instrument name - identical to symbol in this file"),
    "ins_code": ("identity", "Exchange instrument code (unique id per symbol)"),
    "date": ("date", "Gregorian trading date"),
    "date_jalali": ("date", "Jalali (Solar Hijri) trading date, YYYY-MM-DD"),
    "open": ("raw_price", "First trade price of the day (0 on halted days)"),
    "high": ("raw_price", "Highest trade price (0 on halted days)"),
    "low": ("raw_price", "Lowest trade price (0 on halted days)"),
    "last_price": ("raw_price", "Last trade price"),
    "final_price": ("raw_price", "Closing/settlement price (volume-weighted on TSE; not guaranteed inside [low, high])"),
    "prev_final": ("raw_price", "Reference price = previous closing price as published by the exchange"),
    "volume": ("activity", "Traded volume (shares/units)"),
    "value": ("activity", "Traded value (Rial)"),
    "trade_count": ("activity", "Number of trades"),
    "adj_factor": ("adjustment", "Cumulative backward adjustment factor (1.0 for the latest period)"),
    "adj_open": ("adj_price", "open * adj_factor"),
    "adj_high": ("adj_price", "high * adj_factor"),
    "adj_low": ("adj_price", "low * adj_factor"),
    "adj_last": ("adj_price", "last_price * adj_factor"),
    "adj_final": ("adj_price", "final_price * adj_factor"),
    "adj_event": ("adjustment", "True on days where prev_final != previous day's final_price"),
    "adj_ratio": ("adjustment", "prev_final / previous final_price on event days, else 1.0"),
    "event_type": ("adjustment", "Heuristic label from adj_ratio (capital_increase vs dividend_or_adjustment)"),
    "ind_buy_vol": ("flow_individual", "Retail (real persons) buy volume"),
    "ind_sell_vol": ("flow_individual", "Retail sell volume"),
    "ind_buy_val": ("flow_individual", "Retail buy value"),
    "ind_sell_val": ("flow_individual", "Retail sell value"),
    "ind_buy_count": ("flow_individual", "Number of retail buyers"),
    "ind_sell_count": ("flow_individual", "Number of retail sellers"),
    "corp_buy_vol": ("flow_corporate", "Institutional (legal entities) buy volume"),
    "corp_sell_vol": ("flow_corporate", "Institutional sell volume"),
    "corp_buy_val": ("flow_corporate", "Institutional buy value"),
    "corp_sell_val": ("flow_corporate", "Institutional sell value"),
    "corp_buy_count": ("flow_corporate", "Number of institutional buyers"),
    "corp_sell_count": ("flow_corporate", "Number of institutional sellers"),
    "ind_net_value": ("derived", "ind_buy_val - ind_sell_val"),
    "corp_net_value": ("derived", "corp_buy_val - corp_sell_val"),
    "is_halted": ("flag", "True when a record exists but volume==0 or trade_count==0"),
    "gap_days_before": ("flag", "Market days missing for this symbol since its previous record"),
}


def column_dictionary(df: pd.DataFrame) -> pd.DataFrame:
    rows = []
    for col in df.columns:
        s = df[col]
        num = pd.api.types.is_numeric_dtype(s) and not pd.api.types.is_bool_dtype(s)
        group, desc = COLUMN_INFO.get(col, ("unknown", ""))
        rows.append({
            "column": col, "group": group, "dtype": str(s.dtype),
            "n_null": int(s.isna().sum()), "pct_null": round(100 * s.isna().mean(), 2),
            "n_unique": int(s.nunique(dropna=True)),
            "min": s.min() if num else None, "max": s.max() if num else None,
            "example": s.dropna().iloc[0] if s.notna().any() else None,
            "description_inferred": desc,
        })
    return pd.DataFrame(rows)


def gap_stats(df: pd.DataFrame, calendar: pd.DatetimeIndex) -> pd.DataFrame:
    """Per symbol: missing market days between its first and last record."""
    pos = pd.Series(np.arange(len(calendar)), index=calendar)
    rows = []
    for sym, g in df.groupby("symbol", sort=True):
        p = pos.loc[g["date"]].to_numpy()
        missing = np.diff(p) - 1
        missing = missing[missing > 0]
        rows.append({"symbol": sym, "gap_occurrences": int(len(missing)),
                     "total_missing_days": int(missing.sum()),
                     "largest_gap": int(missing.max()) if len(missing) else 0})
    return pd.DataFrame(rows)


def symbol_overview(df: pd.DataFrame, calendar: pd.DatetimeIndex) -> pd.DataFrame:
    pos = pd.Series(np.arange(len(calendar)), index=calendar)
    rows = []
    for sym, g in df.groupby("symbol", sort=True):
        p = pos.loc[g["date"]].to_numpy()
        expected = int(p.max() - p.min() + 1)
        rows.append({
            "symbol": sym, "ins_code": int(g["ins_code"].iloc[0]),
            "first_date": g["date"].min().date(), "last_date": g["date"].max().date(),
            "rows": len(g), "expected_rows_from_first_date": expected,
            "missing_market_days": expected - len(g),
            "halted_days": int(g["is_halted"].sum()),
            "halted_pct": round(100 * g["is_halted"].mean(), 1),
            "active_days": int((~g["is_halted"]).sum()),
            "adj_events": int(g["adj_event"].sum()),
            "starts_after_window_start": bool(g["date"].min() > calendar[0]),
        })
    return pd.DataFrame(rows).sort_values(["first_date", "symbol"]).reset_index(drop=True)


def reconcile(df, meta, calendar, report_md) -> pd.DataFrame:
    recs = []

    def add(section, item, reported, computed):
        recs.append({"section": section, "item": item, "reported": reported, "recomputed": computed,
                     "match": str(reported) == str(computed)})

    # --- headline numbers -------------------------------------------------------------------
    add("headline", "symbols", 27, df["symbol"].nunique())
    add("headline", "total_rows", 17368, len(df))
    add("headline", "market_days_union", 718, len(calendar))
    add("headline", "first_market_day", "2023-11-08", calendar[0].date())
    add("headline", "last_market_day", "2026-11-07", calendar[-1].date())
    add("headline", "halted_rows_total", 2452, int(df["is_halted"].sum()))
    add("headline", "adj_events_total", 69, int(df["adj_event"].sum()))

    # --- halted days per symbol -------------------------------------------------------------
    rep = parse_md_table(report_md, r"Halted")
    rep["halted days"] = rep["halted days"].astype(int)
    comp = df.groupby("symbol")["is_halted"].sum()
    for _, r in rep.iterrows():
        add("halted_per_symbol", r["symbol"], r["halted days"], int(comp.get(r["symbol"], -1)))

    # --- adjustment events (symbol, date, ratio, type) --------------------------------------
    rep = parse_md_table(report_md, r"Price-adjustment")
    ev = df.loc[df["adj_event"], ["symbol", "date", "adj_ratio", "event_type"]].copy()
    ev["key"] = ev["symbol"] + "|" + ev["date"].dt.strftime("%Y-%m-%d") + "|" + ev["adj_ratio"].round(6).astype(str) + "|" + ev["event_type"]
    rep["key"] = rep["symbol"] + "|" + rep["date"] + "|" + rep["ratio"].astype(float).round(6).astype(str) + "|" + rep["event_type"]
    only_rep, only_data = set(rep["key"]) - set(ev["key"]), set(ev["key"]) - set(rep["key"])
    add("events", "events_listed_in_report", len(rep), len(ev))
    add("events", "events_in_report_but_not_in_data", 0, len(only_rep))
    add("events", "events_in_data_but_not_in_report", 0, len(only_data))

    # --- gaps ---------------------------------------------------------------------------------
    rep = parse_md_table(report_md, r"Gaps")
    comp = gap_stats(df, calendar).set_index("symbol")
    add("gaps", "gap_occurrences_total", 9, int(comp["gap_occurrences"].sum()))
    for _, r in rep.iterrows():
        s = r["symbol"]
        add("gaps", f"{s} occurrences", int(r["gap occurrences"]), int(comp.loc[s, "gap_occurrences"]))
        add("gaps", f"{s} total_missing_days", int(r["total missing days"]), int(comp.loc[s, "total_missing_days"]))
    others = comp.drop(index=list(rep["symbol"]))["gap_occurrences"].sum()
    add("gaps", "gaps_on_symbols_not_in_report", 0, int(others))

    # --- coverage: report table vs daily file vs underlyings.csv --------------------------------
    rep = parse_md_table(report_md, r"Coverage").set_index("symbol")
    per = df.groupby("symbol").agg(ins_code=("ins_code", "first"), first_date=("date", "min"),
                                   last_date=("date", "max"), rows=("date", "size"))
    meta = meta.set_index("symbol")
    add("coverage", "symbols_in_meta_vs_daily", sorted(meta.index) == sorted(per.index), True)
    for s in per.index:
        for src, ref in (("report", rep), ("underlyings.csv", meta)):
            if s not in ref.index:
                add(f"coverage_{src}", f"{s} present", True, False)
                continue
            add(f"coverage_{src}", f"{s} ins_code", int(ref.loc[s, "ins_code"]), int(per.loc[s, "ins_code"]))
            add(f"coverage_{src}", f"{s} rows", int(ref.loc[s, "rows"]), int(per.loc[s, "rows"]))
            add(f"coverage_{src}", f"{s} first_date", str(pd.to_datetime(ref.loc[s, "first_date"]).date()), per.loc[s, "first_date"].date())
            add(f"coverage_{src}", f"{s} last_date", str(pd.to_datetime(ref.loc[s, "last_date"]).date()), per.loc[s, "last_date"].date())
    return pd.DataFrame(recs)


def coverage_figure(df: pd.DataFrame, calendar: pd.DatetimeIndex, overview: pd.DataFrame) -> None:
    pos = pd.Series(np.arange(len(calendar)), index=calendar)
    order = list(overview["symbol"])
    mat = np.zeros((len(order), len(calendar)), dtype=int)  # 0 = no record
    for i, sym in enumerate(order):
        g = df[df["symbol"] == sym]
        mat[i, pos.loc[g["date"]].to_numpy()] = np.where(g["is_halted"], 2, 1)

    fig, (ax, ax2) = plt.subplots(2, 1, figsize=(14, 9), sharex=True,
                                  gridspec_kw={"height_ratios": [4, 1]})
    cmap = ListedColormap(["#e5e5e5", "#2a9d8f", "#e76f51"])
    ax.imshow(mat, aspect="auto", cmap=cmap, vmin=0, vmax=2, interpolation="nearest")
    ax.set_yticks(range(len(order)))
    ax.set_yticklabels([fa_label(s) for s in order], fontsize=8)
    ax.set_title("Coverage map: grey = no record, green = traded, orange = halted (record with volume 0)")
    ticks = np.linspace(0, len(calendar) - 1, 10).astype(int)
    ax2.set_xticks(ticks)
    ax2.set_xticklabels([calendar[t].strftime("%Y-%m") for t in ticks])
    ax2.plot((mat > 0).sum(axis=0), color="#264653", lw=1.2, label="symbols with a record")
    ax2.plot((mat == 1).sum(axis=0), color="#2a9d8f", lw=1.2, label="symbols that traded")
    ax2.set_ylabel("# symbols")
    ax2.legend(loc="lower right", fontsize=8)
    fig.tight_layout()
    fig.savefig(OUT / "coverage_heatmap.png", dpi=130)
    plt.close(fig)


def main() -> None:
    df, meta = load_daily(), load_meta()
    calendar = market_calendar(df)
    report_md = REPORT_MD.read_text(encoding="utf-8")

    save_table(column_dictionary(df), OUT / "columns_dictionary.csv")
    overview = symbol_overview(df, calendar)
    save_table(overview, OUT / "symbol_overview.csv")

    cal = pd.DataFrame({"date": calendar})
    per_day = df.groupby("date").agg(symbols_with_record=("symbol", "size"),
                                     symbols_halted=("is_halted", "sum")).reset_index()
    cal = cal.merge(per_day, on="date")
    cal["symbols_traded"] = cal["symbols_with_record"] - cal["symbols_halted"]
    save_table(cal, OUT / "market_calendar.csv")

    rec = reconcile(df, meta, calendar, report_md)
    save_table(rec, OUT / "reconciliation.csv")
    coverage_figure(df, calendar, overview)

    summary = {
        "rows": len(df), "columns": df.shape[1], "symbols": int(df["symbol"].nunique()),
        "market_days": len(calendar), "first_day": str(calendar[0].date()), "last_day": str(calendar[-1].date()),
        "halted_rows": int(df["is_halted"].sum()), "adj_events": int(df["adj_event"].sum()),
        "reconciliation_checks": len(rec), "reconciliation_mismatches": int((~rec["match"]).sum()),
    }
    (OUT / "summary.json").write_text(json.dumps(summary, indent=2, ensure_ascii=False))
    print(json.dumps(summary, indent=2, ensure_ascii=False))
    if (~rec["match"]).any():
        print("\nMISMATCHES:\n", rec[~rec["match"]].to_string())


if __name__ == "__main__":
    main()
