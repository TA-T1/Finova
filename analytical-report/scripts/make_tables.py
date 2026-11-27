"""Markdown tables for the report text (reads outputs/*.csv)."""
import pandas as pd
O = "outputs/"
pc = lambda x, d=0: f"{x*100:.{d}f}%"
def md(df): 
    h = "| " + " | ".join(df.columns) + " |\n|" + "|".join("---" for _ in df.columns) + "|\n"
    return h + "\n".join("| " + " | ".join(str(v) for v in r) + " |" for r in df.itertuples(index=False))
a = pd.read_csv(O+"results_by_template.csv"); b = pd.read_csv(O+"results_by_template_vol_x2.2.csv")
t1 = pd.DataFrame({"Strategy": a.template, "Trades": a.trades.map(lambda x: f"{int(x):,}"), "Win rate": a.win_rate.map(pc), "Median return on risk": a.median_ror.map(pc),
  "Mean return on risk (95% CI)": [f"{pc(m)} ({pc(l)} to {pc(h)})" for m, l, h in zip(a.mean_ror_clipped, a.ci_low, a.ci_high)], "Profit factor": a.profit_factor.round(2), "Skipped": a.skipped_share.map(pc)})
t2 = pd.DataFrame({"Strategy": b.template, "Trades": b.trades.map(lambda x: f"{int(x):,}"), "Win rate": b.win_rate.map(pc), "Median return on risk": b.median_ror.map(pc),
  "Mean return on risk (95% CI)": [f"{pc(m)} ({pc(l)} to {pc(h)})" for m, l, h in zip(b.mean_ror_clipped, b.ci_low, b.ci_high)], "Profit factor": b.profit_factor.round(2)})
s = pd.read_csv(O+"sensitivity.csv")
sw = s.pivot(index="template", columns="scenario", values="win_rate").reindex(a.template)[["base_e30","e14","e7","target_stop_e30","vol_x1.5","vol_x2.2"]]
sm = s.pivot(index="template", columns="scenario", values="mean_ror_clipped").reindex(a.template)[["base_e30","e14","e7","target_stop_e30","vol_x1.5","vol_x2.2"]]
cols = ["30d, hold", "14d, hold", "7d, hold", "30d, target/stop 50%", "1.5x HV20", "2.2x HV20"]
t3 = pd.DataFrame({"Strategy": sm.index, **{c: [f"{pc(w)} / {pc(m)}" for w, m in zip(sw[k], sm[k])] for c, k in zip(cols, sw.columns)}})
mm = pd.read_csv(O+"market_vs_model_by_moneyness.csv")
t4 = mm.assign(**{"Rows": mm.n, "Market / model (median)": mm.median_ratio.round(2), "Market IV / HV20 (median)": mm.median_iv_over_hv.round(2)})[["type","bucket","Rows","Market / model (median)","Market IV / HV20 (median)"]]
t4.columns = ["Type","Strike / price","Rows","Market / model (median)","Market IV / HV20 (median)"]
y = pd.read_csv(O+"realised_vs_model_vol_by_year.csv")
t5 = pd.DataFrame({"Year": y.year, "HV20 at entry": y.median_hv_entry.map(lambda x: pc(x)), "HV over next 20 days": y.median_hv_forward.map(pc), "Ratio": y.median_ratio.round(2), "Mean 20-day return": y.mean_20d_log_return.map(lambda x: pc(x,1)), "Share of 20-day windows up": y.share_20d_up.map(pc)})
h = pd.read_csv(O+"results_by_hv_rank_at_entry.csv")
hp = h.pivot(index="template", columns="hv_bucket", values="mean_ror_clipped").reindex(a.template)[["low (0-33)","mid (33-67)","high (67-100)"]]
hw = h.pivot(index="template", columns="hv_bucket", values="win_rate").reindex(a.template)[["low (0-33)","mid (33-67)","high (67-100)"]]
t6 = pd.DataFrame({"Strategy": hp.index, **{f"HV rank {c.split(' ')[0]} {c.split(' ')[1]}": [f"{pc(w)} / {pc(m)}" for w, m in zip(hw[c], hp[c])] for c in hp.columns}})
yr = pd.read_csv(O+"results_by_template_year.csv").pivot(index="template", columns="year", values="mean_ror_clipped").reindex(a.template)
t7 = pd.DataFrame({"Strategy": yr.index, **{str(c): yr[c].map(pc) for c in yr.columns}})
u = pd.read_csv(O+"results_by_underlying.csv")
t8 = pd.DataFrame({"Underlying": u.underlying, "Trades": u.trades, "Win rate": u.win_rate.map(pc), "Mean return on risk": u.mean_ror_clipped.map(pc), "Median": u.median_ror.map(pc)})
d = pd.read_csv(O+"data_by_underlying.csv").head(10)
t9 = pd.DataFrame({"Underlying": d.underlying_symbol, "Contracts": d.contracts, "Option-day rows": d.rows, "Rows with trades": d.traded_rows, "Traded value (trillion rial)": (d.traded_value_rial/1e12).round(1)})
for n, t in [("t1 baseline", t1), ("t2 vol x2.2", t2), ("t3 sensitivity win/mean", t3), ("t4 market vs model", t4), ("t5 realised vol", t5), ("t6 hv rank win/mean", t6), ("t7 by year", t7), ("t8 by underlying", t8), ("t9 data top10", t9)]:
    print("###", n); print(md(t)); print()
