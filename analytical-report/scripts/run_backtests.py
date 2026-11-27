"""Task 9-style runs for the report: every template on every eligible underlying (weekly Wednesday expiries, model-priced)."""
from __future__ import annotations

import json
import sys
import time
from datetime import timedelta

sys.path.insert(0, str(__import__("pathlib").Path(__file__).parent))
import pandas as pd
from common import OUT, load_market
import finova_backtest.engine as eng
from finova_backtest import TEMPLATES, Rule, expiries_weekly, run_backtest, to_backtest_run

MIN_DAYS = 400
SCENARIOS = {
    "base_e30": dict(entry_days=30),                                   # open 30 days before expiry, hold to expiry
    "e14": dict(entry_days=14),
    "e7": dict(entry_days=7),
    "target_stop_e30": dict(entry_days=30, exit="target_stop", target_pct=0.5, stop_pct=0.5),
    "vol_x1.5": dict(entry_days=30, _vol=1.5),          # options priced at 1.5 x HV20
    "vol_x2.2": dict(entry_days=30, _vol=2.2),          # options priced at 2.2 x HV20 (median market IV / HV20 seen in the data)
}


def main() -> None:
    m, _ = load_market()
    unders = [u for u in m.underlyings() if len(m.trading_days(u)) >= MIN_DAYS]
    print(len(unders), "underlyings")
    run_rows, trade_rows, skip_rows, curves = [], [], [], {}
    t0 = time.time()
    base_hv = eng.hv20
    for sc, kw in SCENARIOS.items():
        kw = dict(kw)
        k = kw.pop("_vol", 1.0)
        eng.hv20 = (lambda c, k=k: (None if base_hv(c) is None else base_hv(c) * k))
        for u in unders:
            first, last = m.first_day(u), m.last_day(u)
            start = max(first + timedelta(days=60), last - timedelta(days=3 * 365))
            exps = expiries_weekly(start, last)
            for tpl in TEMPLATES:
                res = run_backtest(m, u, Rule(tpl, **kw), exps)
                row = to_backtest_run(res)
                met = row["metrics"]
                run_rows.append({"scenario": sc, "underlying": u, "template": tpl, "start": row["start_date"], "end": row["end_date"],
                                 "win_rate": met["win_rate"], "max_drawdown": met["max_drawdown"], "total_return": met["total_return"], **{k: met[k] for k in ("trades", "skipped", "avg_pnl", "avg_return_on_risk", "total_pnl", "total_fees", "profit_factor", "best_pnl", "worst_pnl", "model_priced_share")}})
                for t in row["trades"]:
                    trade_rows.append({"scenario": sc, "underlying": u, "template": tpl, **{k: t[k] for k in ("expiry", "entry_date", "exit_date", "exit_reason", "spot_entry", "spot_exit", "net_premium", "capital_at_risk", "max_profit", "max_loss", "gross_pnl", "fees", "pnl", "return_on_risk", "price_source")}})
                for s in row["skipped"]:
                    skip_rows.append({"scenario": sc, "underlying": u, "template": tpl, **s})
                if sc == "base_e30":
                    curves[f"{u}|{tpl}"] = row["equity_curve"]
        print(sc, "done", round(time.time() - t0), "s")
    pd.DataFrame(run_rows).to_csv(OUT / "runs.csv", index=False, encoding="utf-8-sig")
    pd.DataFrame(trade_rows).to_csv(OUT / "trades.csv", index=False, encoding="utf-8-sig")
    pd.DataFrame(skip_rows).to_csv(OUT / "skipped.csv", index=False, encoding="utf-8-sig")
    (OUT / "equity_curves_base.json").write_text(json.dumps(curves, ensure_ascii=False), encoding="utf-8")


if __name__ == "__main__":
    main()
