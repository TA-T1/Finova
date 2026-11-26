from __future__ import annotations

import argparse
import json
import sys
from datetime import date, timedelta

from . import data as data_mod
from .engine import Fees, Rule, completed_expiries, expiries_weekly, run_backtest, to_backtest_run
from .templates import TEMPLATES


def _parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="finova_backtest", description="Backtest an option strategy rule over past expiries.")
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("templates", help="list the strategy templates")
    r = sub.add_parser("run", help="run one rule on one underlying")
    src = r.add_mutually_exclusive_group(required=True)
    src.add_argument("--data-dir", help="demo dataset directory (backfill export)")
    src.add_argument("--db", help="PostgreSQL URL")
    r.add_argument("--prices-csv", help="adjusted underlying prices (data-pipeline/output/underlyings_daily.csv); "
                                        "recommended: the database's raw prices have capital-increase jumps")
    r.add_argument("--underlying", required=True, help="underlying symbol, e.g. خودرو")
    r.add_argument("--template", required=True, choices=sorted(TEMPLATES))
    r.add_argument("--entry-days", type=int, default=30, help="calendar days before expiry to open (default 30)")
    r.add_argument("--quantity", type=int, default=1)
    r.add_argument("--exit", choices=["expiry", "target_stop"], default="expiry")
    r.add_argument("--target", type=float, help="exit at this profit, as a fraction of capital at risk (0.5 = 50%%)")
    r.add_argument("--stop", type=float, help="exit at this loss, as a fraction of capital at risk")
    r.add_argument("--fill", choices=["last", "mid", "cross"], default="last")
    r.add_argument("--allocation", type=float, default=0.05, help="fraction of the starting equity risked per trade")
    r.add_argument("--no-model", action="store_true", help="snapshot prices only; skip expiries that need Black-Scholes")
    r.add_argument("--expiries", choices=["observed", "weekly"], default="observed",
                   help="observed: expiries seen in quote_snapshots that have passed; "
                        "weekly: a synthetic Wednesday calendar (model-priced)")
    r.add_argument("--from", dest="start", type=date.fromisoformat, help="weekly: first expiry date")
    r.add_argument("--to", dest="end", type=date.fromisoformat, help="weekly: last expiry date")
    r.add_argument("--out", help="write the result JSON here (default: print a summary)")
    return p


def main(argv=None) -> int:
    args = _parser().parse_args(argv)
    if args.cmd == "templates":
        for t in TEMPLATES.values():
            legs = ", ".join(f"{l.side} {l.type}" + (f" {l.strike.rule}{l.strike.steps or ''}" if l.strike else "") for l in t.legs)
            print(f"{t.id:18} {t.outlook:9} {t.name_fa:14} {legs}")
        return 0

    market = data_mod.load_dir(args.data_dir, args.prices_csv) if args.data_dir else data_mod.load_db(args.db)
    if args.db and args.prices_csv:
        market = market.with_daily(data_mod.load_prices_csv(args.prices_csv))
    u = args.underlying
    if u not in market.underlyings():
        print(f"unknown underlying {u!r}; known: {', '.join(market.underlyings())}", file=sys.stderr)
        return 2
    rule = Rule(args.template, entry_days=args.entry_days, quantity=args.quantity, exit=args.exit,
                target_pct=args.target, stop_pct=args.stop, fill=args.fill, allocation=args.allocation,
                allow_model=not args.no_model, fees=Fees())
    if args.expiries == "weekly":
        first, last = market.first_day(u), market.last_day(u)
        # default: the last three years (the length of the project's price dataset), once HV20 exists
        start = args.start or max(first + timedelta(days=60), last - timedelta(days=3 * 365))
        end = args.end or last
        expiries = expiries_weekly(start, end)
    else:
        expiries = completed_expiries(market, u)
        if not expiries:
            print("no expiry of this underlying has passed within the data yet (quote_snapshots only holds "
                  "contracts that are still listed). Use --expiries weekly for a model-priced backtest.", file=sys.stderr)
            return 3
    res = run_backtest(market, u, rule, expiries)
    out = to_backtest_run(res)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(out, f, ensure_ascii=False, indent=1)
    m = res.metrics
    print(f"{u} {rule.template_id}: {m['trades']} trades, {m['skipped']} skipped, "
          f"win rate {m['win_rate']}, total return {m['total_return']:.3f}, max drawdown {m['max_drawdown']:.3f}")
    return 0
