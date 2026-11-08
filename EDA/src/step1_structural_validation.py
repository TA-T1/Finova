"""Step 1 - structural validation of underlyings_daily.csv.

Outputs (outputs/step1_structure/):
  checks_summary.csv            one row per check: PASS / WARN / FAIL / INFO + detail
  null_summary.csv              nulls per column, split by halted vs active rows
  symbol_normalization_map.csv  raw symbol -> normalized (Persian ی/ک) spelling
  weekday_counts.csv            rows per weekday (market week is Sat-Wed)
  gap_dates.csv                 market days on which a listed symbol has no record
  market_wide_closures.csv      periods where >=90% of symbols with a record are halted
  jalali_mismatches.csv         rows whose date_jalali disagrees with date (empty if none)
  summary.json
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import jdatetime
import numpy as np
import pandas as pd

from common import (DAILY_CSV, FLOW_COLS, MARKET_WEEKDAYS, load_daily, market_calendar,
                    normalize_fa, out_dir, save_table)

OUT = out_dir("step1_structure")
CLOSURE_SHARE = 0.90   # share of halted symbols that makes a day "market-wide closed"
MIN_SYMBOLS_FOR_CLOSURE = 10

checks: list[dict] = []


def add(check_id: str, name: str, status: str, n_issues: int | None, detail: str) -> None:
    checks.append({"id": check_id, "check": name, "status": status,
                   "n_issues": n_issues, "detail": detail})


def main() -> None:
    df = load_daily()
    raw = pd.read_csv(DAILY_CSV, encoding="utf-8-sig")
    calendar = market_calendar(df)
    pos = pd.Series(np.arange(len(calendar)), index=calendar)

    # S1.01 shape & columns ---------------------------------------------------------------------
    expected_cols = ["symbol", "name", "ins_code", "date", "date_jalali", "open", "high", "low",
                     "last_price", "final_price", "prev_final", "volume", "value", "trade_count",
                     "adj_factor", "adj_open", "adj_high", "adj_low", "adj_last", "adj_final",
                     "adj_event", "adj_ratio", "event_type", "ind_buy_vol", "ind_sell_vol",
                     "ind_buy_val", "ind_sell_val", "ind_buy_count", "ind_sell_count",
                     "corp_buy_vol", "corp_sell_vol", "corp_buy_val", "corp_sell_val",
                     "corp_buy_count", "corp_sell_count", "ind_net_value", "corp_net_value",
                     "is_halted", "gap_days_before"]
    miss, extra = set(expected_cols) - set(df.columns), set(df.columns) - set(expected_cols)
    add("S1.01", "column set equals the 39 documented columns",
        "PASS" if not miss and not extra else "FAIL", len(miss) + len(extra),
        f"rows={len(df)}, cols={df.shape[1]}, missing={sorted(miss)}, extra={sorted(extra)}")

    # S1.02 dtypes ------------------------------------------------------------------------------
    num_cols = [c for c in expected_cols if c not in
                {"symbol", "name", "date", "date_jalali", "event_type", "adj_event", "is_halted"}]
    bad = [c for c in num_cols if not pd.api.types.is_numeric_dtype(df[c])]
    bad += [c for c in ("adj_event", "is_halted") if not pd.api.types.is_bool_dtype(df[c])]
    add("S1.02", "numeric columns are numeric, flags are boolean", "PASS" if not bad else "FAIL",
        len(bad), f"bad={bad}; *_count columns are float only because of NaN on halted rows")

    # S1.03 nulls -------------------------------------------------------------------------------
    nulls = pd.DataFrame({
        "column": df.columns,
        "n_null": df.isna().sum().values,
        "pct_null": (100 * df.isna().mean()).round(2).values,
        "n_null_on_halted_rows": df[df["is_halted"]].isna().sum().values,
        "n_null_on_active_rows": df[~df["is_halted"]].isna().sum().values,
    })
    save_table(nulls[nulls["n_null"] > 0], OUT / "null_summary.csv")
    null_cols = set(nulls.loc[nulls["n_null"] > 0, "column"])
    unexpected = null_cols - set(FLOW_COLS) - {"event_type"}
    active_nulls = int(nulls.loc[~nulls["column"].isin(["event_type"]), "n_null_on_active_rows"].sum())
    halted_null_flow = int(df.loc[df["is_halted"], "ind_buy_vol"].isna().sum())
    halted_zero_flow = int((df["is_halted"] & df["ind_buy_vol"].notna()).sum())
    add("S1.03", "nulls only in flow columns on halted rows (+ event_type off-event)",
        "PASS" if not unexpected and active_nulls == 0 else "FAIL", active_nulls + len(unexpected),
        f"flow columns null on {halted_null_flow} of {int(df['is_halted'].sum())} halted rows; "
        f"the other {halted_zero_flow} halted rows carry 0 instead of NaN (inconsistent encoding); "
        f"nulls on active rows = {active_nulls}; unexpected null columns = {sorted(unexpected)}")
    ev_ok = bool((df["event_type"].notna() == df["adj_event"]).all())
    add("S1.03b", "event_type is filled exactly when adj_event is True", "PASS" if ev_ok else "FAIL",
        int((df["event_type"].notna() != df["adj_event"]).sum()), "")
    halted_def = (df["volume"].eq(0) | df["trade_count"].eq(0))
    add("S1.03c", "is_halted == (volume==0 or trade_count==0)",
        "PASS" if (halted_def == df["is_halted"]).all() else "FAIL",
        int((halted_def != df["is_halted"]).sum()),
        f"volume==0 and trade_count==0 coincide on {int((df['volume'].eq(0) == df['trade_count'].eq(0)).sum())} rows")

    # S1.04 duplicates --------------------------------------------------------------------------
    d1, d2, d3 = (int(df.duplicated(["symbol", "date"]).sum()),
                  int(df.duplicated(["ins_code", "date"]).sum()), int(df.duplicated().sum()))
    add("S1.04", "no duplicate (symbol,date) / (ins_code,date) / full rows",
        "PASS" if d1 + d2 + d3 == 0 else "FAIL", d1 + d2 + d3, f"symbol-date={d1}, code-date={d2}, full-row={d3}")

    # S1.05 identity ----------------------------------------------------------------------------
    one_to_one = df.groupby("symbol")["ins_code"].nunique().max() == 1 and df.groupby("ins_code")["symbol"].nunique().max() == 1
    add("S1.05", "symbol <-> ins_code is 1:1", "PASS" if one_to_one else "FAIL", 0 if one_to_one else 1,
        f"{df['symbol'].nunique()} symbols / {df['ins_code'].nunique()} codes")
    add("S1.05b", "name column adds no information (name == symbol)",
        "INFO" if (df["name"] == df["symbol"]).all() else "WARN", int((df["name"] != df["symbol"]).sum()),
        "safe to drop `name` downstream")

    # S1.06 Persian/Arabic spelling ---------------------------------------------------------------
    syms = pd.DataFrame({"symbol_raw": sorted(df["symbol"].unique())})
    syms["symbol_normalized"] = syms["symbol_raw"].map(normalize_fa)
    syms["changed"] = syms["symbol_raw"] != syms["symbol_normalized"]
    syms["has_space"] = syms["symbol_raw"].str.contains(" ")
    save_table(syms, OUT / "symbol_normalization_map.csv")
    collide = int(syms["symbol_normalized"].duplicated().sum())
    add("S1.06", "symbols use Arabic ي/ك: normalize before joining with the options file",
        "WARN" if syms["changed"].any() else "PASS", int(syms["changed"].sum()),
        f"{int(syms['changed'].sum())} symbols change after normalization, {collide} collisions; "
        f"{int(syms['has_space'].sum())} symbols contain a space")

    # S1.07 dates ---------------------------------------------------------------------------------
    sorted_ok = bool(raw.assign(date=pd.to_datetime(raw["date"])).groupby("symbol")["date"]
                     .apply(lambda s: s.is_monotonic_increasing).all())
    add("S1.07", "dates parse and the file is sorted by date within each symbol",
        "PASS" if sorted_ok else "WARN", 0 if sorted_ok else 1,
        f"{df['date'].min().date()} .. {df['date'].max().date()}")

    # S1.08 weekdays ------------------------------------------------------------------------------
    wd = df["date"].dt.day_name().value_counts().rename_axis("weekday").reset_index(name="rows")
    save_table(wd, OUT / "weekday_counts.csv")
    off = df[~df["date"].dt.weekday.isin(MARKET_WEEKDAYS)]
    add("S1.08", "all rows fall on Sat-Wed (Iranian market week)",
        "PASS" if off.empty else "WARN", len(off),
        f"{len(off)} rows on Thu/Fri; dates={sorted(off['date'].dt.strftime('%Y-%m-%d').unique())[:10]}")

    # S1.09 Jalali date ---------------------------------------------------------------------------
    uniq = pd.Series(df["date"].unique())
    jmap = {d: jdatetime.date.fromgregorian(date=d.date()).strftime("%Y-%m-%d") for d in uniq}
    bad_j = df[df["date"].map(jmap) != df["date_jalali"]][["symbol", "date", "date_jalali"]].copy()
    bad_j["expected_jalali"] = bad_j["date"].map(jmap)
    save_table(bad_j, OUT / "jalali_mismatches.csv")
    add("S1.09", "date_jalali agrees with date (jdatetime conversion)",
        "PASS" if bad_j.empty else "FAIL", len(bad_j), "")

    # S1.10 gap_days_before -----------------------------------------------------------------------
    p = df["date"].map(pos)
    recomputed = (p.groupby(df["symbol"]).diff() - 1).fillna(0).astype(int)
    mism = int((recomputed != df["gap_days_before"]).sum())
    add("S1.10", "gap_days_before equals missing market days since previous record",
        "PASS" if mism == 0 else "FAIL", mism, f"rows with gap>0: {int((df['gap_days_before'] > 0).sum())}")

    # S1.11 calendar gaps (listed symbol without a record) -----------------------------------------
    first = df.groupby("symbol")["date"].min()
    rows = []
    have = df.groupby("date")["symbol"].apply(set)
    for d in calendar:
        listed = set(first.index[first <= d])
        missing = sorted(listed - have.loc[d])
        if missing:
            rows.append({"date": d.date(), "n_listed": len(listed), "n_missing": len(missing),
                         "missing_symbols": " ".join(missing)})
    gap_dates = pd.DataFrame(rows)
    save_table(gap_dates, OUT / "gap_dates.csv")
    add("S1.11", "listed symbols without a record on a market day", "WARN" if len(gap_dates) else "PASS",
        int(gap_dates["n_missing"].sum()) if len(gap_dates) else 0,
        f"{len(gap_dates)} dates; see gap_dates.csv")

    # S1.12 market-wide closures ------------------------------------------------------------------
    per_day = df.groupby("date").agg(n_rec=("symbol", "size"), n_halted=("is_halted", "sum"))
    per_day["halted_share"] = per_day["n_halted"] / per_day["n_rec"]
    closed = per_day[(per_day["n_rec"] >= MIN_SYMBOLS_FOR_CLOSURE) & (per_day["halted_share"] >= CLOSURE_SHARE)].copy()
    closed["pos"] = closed.index.map(pos)
    closed["block"] = (closed["pos"].diff() != 1).cumsum()
    blocks = closed.reset_index().groupby("block").agg(
        start=("date", "min"), end=("date", "max"), market_days=("date", "size"),
        min_halted_share=("halted_share", "min")).reset_index(drop=True)
    save_table(blocks, OUT / "market_wide_closures.csv")
    closed_rows = int(closed["n_halted"].sum())
    total_halted = int(df["is_halted"].sum())
    # per-symbol split of halted days: market-wide closure vs symbol-specific halt
    in_closure = df["date"].isin(closed.index)
    split = pd.DataFrame({
        "halted_total": df[df["is_halted"]].groupby("symbol").size(),
        "halted_in_market_closure": df[df["is_halted"] & in_closure].groupby("symbol").size(),
    }).fillna(0).astype(int)
    split["halted_symbol_specific"] = split["halted_total"] - split["halted_in_market_closure"]
    split = split.reindex(sorted(df["symbol"].unique())).fillna(0).astype(int)
    split["rows"] = df.groupby("symbol").size()
    split["rows_excl_closure"] = split["rows"] - df[in_closure].groupby("symbol").size().reindex(split.index).fillna(0).astype(int)
    split["symbol_specific_halt_pct"] = (100 * split["halted_symbol_specific"] / split["rows_excl_closure"]).round(1)
    save_table(split.sort_values("halted_symbol_specific", ascending=False).reset_index(), OUT / "halted_breakdown_by_symbol.csv")
    add("S1.12", "market-wide closure days hidden inside is_halted",
        "WARN" if len(blocks) else "PASS", int(len(closed)),
        f"{len(blocks)} periods / {len(closed)} market days account for {closed_rows} of {total_halted} "
        f"halted rows ({100 * closed_rows / total_halted:.0f}%) -> halted != symbol-specific suspension")

    # S1.13 inf / negative ------------------------------------------------------------------------
    numeric = df.select_dtypes(include=[np.number])
    n_inf = int(np.isinf(numeric).sum().sum())
    nonneg = [c for c in numeric.columns if c not in {"ind_net_value", "corp_net_value"}]
    neg = (df[nonneg] < 0).sum()
    neg = neg[neg > 0]
    add("S1.13", "no inf values and no negative prices/volumes/counts",
        "PASS" if n_inf == 0 and neg.empty else "FAIL", n_inf + int(neg.sum()),
        f"inf={n_inf}, negative columns={neg.to_dict()}")

    # S1.14 redundant columns ----------------------------------------------------------------------
    act = df[~df["is_halted"]]
    redundant = bool(np.allclose(act["ind_net_value"], -act["corp_net_value"], rtol=1e-9, atol=1.0))
    add("S1.14", "redundant columns", "INFO", None,
        f"ind_net_value == -corp_net_value on active rows: {redundant}; name == symbol: True; "
        f"constant columns: {[c for c in df.columns if df[c].nunique(dropna=False) == 1]}")

    # S1.15 short histories -------------------------------------------------------------------------
    cnt = df.groupby("symbol").size()
    short = cnt[cnt < 500].sort_values()
    add("S1.15", "symbols with short history (< 500 rows)", "INFO", int(len(short)),
        "; ".join(f"{s}={n}" for s, n in short.items()))

    # ---- write -------------------------------------------------------------------------------
    res = pd.DataFrame(checks)
    save_table(res, OUT / "checks_summary.csv")
    summary = {"checks": len(res), **res["status"].value_counts().to_dict()}
    (OUT / "summary.json").write_text(json.dumps(summary, indent=2, ensure_ascii=False))
    pd.set_option("display.width", 250, "display.max_colwidth", 140)
    print(res[["id", "status", "n_issues", "detail"]].to_string(index=False))
    print(blocks.to_string(index=False))
    print(gap_dates.to_string(index=False))


if __name__ == "__main__":
    main()
