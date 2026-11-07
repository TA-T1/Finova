#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Finova data pipeline
====================
Collect multi-year daily data for every TSE/IFB stock & ETF that currently has
options (the "option underlyings"), run quality checks, and emit tidy CSVs.

Task covered (fa):
  داده روزانه چندساله سهام پایه دارای آپشن (قیمت تعدیل‌شده، حجم، جریان حقیقی/حقوقی)
  جمع و بررسی کیفیت می‌شود: شکاف‌ها، افزایش سرمایه، روزهای بسته.

Source: TSETMC public JSON API (https://cdn.tsetmc.com/api).

Outputs (in ./output):
  - underlyings.csv          master list of underlyings (symbol, name, ins_code, coverage)
  - underlyings_daily.csv    LONG/tidy daily panel: one row per (symbol, date)
  - quality_report.md        gaps / capital-increase events / halted (closed) days

Reproducible: re-running refreshes everything up to the latest trading day.
"""
import sys, os, time, json, datetime as dt
import requests
import pandas as pd

try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

# ----------------------------------------------------------------------------- config
YEARS_BACK   = 3
BASE         = "https://cdn.tsetmc.com/api"
HEADERS      = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
    "Accept": "application/json, text/plain, */*",
    "Referer": "https://cdn.tsetmc.com/",
}
OUT_DIR      = os.path.join(os.path.dirname(os.path.abspath(__file__)), "output")
REQ_PAUSE    = 0.4          # polite delay between requests (seconds)
RETRIES      = 4

TODAY        = dt.date.today()
START_DATE   = TODAY.replace(year=TODAY.year - YEARS_BACK)
CUTOFF       = int(START_DATE.strftime("%Y%m%d"))   # dEven is Gregorian yyyymmdd


# ----------------------------------------------------------------------------- helpers
def get_json(url):
    last = None
    for attempt in range(RETRIES):
        try:
            r = requests.get(url, headers=HEADERS, timeout=30)
            if r.status_code == 200:
                return r.json()
            last = f"HTTP {r.status_code}"
        except Exception as e:
            last = str(e)
        time.sleep(1.0 + attempt)
    raise RuntimeError(f"failed {url}: {last}")


def g2j(gy, gm, gd):
    """Gregorian -> Jalali (Solar Hijri). Standard div-based algorithm."""
    g_d_m = [0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334]
    if gy > 1600:
        jy = 979; gy -= 1600
    else:
        jy = 0;   gy -= 621
    gy2 = gy + 1 if gm > 2 else gy
    days = (365 * gy + (gy2 + 3) // 4 - (gy2 + 99) // 100 + (gy2 + 399) // 400
            - 80 + gd + g_d_m[gm - 1])
    jy += 33 * (days // 12053); days %= 12053
    jy += 4 * (days // 1461);   days %= 1461
    if days > 365:
        jy += (days - 1) // 365
        days = (days - 1) % 365
    if days < 186:
        jm = 1 + days // 31;        jd = 1 + days % 31
    else:
        jm = 7 + (days - 186) // 30; jd = 1 + (days - 186) % 30
    return jy, jm, jd


def deven_to_iso(deven):
    s = str(int(deven))
    return f"{s[0:4]}-{s[4:6]}-{s[6:8]}"


def deven_to_jalali(deven):
    s = str(int(deven))
    jy, jm, jd = g2j(int(s[0:4]), int(s[4:6]), int(s[6:8]))
    return f"{jy:04d}-{jm:02d}-{jd:02d}"


# ----------------------------------------------------------------------------- discovery
def discover_underlyings():
    """Distinct option underlyings from the live option market watch."""
    data = get_json(f"{BASE}/Instrument/GetInstrumentOptionMarketWatch/0")
    rows = data["instrumentOptMarketWatch"]
    seen = {}
    for o in rows:
        ins = str(o.get("uaInsCode") or "").strip()
        nm  = (o.get("lval30_UA") or "").strip()
        if ins and ins not in seen:
            seen[ins] = nm
    return seen  # {ins_code: symbol_name}


# ----------------------------------------------------------------------------- per-symbol fetch
def fetch_daily(ins):
    data = get_json(f"{BASE}/ClosingPrice/GetClosingPriceDailyList/{ins}/0")
    recs = data.get("closingPriceDaily", []) or []
    out = []
    for r in recs:
        d = int(r["dEven"])
        if d < CUTOFF:
            continue
        out.append({
            "deven": d,
            "open":        float(r.get("priceFirst") or 0),
            "high":        float(r.get("priceMax") or 0),
            "low":         float(r.get("priceMin") or 0),
            "last_price":  float(r.get("pDrCotVal") or 0),   # آخرین معامله
            "final_price": float(r.get("pClosing") or 0),    # قیمت پایانی (canonical close)
            "prev_final":  float(r.get("priceYesterday") or 0),
            "volume":      float(r.get("qTotTran5J") or 0),
            "value":       float(r.get("qTotCap") or 0),
            "trade_count": float(r.get("zTotTran") or 0),
        })
    return out


def fetch_clienttype(ins):
    data = get_json(f"{BASE}/ClientType/GetClientTypeHistory/{ins}")
    recs = data.get("clientType", []) or []
    out = {}
    for r in recs:
        d = int(r["recDate"])
        if d < CUTOFF:
            continue
        out[d] = {
            "ind_buy_vol":   float(r.get("buy_I_Volume") or 0),
            "ind_sell_vol":  float(r.get("sell_I_Volume") or 0),
            "ind_buy_val":   float(r.get("buy_I_Value") or 0),
            "ind_sell_val":  float(r.get("sell_I_Value") or 0),
            "ind_buy_count":  int(r.get("buy_I_Count") or 0),
            "ind_sell_count": int(r.get("sell_I_Count") or 0),
            "corp_buy_vol":  float(r.get("buy_N_Volume") or 0),
            "corp_sell_vol": float(r.get("sell_N_Volume") or 0),
            "corp_buy_val":  float(r.get("buy_N_Value") or 0),
            "corp_sell_val": float(r.get("sell_N_Value") or 0),
            "corp_buy_count":  int(r.get("buy_N_Count") or 0),
            "corp_sell_count": int(r.get("sell_N_Count") or 0),
        }
    return out


# ----------------------------------------------------------------------------- adjustment
def compute_adjusted(rows):
    """
    rows: list of daily dicts sorted ASCENDING by date.
    Back-adjust prices for capital increases / dividends using the ratio method:
    when prev_final[d] != final_price[d-1] there was an adjustment event; scale
    all earlier days by the cumulative ratio so the series has no artificial jump.
    Adds adj_factor, adj_* prices, adj_event, adj_ratio, event_type.
    """
    n = len(rows)
    for r in rows:
        r["adj_event"] = False
        r["adj_ratio"] = 1.0
        r["event_type"] = ""
    # detect events (on the later day of each boundary)
    for i in range(1, n):
        prev_close = rows[i - 1]["final_price"]
        ref        = rows[i]["prev_final"]
        if prev_close > 0 and ref > 0 and abs(ref - prev_close) / prev_close > 0.005:
            ratio = ref / prev_close
            rows[i]["adj_event"] = True
            rows[i]["adj_ratio"] = round(ratio, 6)
            # heuristic classification (documented in data_dictionary.md)
            rows[i]["event_type"] = "capital_increase" if ratio < 0.90 else "dividend_or_adjustment"
    # back-adjust: walk newest -> oldest accumulating factor
    cf = 1.0
    for i in range(n - 1, -1, -1):
        rows[i]["adj_factor"] = round(cf, 8)
        for src, dst in (("open", "adj_open"), ("high", "adj_high"), ("low", "adj_low"),
                         ("last_price", "adj_last"), ("final_price", "adj_final")):
            rows[i][dst] = round(rows[i][src] * cf, 4)
        if i > 0 and rows[i]["adj_event"]:
            cf *= rows[i]["adj_ratio"]
    return rows


# ----------------------------------------------------------------------------- main build
def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    print(f"[i] window: {START_DATE} .. {TODAY}  (cutoff dEven={CUTOFF})")
    unders = discover_underlyings()
    print(f"[i] discovered {len(unders)} option underlyings")

    all_rows = []
    master   = []
    for idx, (ins, name) in enumerate(sorted(unders.items(), key=lambda x: x[1]), 1):
        try:
            daily = fetch_daily(ins)
            time.sleep(REQ_PAUSE)
            client = fetch_clienttype(ins)
            time.sleep(REQ_PAUSE)
        except Exception as e:
            print(f"[!] {name} ({ins}): {e}")
            continue
        if not daily:
            print(f"[!] {name} ({ins}): no daily data in window")
            continue
        daily.sort(key=lambda r: r["deven"])
        compute_adjusted(daily)
        for r in daily:
            r["symbol"]  = name
            r["name"]    = name
            r["ins_code"] = ins
            r["date"]    = deven_to_iso(r["deven"])
            r["date_jalali"] = deven_to_jalali(r["deven"])
            ct = client.get(r["deven"], {})
            for k in ("ind_buy_vol","ind_sell_vol","ind_buy_val","ind_sell_val",
                      "ind_buy_count","ind_sell_count","corp_buy_vol","corp_sell_vol",
                      "corp_buy_val","corp_sell_val","corp_buy_count","corp_sell_count"):
                r[k] = ct.get(k, "")
            # net real(individual)-money flow = the classic "ورود/خروج پول حقیقی"
            if ct:
                r["ind_net_value"]  = ct["ind_buy_val"]  - ct["ind_sell_val"]
                r["corp_net_value"] = ct["corp_buy_val"] - ct["corp_sell_val"]
            else:
                r["ind_net_value"] = ""
                r["corp_net_value"] = ""
            # halted / closed day: a record exists but nothing traded
            r["is_halted"] = (r["trade_count"] == 0) or (r["volume"] == 0)
        all_rows.extend(daily)
        master.append({"symbol": name, "ins_code": ins,
                       "first_date": deven_to_iso(daily[0]["deven"]),
                       "last_date": deven_to_iso(daily[-1]["deven"]),
                       "rows": len(daily)})
        print(f"[{idx:2d}/{len(unders)}] {name:<12} rows={len(daily):4d} "
              f"{deven_to_iso(daily[0]['deven'])}..{deven_to_iso(daily[-1]['deven'])}")

    df = pd.DataFrame(all_rows)

    # ---- gap detection on a shared market calendar (union of all traded days) ----
    market_days = sorted(df["deven"].unique())
    pos = {d: i for i, d in enumerate(market_days)}
    df = df.sort_values(["symbol", "deven"]).reset_index(drop=True)
    gap_before = []
    prev_sym, prev_pos = None, None
    for sym, d in zip(df["symbol"], df["deven"]):
        if sym != prev_sym:
            gap_before.append(0)
        else:
            gap_before.append(pos[d] - prev_pos - 1)   # market days missing in-between
        prev_sym, prev_pos = sym, pos[d]
    df["gap_days_before"] = gap_before

    # ---- column order ----
    cols = ["symbol", "name", "ins_code", "date", "date_jalali",
            "open", "high", "low", "last_price", "final_price", "prev_final",
            "volume", "value", "trade_count",
            "adj_factor", "adj_open", "adj_high", "adj_low", "adj_last", "adj_final",
            "adj_event", "adj_ratio", "event_type",
            "ind_buy_vol", "ind_sell_vol", "ind_buy_val", "ind_sell_val",
            "ind_buy_count", "ind_sell_count",
            "corp_buy_vol", "corp_sell_vol", "corp_buy_val", "corp_sell_val",
            "corp_buy_count", "corp_sell_count",
            "ind_net_value", "corp_net_value",
            "is_halted", "gap_days_before"]
    df = df[cols]

    daily_path = os.path.join(OUT_DIR, "underlyings_daily.csv")
    df.to_csv(daily_path, index=False, encoding="utf-8-sig")

    mdf = pd.DataFrame(master).sort_values("symbol")
    mdf.to_csv(os.path.join(OUT_DIR, "underlyings.csv"), index=False, encoding="utf-8-sig")

    write_quality_report(df, mdf, market_days)
    print(f"\n[done] {len(df)} rows across {df['symbol'].nunique()} symbols -> {daily_path}")


def write_quality_report(df, mdf, market_days):
    lines = []
    L = lines.append
    L("# Data Quality Report — Option Underlyings (daily)\n")
    L(f"- Generated: {dt.datetime.now():%Y-%m-%d %H:%M}")
    L(f"- Window: **{START_DATE} .. {TODAY}** ({YEARS_BACK} years)")
    L(f"- Symbols: **{df['symbol'].nunique()}**")
    L(f"- Total daily rows: **{len(df):,}**")
    L(f"- Market trading days in window (union of all symbols): **{len(market_days)}**")
    L(f"- First / last market day: **{deven_to_iso(market_days[0])} .. {deven_to_iso(market_days[-1])}**\n")

    halted = df[df["is_halted"]]
    L(f"## 1. Halted / closed days (روزهای بسته) — total {len(halted):,}")
    L("Rows where a record exists but trade_count==0 or volume==0.\n")
    by = halted.groupby("symbol").size().sort_values(ascending=False)
    L("| symbol | halted days |")
    L("|---|---|")
    for s, c in by.items():
        L(f"| {s} | {c} |")
    L("")

    ev = df[df["adj_event"]]
    L(f"## 2. Price-adjustment events (افزایش سرمایه / تعدیل) — total {len(ev):,}")
    L("Days where the opening reference price differs from the previous closing "
      "price (capital increase, rights/bonus issue, or dividend). `event_type` is a "
      "ratio-based heuristic.\n")
    L("| symbol | date | ratio | event_type |")
    L("|---|---|---|---|")
    for _, r in ev.sort_values(["symbol", "date"]).iterrows():
        L(f"| {r['symbol']} | {r['date']} | {r['adj_ratio']} | {r['event_type']} |")
    L("")

    gaps = df[df["gap_days_before"] > 0]
    L(f"## 3. Gaps (شکاف‌ها) — total {len(gaps):,} gap occurrences")
    L("A gap = market trading day(s) on which the symbol had no record while other "
      "symbols traded (suspensions, late listing, long halts).\n")
    g = (gaps.groupby("symbol")["gap_days_before"]
         .agg(["count", "sum", "max"]).sort_values("sum", ascending=False))
    L("| symbol | gap occurrences | total missing days | largest gap |")
    L("|---|---|---|---|")
    for s, row in g.iterrows():
        L(f"| {s} | {int(row['count'])} | {int(row['sum'])} | {int(row['max'])} |")
    L("")

    L("## 4. Coverage per symbol")
    L("| symbol | ins_code | first_date | last_date | rows |")
    L("|---|---|---|---|---|")
    for _, r in mdf.iterrows():
        L(f"| {r['symbol']} | {r['ins_code']} | {r['first_date']} | {r['last_date']} | {r['rows']} |")

    with open(os.path.join(OUT_DIR, "quality_report.md"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines))


if __name__ == "__main__":
    main()
