"""Run every template on some underlyings and write the results in the API shape
of docs/strategy-backtest.md (camelCase, list items + details), as one JSON file.

The front end uses it as its offline demo data (frontend/src/mocks/backtestRuns.json),
and it is the reference for what tasks 9 and 11 should store and serve.

    python scripts/export_api_runs.py --data-dir <backfill export dir> \
        --prices-csv ../data-pipeline/output/underlyings_daily.csv \
        --underlyings خودرو وبملت شپنا اهرم --weeks 52 --out run_api.json
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from finova_backtest import TEMPLATES, Rule, expiries_weekly, load_dir, run_backtest, to_backtest_run  # noqa: E402
from finova_backtest.data import load_prices_csv  # noqa: E402


def camel(s: str) -> str:
    return re.sub(r"_([a-z])", lambda m: m.group(1).upper(), s)


def camelize(x):
    if isinstance(x, dict):
        return {camel(k): camelize(v) for k, v in x.items()}
    if isinstance(x, list):
        return [camelize(v) for v in x]
    return x


def round_money(x):
    return None if x is None else round(x)


def detail(run_id: int, under: str, tpl: str, row: dict, created: str) -> dict:
    m = row["metrics"]
    trades = []
    for t in row["trades"]:
        t = camelize(t)
        for k in ("netPremium", "capitalAtRisk", "maxProfit", "maxLoss", "grossPnl", "fees", "pnl"):
            t[k] = round_money(t[k])
        t["returnOnRisk"] = round(t["returnOnRisk"], 4)
        for l in t["legs"]:
            l["entryPrice"], l["exitPrice"] = round(l["entryPrice"], 2), round(l["exitPrice"], 2)
        for k in ("chainSource",):
            t.pop(k, None)
        trades.append(t)
    metrics = camelize(m)
    for k in ("avgPnl", "totalPnl", "totalFees", "bestPnl", "worstPnl"):
        metrics[k] = round_money(metrics[k])
    for k in ("winRate", "avgReturnOnRisk", "profitFactor", "maxDrawdown", "totalReturn", "modelPricedShare"):
        if metrics[k] is not None:
            metrics[k] = round(metrics[k], 4)
    curve = [{"date": p["date"], "equity": round(p["equity"], 4)} for p in row["equity_curve"]]
    return {
        "id": run_id, "modelName": row["model_name"], "template": tpl, "underlying": under,
        "startDate": row["start_date"], "endDate": row["end_date"],
        "winRate": metrics["winRate"], "maxDrawdown": metrics["maxDrawdown"], "totalReturn": metrics["totalReturn"],
        "tradeCount": m["trades"], "avgPnl": metrics["avgPnl"], "modelPricedShare": metrics["modelPricedShare"],
        "createdAt": created,
        "params": camelize({k: v for k, v in row["params"].items() if k != "fees"}) | {"fees": camelize(row["params"]["fees"])},
        "metrics": metrics, "equityCurve": curve, "trades": trades,
        "skipped": row["skipped"],
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--data-dir", required=True)
    ap.add_argument("--prices-csv", required=True)
    ap.add_argument("--underlyings", nargs="+", required=True)
    ap.add_argument("--weeks", type=int, default=26)
    ap.add_argument("--entry-days", type=int, default=30)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()

    market = load_dir(a.data_dir, a.prices_csv)
    created = datetime.now(timezone.utc).replace(microsecond=0).isoformat()
    runs, run_id = [], 1
    for u in a.underlyings:
        last = market.last_day(u)
        exps = expiries_weekly(last - timedelta(weeks=a.weeks), last - timedelta(days=1))
        for tpl in TEMPLATES:
            row = to_backtest_run(run_backtest(market, u, Rule(tpl, entry_days=a.entry_days), exps))
            d = detail(run_id, u, tpl, row, created)
            runs.append(d)
            run_id += 1
    with open(a.out, "w", encoding="utf-8") as f:
        json.dump(runs, f, ensure_ascii=False, separators=(",", ":"))
    print(f"wrote {len(runs)} runs to {a.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
