"""Step 4 - review of the 69 price-adjustment events and the choice of a volume-adjustment method.

What the step answers
  1. Is every adjustment event a real corporate action, and of which kind (bonus/split, rights issue, dividend-type)?
  2. Are the adjustment ratios consistent with the price path (halt-to-reopen return) and with the volume path?
  3. When does the share count really change (same day as the reference price, or later)?
  4. Which volume series should the later steps use?

Outputs (outputs/step4_adjustments/):
  adj_event_final.csv          one row per original event with all evidence, class, confidence and the decision
  adj_events_corrected.csv     the corrected event list that rebuilds the adjustment factor (net / dropped events applied)
  adjusted_series.csv          symbol x record: corrected price factor, share factor, adj_volume, flags
  halt_run_returns.csv         every halt run with its adjusted return across the halt (price-side reference distribution)
  volume_paths.csv             market-adjusted log-volume path around every event (5 windows, volume and value)
  volume_pooled_slope.csv      pooled regression: observed log volume step vs the exchange's log(1/ratio)
  volume_method_comparison.csv residual jump after applying each candidate volume adjustment
  checks_summary.csv, summary.json, three PNG charts
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

from common import load_daily, market_closure_days, out_dir, save_table


def main() -> None:

    OUT = out_dir("step4_adjustments")
    checks: list[dict] = []

    # ---------------------------------------------------------------- parameters
    N_WIN = 20                      # active rows per window
    OFFSETS = [3, 23, 43, 63, 83]   # post windows start this many ACTIVE rows after the event (first 3 = reopen noise)
    MIN_PRE, MIN_POST = 5, 8        # minimum active rows in the pre / post window
    MIN_MKT = 5                     # minimum other symbols for the market benchmark
    SHARE_RATIO_MAX = 0.9           # ratio below this is treated as a share-count change (evidence: pooled slope)
    SUPPORT_OK, POWER_OK = 4.0, 4.0  # chi-square-like thresholds (~2 sigma) for "confirmed" / "testable"
    PRICE_PCTL_FLAG = 0.95
    CLUSTER_GAP_ROWS = 7


    def add(check_id: str, name: str, status: str, n: int | None, detail: str) -> None:
        checks.append({"id": check_id, "check": name, "status": status, "n_issues": n, "detail": detail})


    # ====================================================================== load
    df = load_daily()
    df["pos"] = df.groupby("symbol").cumcount()
    closure = set(market_closure_days(df))
    df["in_closure"] = df["date"].isin(closure)
    df["prev_row_final"] = df.groupby("symbol")["final_price"].shift(1)
    df["prev_row_adj_final"] = df.groupby("symbol")["adj_final"].shift(1)
    df["jal_month"] = df["date_jalali"].str[5:7].astype(int)

    SYM: dict[str, dict] = {}
    for s, g in df.groupby("symbol", sort=True):
        g = g.reset_index(drop=True)
        act = ~g["is_halted"].values
        SYM[s] = dict(g=g, n=len(g), date=g["date"].values, ap=np.flatnonzero(act),
                      vol=g["volume"].values.astype(float), val=g["value"].values.astype(float))

    # ====================================================================== 1. event table + structural checks
    ev = df[df["adj_event"]].copy().reset_index(drop=True)
    ev["event_id"] = [f"E{i + 1:02d}" for i in range(len(ev))]
    ev["inv_ratio"] = 1 / ev["adj_ratio"]
    ev["exp_log"] = -np.log(ev["adj_ratio"])
    ev["on_halted_row"] = ev["is_halted"]

    ratio_re = ev["prev_final"] / ev["prev_row_final"]
    add("S4.01", "event ratio == prev_final / previous record's final_price (69 events)",
        "PASS" if len(ev) == 69 and (ratio_re - ev["adj_ratio"]).abs().max() < 1e-5 else "FAIL",
        int(((ratio_re - ev["adj_ratio"]).abs() > 1e-5).sum()),
        f"{len(ev)} events; max |recomputed - adj_ratio| = {(ratio_re - ev['adj_ratio']).abs().max():.1e}")

    # rebuild the factor chain from the ratios alone
    def rebuild_factor(events_by_sym: dict[str, list[tuple[int, float]]]) -> pd.Series:
        out = []
        for s, S in SYM.items():
            f = np.ones(S["n"])
            for pos, ratio in events_by_sym.get(s, []):
                f[:pos] *= ratio
            out.append(pd.Series(f, index=df.index[df["symbol"] == s]))
        return pd.concat(out).sort_index()

    orig_events = {s: [(int(r.pos), float(r.adj_ratio)) for r in ev[ev["symbol"] == s].itertuples()] for s in SYM}
    f_chk = rebuild_factor(orig_events)
    dev = (f_chk / df["adj_factor"] - 1).abs().max()
    add("S4.02", "adj_factor equals the cumulative backward product of the event ratios (every row)",
        "PASS" if dev < 1e-5 else "FAIL", int(((f_chk / df["adj_factor"] - 1).abs() > 1e-5).sum()),
        f"max relative deviation {dev:.1e}")

    h = ev[ev["on_halted_row"]]
    same = (h["final_price"] == h["prev_final"]).sum()
    add("S4.03", "events on halted rows: final_price already equals the NEW reference (prev_final)",
        "PASS" if same == len(h) else "WARN", int(len(h) - same),
        f"{same}/{len(h)} halted event rows carry the new reference as final_price (so halted rows hold post-event prices, "
        f"and the old price level ends on the previous record)")

    tick = 1 / ev["prev_row_final"].clip(lower=1)
    min_dev = (1 - ev["adj_ratio"]).abs().min()
    add("S4.04", "smallest event is larger than tick/rounding noise",
        "PASS" if min_dev > 0.003 else "WARN", int(((1 - ev["adj_ratio"]).abs() < 0.003).sum()),
        f"min |1-ratio| = {min_dev:.4f}; prices are integers so rounding error is ~{(1 / ev['prev_row_final']).median():.1e}")

    # ---- clusters (events of one symbol within CLUSTER_GAP_ROWS rows)
    ev = ev.sort_values(["symbol", "pos"]).reset_index(drop=True)
    cl = np.zeros(len(ev), dtype=int)
    cid = 0
    for i in range(len(ev)):
        if i == 0 or ev.loc[i, "symbol"] != ev.loc[i - 1, "symbol"] or ev.loc[i, "pos"] - ev.loc[i - 1, "pos"] > CLUSTER_GAP_ROWS:
            cid += 1
        cl[i] = cid
    ev["cluster_id"] = cl
    ev["n_in_cluster"] = ev.groupby("cluster_id")["event_id"].transform("size")
    ev["cluster_net_ratio"] = ev.groupby("cluster_id")["adj_ratio"].transform("prod")

    first_active = {s: SYM[s]["g"]["date"].values[SYM[s]["ap"][0]] for s in SYM}
    ev["is_first_active_row"] = [np.datetime64(d) == first_active[s] for s, d in zip(ev["symbol"], ev["date"])]

    ev["cls_special"] = ""
    rev_clusters = ev[(ev["n_in_cluster"] > 1)].groupby("cluster_id").filter(
        lambda x: (x["adj_ratio"] > 1).any() and abs(x["adj_ratio"].prod() - 1) < 0.10)["cluster_id"].unique()
    ev.loc[ev["cluster_id"].isin(rev_clusters), "cls_special"] = "REVERSAL_CLUSTER"
    ev.loc[(ev["adj_ratio"] > 1) & ev["is_first_active_row"] & (ev["cls_special"] == ""), "cls_special"] = "LISTING_REFERENCE"

    nclus = ev[ev["n_in_cluster"] > 1].groupby("cluster_id").agg(sym=("symbol", "first"), n=("event_id", "size"),
                                                                  net=("adj_ratio", "prod"))
    add("S4.05", "clusters of events (same symbol, <= 7 records apart)", "WARN" if len(nclus) else "PASS", int(len(nclus)),
        "; ".join(f"{r.sym}: {r.n} events, net ratio {r.net:.4f}" for r in nclus.itertuples()))
    up = ev[ev["adj_ratio"] > 1]
    add("S4.06", "events with ratio > 1 (reference price UP on an adjustment day)", "WARN", int(len(up)),
        "; ".join(f"{r.symbol} {r.date.date()} ratio {r.adj_ratio:.3f} -> {r.cls_special or 'unclassified'}" for r in up.itertuples()))

    # ====================================================================== 2. price-side evidence: return across the halt
    for s, S in SYM.items():
        S["g"]["adj_final_orig"] = S["g"]["adj_final"]

    runs = []
    for s, S in SYM.items():
        g = S["g"]
        h_arr = g["is_halted"].values
        rid = (pd.Series(h_arr) != pd.Series(h_arr).shift()).cumsum().values
        for k in np.unique(rid[h_arr]):
            idx = np.flatnonzero((rid == k) & h_arr)
            i0, i1 = idx[0], idx[-1]
            if i0 == 0 or i1 + 1 >= len(g):
                continue
            la, fa = i0 - 1, i1 + 1
            n_sym = int((~g["in_closure"].values[idx]).sum())
            has_ev = bool(g["adj_event"].values[idx].any() or g["adj_event"].values[fa])
            runs.append(dict(symbol=s, run_start=g["date"].iloc[i0].date(), la_pos=la, fa_pos=fa, n_records=len(idx),
                             n_symbol_days=n_sym, has_event=has_ev,
                             ret_across=g["adj_final"].values[fa] / g["adj_final"].values[la] - 1,
                             reopen_ret=g["final_price"].values[fa] / g["prev_final"].values[fa] - 1))
    runs = pd.DataFrame(runs)
    runs["len_bin"] = pd.cut(runs["n_symbol_days"], [-1, 0, 1, 3, 10, 10_000], labels=["0", "1", "2-3", "4-10", ">10"]).astype(str)
    runs["abs_ret"] = runs["ret_across"].abs()
    base = runs[(~runs["has_event"]) & (runs["len_bin"] != "0")]


    def pctile(row) -> float:
        pool = base[base["len_bin"] == row["len_bin"]]["abs_ret"]
        return float((pool <= row["abs_ret"]).mean()) if len(pool) else np.nan


    runs["pctile_vs_nonevent"] = runs.apply(lambda r: pctile(r) if r["has_event"] and r["len_bin"] != "0" else np.nan, axis=1)
    save_table(runs.drop(columns=["la_pos", "fa_pos"]), OUT / "halt_run_returns.csv")

    dev_reopen = (runs["ret_across"] - runs["reopen_ret"]).abs()
    add("S4.07", "adjusted return across each halt reproduces the exchange's reopen return (final/prev_final-1)",
        "PASS" if dev_reopen.max() < 0.01 else "WARN", int((dev_reopen > 0.005).sum()),
        f"{len(runs)} halt runs; max |difference| = {dev_reopen.max():.4f}, median {dev_reopen.median():.1e}")

    # attach to events: the run that ends right before the first active record on/after the event
    pr = []
    for r in ev.itertuples():
        S = SYM[r.symbol]
        ap = S["ap"]
        k = int(np.searchsorted(ap, r.pos, "left"))
        fa = int(ap[k]) if k < len(ap) else None
        m = runs[(runs["symbol"] == r.symbol) & (runs["fa_pos"] == fa)] if fa is not None else runs.iloc[0:0]
        if len(m):
            m = m.iloc[0]
            pr.append(dict(halt_symbol_days=int(m["n_symbol_days"]), halt_records=int(m["n_records"]),
                           adj_ret_across_halt=float(m["ret_across"]), ret_pctile_vs_nonevent=m["pctile_vs_nonevent"]))
        else:
            pr.append(dict(halt_symbol_days=np.nan, halt_records=np.nan, adj_ret_across_halt=np.nan, ret_pctile_vs_nonevent=np.nan))
    ev = pd.concat([ev, pd.DataFrame(pr)], axis=1)
    ev["price_outlier"] = ev["ret_pctile_vs_nonevent"] >= PRICE_PCTL_FLAG
    n_out = int(ev["price_outlier"].sum())
    add("S4.08", "adjusted return across the halt of event runs vs non-event halt runs of the same length (p>=95% = outlier)",
        "PASS" if n_out == 0 else "WARN", n_out,
        f"{int(ev['ret_pctile_vs_nonevent'].notna().sum())} testable events, {n_out} above the 95th percentile: "
        + ", ".join(f"{r.event_id}({r.symbol} {r.date.date()} {r.adj_ret_across_halt:+.1%})" for r in ev[ev['price_outlier']].itertuples()))

    # ====================================================================== 3. volume-side evidence
    share_like = ev[(ev["adj_ratio"] < SHARE_RATIO_MAX) & (ev["cls_special"] == "")]
    SHARE_POS = {s: sorted(share_like[share_like["symbol"] == s]["pos"].astype(int).tolist()) for s in SYM}


    def jump(S: dict, i: int, k0: int, lo_cut: int, hi_cut: int):
        """log(median post / median pre) for volume and value. pre = last N active rows before i, post = rows k0.. after."""
        ap = S["ap"]
        r = int(np.searchsorted(ap, i, "left"))
        pre = ap[max(0, r - N_WIN):r]
        pre = pre[pre > lo_cut]
        post = ap[r + k0:r + k0 + N_WIN]
        post = post[post < hi_cut]
        if len(pre) < MIN_PRE or len(post) < MIN_POST:
            return np.nan, np.nan, len(pre), len(post)
        return (float(np.log(np.median(S["vol"][post]) / np.median(S["vol"][pre]))),
                float(np.log(np.median(S["val"][post]) / np.median(S["val"][pre]))), len(pre), len(post))


    def market_jump(date, k0: int, exclude: str):
        """Median jump of all OTHER symbols over the same calendar window (share-type events nearby are excluded)."""
        v, u = [], []
        for t, T in SYM.items():
            if t == exclude:
                continue
            i = int(np.searchsorted(T["date"], np.datetime64(date)))
            if i >= T["n"] or any(i - 140 <= p <= i + k0 + 30 for p in SHARE_POS[t]):
                continue
            a, b, _, _ = jump(T, i, k0, -1, T["n"])
            if np.isfinite(a):
                v.append(a)
                u.append(b)
        return (float(np.median(v)), float(np.median(u))) if len(v) >= MIN_MKT else (np.nan, np.nan)


    rows = []
    for r in ev.itertuples():
        S = SYM[r.symbol]
        prev_p = [p for p in SHARE_POS[r.symbol] if p < r.pos]
        next_p = [p for p in SHARE_POS[r.symbol] if p > r.pos]
        lo, hi = (max(prev_p) if prev_p else -1), (min(next_p) if next_p else S["n"])
        d = dict(event_id=r.event_id)
        for w, k0 in enumerate(OFFSETS):
            a, b, npre, npost = jump(S, int(r.pos), k0, lo, hi)
            mv, mu = market_jump(r.date, k0, r.symbol)
            d[f"lv{w}"], d[f"lval{w}"], d[f"mv{w}"], d[f"mu{w}"] = a, b, mv, mu
            d[f"ex{w}"], d[f"exv{w}"] = a - mv, b - mu
            if w == 0:
                d["n_pre"], d["n_post0"] = npre, npost
        rows.append(d)
    VP = pd.DataFrame(rows)

    # placebo: same statistic at event-free dates -> noise level per window
    pl = {w: [] for w in range(len(OFFSETS))}
    plv = {w: [] for w in range(len(OFFSETS))}
    for s, S in SYM.items():
        cand = [i for i in S["ap"] if 25 <= i <= S["n"] - 150 and not any(i - 140 <= p <= i + 130 for p in SHARE_POS[s])]
        for i in cand[::7]:
            for w, k0 in enumerate(OFFSETS):
                a, b, _, _ = jump(S, int(i), k0, -1, S["n"])
                mv, mu = market_jump(S["date"][i], k0, s)
                if np.isfinite(a) and np.isfinite(mv):
                    pl[w].append(a - mv)
                    plv[w].append(b - mu)
    SIG = np.array([np.std(pl[w]) for w in range(len(OFFSETS))])
    SIGV = np.array([np.std(plv[w]) for w in range(len(OFFSETS))])
    PLACEBO = pd.DataFrame({"window": [f"+{k}..+{k + N_WIN}" for k in OFFSETS], "n_placebo": [len(pl[w]) for w in range(len(OFFSETS))],
                            "sd_excess_log_volume": SIG, "mean_excess_log_volume": [np.mean(pl[w]) for w in range(len(OFFSETS))],
                            "sd_excess_log_value": SIGV})
    save_table(PLACEBO, OUT / "volume_placebo_noise.csv")

    ev = ev.merge(VP, on="event_id", how="left")
    # data artefacts (reversal cluster / listing reference) are not corporate actions: no volume test
    sp = ev["cls_special"] != ""
    for c in [c for c in VP.columns if c != "event_id"]:
        ev.loc[sp, c] = np.nan
    EXC = ev[[f"ex{w}" for w in range(len(OFFSETS))]].values
    EXV = ev[[f"exv{w}" for w in range(len(OFFSETS))]].values

    # step-model fit: path_w = exp * 1[w >= j] ; j in 0..4 or never
    fit = []
    for k in range(len(ev)):
        x, e = EXC[k], float(ev.loc[k, "exp_log"])
        m = np.isfinite(x)
        if m.sum() == 0 or ev.loc[k, "cls_special"] != "" or e <= 0:
            fit.append(dict(best_j=np.nan, support=np.nan, power=np.nan, amp_hat=np.nan, n_windows=int(m.sum())))
            continue
        sse_none = float((((x[m]) / SIG[m]) ** 2).sum())
        best_j, best_sse = None, np.inf
        for j in range(len(OFFSETS)):
            step = np.where(np.arange(len(OFFSETS)) >= j, e, 0.0)
            sse = float((((x[m] - step[m]) / SIG[m]) ** 2).sum())
            if sse < best_sse:
                best_j, best_sse = j, sse
        sel = m & (np.arange(len(OFFSETS)) >= best_j)
        amp = float(np.mean(x[sel]) / e) if sel.any() else np.nan
        power = float(((e / SIG[m]) ** 2).sum())
        fit.append(dict(best_j=best_j, support=sse_none - best_sse, power=power, amp_hat=amp, n_windows=int(m.sum())))
    ev = pd.concat([ev, pd.DataFrame(fit)], axis=1)
    ev["z0_vs_expected"] = (ev["ex0"] - ev["exp_log"]) / SIG[0]
    ev["z0_vs_none"] = ev["ex0"] / SIG[0]


    def vol_test(r) -> str:
        if r["cls_special"] != "":
            return "N/A"
        if not np.isfinite(r["support"]):
            return "NO_DATA"
        if r["support"] >= SUPPORT_OK:
            return "CONFIRMED_IMMEDIATE" if r["best_j"] == 0 else "CONFIRMED_DELAYED"
        if r["power"] < POWER_OK:
            return "NOT_TESTABLE"
        if r["support"] >= 1:
            return "PLAUSIBLE"
        return "UNSUPPORTED"


    ev["volume_test"] = ev.apply(vol_test, axis=1)

    # ---- pooled slope (through the origin) with bootstrap CI, by size group (window 0 = immediate)
    rng = np.random.default_rng(0)
    pool_rows = []
    base_ev = ev[(ev["cls_special"] == "") & ev["ex0"].notna()]
    for name, m in [("ratio<0.3", base_ev["adj_ratio"] < 0.3),
                    ("0.3<=ratio<0.9", (base_ev["adj_ratio"] >= 0.3) & (base_ev["adj_ratio"] < 0.9)),
                    ("ratio<0.9 (all)", base_ev["adj_ratio"] < 0.9),
                    ("ratio>=0.9", base_ev["adj_ratio"] >= 0.9)]:
        for wname, col in [("w0 (+3..+23)", "ex0"), ("mean of w0..w4", None)]:
            x = base_ev[m]
            if col is None:
                y = np.nanmean(x[[f"ex{w}" for w in range(len(OFFSETS))]].values, axis=1)
            else:
                y = x[col].values
            e = x["exp_log"].values
            if len(x) < 3:
                continue
            keep = np.isfinite(y)
            x, y = x[keep], y[keep]
            slope = (y * e).sum() / (e ** 2).sum()
            bs = []
            for _ in range(2000):
                ii = rng.integers(0, len(x), len(x))
                bs.append((y[ii] * e[ii]).sum() / max((e[ii] ** 2).sum(), 1e-12))
            pool_rows.append(dict(group=name, window=wname, n=len(x), mean_expected_log=e.mean(), mean_observed_excess=float(np.mean(y)),
                                  slope_through_origin=slope, ci95_low=np.percentile(bs, 2.5), ci95_high=np.percentile(bs, 97.5)))
    POOL = pd.DataFrame(pool_rows)
    save_table(POOL, OUT / "volume_pooled_slope.csv")
    g_lo = POOL[(POOL["group"] == "ratio<0.9 (all)") & (POOL["window"].str.startswith("w0"))].iloc[0]
    add("S4.09", "pooled volume evidence: observed log volume step vs the exchange's log(1/ratio), events with ratio < 0.9",
        "INFO", int(g_lo["n"]),
        f"slope {g_lo['slope_through_origin']:.2f} (95% CI {g_lo['ci95_low']:.2f}..{g_lo['ci95_high']:.2f}); "
        + "; ".join(f"{r.group}: slope {r.slope_through_origin:.2f} [{r.ci95_low:.2f},{r.ci95_high:.2f}] n={r.n}"
                    for r in POOL[POOL['window'].str.startswith('w0')].itertuples()))

    # ====================================================================== 4. classification and decision
    def classify(r) -> pd.Series:
        if r["cls_special"] == "REVERSAL_CLUSTER":
            last = ev[(ev["cluster_id"] == r["cluster_id"])]["pos"].max()
            act = "REPLACE_WITH_NET" if r["pos"] == last else "DROP_MERGED"
            return pd.Series(dict(event_class="REVERSAL_CLUSTER", confidence="HIGH", share_event=False, action=act, arrival="-"))
        if r["cls_special"] == "LISTING_REFERENCE":
            return pd.Series(dict(event_class="LISTING_REFERENCE", confidence="HIGH", share_event=False, action="DROP_EVENT", arrival="-"))
        if r["adj_ratio"] >= SHARE_RATIO_MAX:
            return pd.Series(dict(event_class="PRICE_ONLY_DIVIDEND_LIKE", confidence="MEDIUM", share_event=False, action="KEEP", arrival="-"))
        t = r["volume_test"]
        if t == "CONFIRMED_IMMEDIATE":
            return pd.Series(dict(event_class="SHARE_INCREASE_IMMEDIATE", confidence="HIGH", share_event=True, action="KEEP", arrival="event_day"))
        if t == "CONFIRMED_DELAYED":
            return pd.Series(dict(event_class="SHARE_INCREASE_DELAYED", confidence="HIGH", share_event=True, action="KEEP", arrival="delayed"))
        if t == "PLAUSIBLE":
            return pd.Series(dict(event_class="SHARE_INCREASE_PROBABLE", confidence="MEDIUM", share_event=True, action="KEEP", arrival="event_day"))
        if t == "UNSUPPORTED":
            return pd.Series(dict(event_class="AMBIGUOUS_NO_VOLUME_STEP", confidence="LOW", share_event=True, action="KEEP", arrival="event_day"))
        return pd.Series(dict(event_class="AMBIGUOUS_SHARE_OR_DIVIDEND", confidence="LOW", share_event=True, action="KEEP", arrival="event_day"))


    ev = pd.concat([ev, ev.apply(classify, axis=1)], axis=1)
    # arrival rank (active rows after the event) of the new shares
    ev["share_arrival_rank"] = np.where(ev["share_event"] & (ev["arrival"] == "delayed"), [OFFSETS[int(j)] - 10 if np.isfinite(j) else 0 for j in ev["best_j"]],
                                        0)
    ev["uncertain_from_rank"] = np.where(ev["arrival"] == "delayed", np.maximum(ev["share_arrival_rank"] - 10, 0), np.nan)
    ev["uncertain_to_rank"] = np.where(ev["arrival"] == "delayed", ev["share_arrival_rank"] + 30, np.nan)
    ev["share_step"] = np.where(ev["share_event"], ev["inv_ratio"], 1.0)
    ev["inv_ratio_is_round"] = (ev["inv_ratio"] - ev["inv_ratio"].round()).abs() < 0.002 * ev["inv_ratio"]
    ev["in_dividend_season"] = ev["jal_month"].isin([4, 5])      # Tir-Mordad: AGM / ex-dividend season

    # external check list
    def needs_check(r) -> str:
        why = []
        if r["event_class"] in ("REVERSAL_CLUSTER", "LISTING_REFERENCE"):
            why.append("data artefact: confirm the reference price on the exchange page")
        if r["event_class"] == "SHARE_INCREASE_DELAYED":
            why.append("confirm registration date of the new shares")
        if r["share_event"] and np.isfinite(r["amp_hat"]) and r["power"] >= POWER_OK and (r["amp_hat"] > 2 or r["amp_hat"] < 0.4):
            why.append(f"volume step is {r['amp_hat']:.1f}x the ratio-implied step: check rights/bonus terms")
        if r["event_class"].startswith("AMBIGUOUS") and r["adj_ratio"] < 0.8:
            why.append("large ratio but volume cannot confirm: capital increase or big dividend?")
        if r["event_class"].startswith("AMBIGUOUS") and r["in_dividend_season"]:
            why.append("falls in the dividend season (Jalali months 4-5): dividend is a serious alternative")
        if r["inv_ratio_is_round"] and r["adj_ratio"] < 0.5:
            why.append("round 1/ratio (unit split or pure bonus issue): confirm terms")
        if r["price_outlier"]:
            why.append("halt-to-reopen return is an outlier for its halt length")
        return "; ".join(why)


    ev["external_check"] = ev.apply(needs_check, axis=1)

    # slope inside the ambiguous group (selection-biased LOW: strongly confirmed events were moved out of it)
    amb = ev[ev["event_class"].isin(["AMBIGUOUS_SHARE_OR_DIVIDEND", "SHARE_INCREASE_PROBABLE"])]
    for wname, y in [("w0 (+3..+23)", amb["ex0"].values),
                     ("mean of w0..w4", np.nanmean(amb[[f"ex{w}" for w in range(len(OFFSETS))]].values, axis=1))]:
        e_ = amb["exp_log"].values
        k_ = np.isfinite(y)
        sl = (y[k_] * e_[k_]).sum() / (e_[k_] ** 2).sum()
        bs = []
        for _ in range(2000):
            ii = rng.integers(0, k_.sum(), k_.sum())
            bs.append((y[k_][ii] * e_[k_][ii]).sum() / (e_[k_][ii] ** 2).sum())
        POOL = pd.concat([POOL, pd.DataFrame([dict(group="ambiguous/probable only (biased low)", window=wname, n=int(k_.sum()),
                                                   mean_expected_log=e_[k_].mean(), mean_observed_excess=float(y[k_].mean()),
                                                   slope_through_origin=sl, ci95_low=np.percentile(bs, 2.5), ci95_high=np.percentile(bs, 97.5))])],
                         ignore_index=True)
    save_table(POOL, OUT / "volume_pooled_slope.csv")

    cls_counts = ev["event_class"].value_counts().to_dict()
    add("S4.10", "final event classes", "INFO", len(ev), str(cls_counts))
    months = ev[ev["event_class"] == "PRICE_ONLY_DIVIDEND_LIKE"]["jal_month"].value_counts().sort_index().to_dict()
    in_season = int(ev[ev["event_class"] == "PRICE_ONLY_DIVIDEND_LIKE"]["jal_month"].between(3, 6).sum())
    add("S4.11", "ratio>=0.9 events fall in the AGM/dividend season (Jalali months 3-6)", "INFO",
        int(len(ev[ev["event_class"] == "PRICE_ONLY_DIVIDEND_LIKE"])),
        f"{in_season}/{len(ev[ev['event_class'] == 'PRICE_ONLY_DIVIDEND_LIKE'])} in months 3-6; by Jalali month: {months}")
    delayed = ev[ev["event_class"] == "SHARE_INCREASE_DELAYED"]
    add("S4.12", "share count changes later than the reference price (rights-issue pattern)", "WARN" if len(delayed) else "PASS", int(len(delayed)),
        "; ".join(f"{r.event_id} {r.symbol} {r.date.date()}: step visible from window +{OFFSETS[int(r.best_j)]}..+{OFFSETS[int(r.best_j)] + N_WIN}"
                  for r in delayed.itertuples()))

    # ====================================================================== 5. corrected factor + volume series
    corr_events: dict[str, list[tuple[int, float]]] = {s: [] for s in SYM}
    net_rows = []
    for r in ev.itertuples():
        if r.action == "KEEP":
            corr_events[r.symbol].append((int(r.pos), float(r.adj_ratio)))
        elif r.action == "REPLACE_WITH_NET":
            corr_events[r.symbol].append((int(r.pos), float(r.cluster_net_ratio)))
            net_rows.append(dict(symbol=r.symbol, date=r.date.date(), pos=int(r.pos), ratio=float(r.cluster_net_ratio),
                                 source=f"net of cluster {r.symbol}"))
    fc = rebuild_factor(corr_events)
    df["adj_factor_corrected"] = fc.values
    changed = df[(df["adj_factor_corrected"] / df["adj_factor"] - 1).abs() > 1e-6]   # file factor has 8 significant digits
    add("S4.13", "corrected price factor differs from the file's adj_factor only where an event was dropped or merged",
        "INFO", int(len(changed)),
        f"{len(changed)} rows changed: " + str(changed.groupby('symbol').size().to_dict()))
    assert np.allclose(df.groupby("symbol")["adj_factor_corrected"].last(), 1.0)
    add("S4.14", "corrected factor of the latest record of every symbol is 1.0", "PASS", 0, "all 27 symbols")

    # price anomalies: halted rows inside a reversal cluster carry bogus final prices
    anom = np.zeros(len(df), dtype=bool)
    for c in rev_clusters:
        sub = ev[ev["cluster_id"] == c]
        sym = sub["symbol"].iloc[0]
        lo_p, hi_p = sub["pos"].min(), sub["pos"].max()
        m = (df["symbol"] == sym) & (df["pos"] >= lo_p) & (df["pos"] < hi_p)
        anom |= m.values
    df["price_anomaly"] = anom
    df["adj_final_corrected"] = np.where(df["price_anomaly"], np.nan, df["final_price"] * df["adj_factor_corrected"])

    # continuity of the corrected chain: return across every halt equals the exchange's reopen return
    cc = []
    for s, S in SYM.items():
        g = df[df["symbol"] == s].reset_index(drop=True)
        for r in runs[runs["symbol"] == s].itertuples():
            la, fa = r.la_pos, r.fa_pos
            a = g["adj_final_corrected"].iloc[fa] / g["adj_final_corrected"].iloc[la] - 1
            b = g["final_price"].iloc[fa] / g["prev_final"].iloc[fa] - 1
            cc.append(abs(a - b))
    cc = np.array(cc)
    add("S4.15", "corrected adjusted-price chain reproduces the exchange reopen return across every halt",
        "PASS" if np.nanmax(cc) < 0.01 else "WARN", int((cc > 0.005).sum()),
        f"{len(cc)} halt runs; max deviation {np.nanmax(cc):.4f}; median {np.nanmedian(cc):.1e}")

    # share factor: product of 1/ratio over share events whose new shares arrive AFTER the row
    arr_pos: dict[str, list[tuple[int, float]]] = {s: [] for s in SYM}
    for r in ev[ev["share_event"] & (ev["action"] == "KEEP")].itertuples():
        S = SYM[r.symbol]
        k = int(np.searchsorted(S["ap"], r.pos, "left"))
        rank = int(r.share_arrival_rank)
        apos = int(S["ap"][min(k + rank, len(S["ap"]) - 1)]) if rank > 0 else int(r.pos)
        arr_pos[r.symbol].append((apos, float(r.inv_ratio)))
    sf = np.ones(len(df))
    uncertain = np.zeros(len(df), dtype=bool)
    for s, S in SYM.items():
        idx = df.index[df["symbol"] == s].values
        f = np.ones(S["n"])
        for apos, step in arr_pos[s]:
            f[:apos] *= step
        sf[idx] = f
    for r in ev[ev["arrival"] == "delayed"].itertuples():
        S = SYM[r.symbol]
        k = int(np.searchsorted(S["ap"], r.pos, "left"))
        a = int(S["ap"][min(k + int(r.uncertain_from_rank), len(S["ap"]) - 1)]) if r.uncertain_from_rank > 0 else int(r.pos)   # band starts at the event
        b = int(S["ap"][min(k + int(r.uncertain_to_rank), len(S["ap"]) - 1)])
        m = (df["symbol"] == r.symbol) & (df["pos"] >= a) & (df["pos"] <= b)
        uncertain |= m.values
    df["share_factor"] = sf
    df["adj_volume"] = df["volume"] * df["share_factor"]
    df["volume_adj_uncertain"] = uncertain
    df["adj_volume_m1_by_adj_factor"] = df["volume"] / df["adj_factor"]
    for c in ["ind_buy_vol", "ind_sell_vol", "corp_buy_vol", "corp_sell_vol"]:
        df[c + "_adj"] = df[c] * df["share_factor"]

    # sensitivity of the default: ambiguous events treated as dividends (no share change) instead
    sf_alt = np.ones(len(df))
    for s, S in SYM.items():
        idx = df.index[df["symbol"] == s].values
        f = np.ones(S["n"])
        for r in ev[(ev["symbol"] == s) & ev["share_event"] & (ev["action"] == "KEEP") & ~ev["event_class"].str.startswith("AMBIGUOUS")].itertuples():
            k = int(np.searchsorted(S["ap"], r.pos, "left"))
            rank = int(r.share_arrival_rank)
            apos = int(S["ap"][min(k + rank, len(S["ap"]) - 1)]) if rank > 0 else int(r.pos)
            f[:apos] *= r.inv_ratio
        sf_alt[idx] = f
    df["share_factor_alt"] = sf_alt
    d_log = np.abs(np.log(df["share_factor"] / df["share_factor_alt"]))
    n_amb = int(ev["event_class"].str.startswith("AMBIGUOUS").sum())
    add("S4.18", "sensitivity: treat the ambiguous ratio<0.9 events as dividends (no share change) instead of share increases",
        "INFO", int((d_log > 0.1).sum()),
        f"{n_amb} ambiguous events; adj_volume would change by >10% on {int((d_log > 0.1).sum())} records "
        f"({(d_log > 0.1).mean():.0%}), by >25% on {int((d_log > 0.25).sum())}; max {np.exp(d_log.max()):.2f}x. "
        f"Median effect {np.exp(d_log[d_log > 0].median()):.2f}x on affected records")

    ser_cols = ["symbol", "date", "is_halted", "volume", "value", "adj_factor", "adj_factor_corrected", "adj_final_corrected",
                "price_anomaly", "share_factor", "adj_volume", "volume_adj_uncertain", "adj_volume_m1_by_adj_factor",
                "ind_buy_vol_adj", "ind_sell_vol_adj", "corp_buy_vol_adj", "corp_sell_vol_adj"]
    save_table(df[ser_cols], OUT / "adjusted_series.csv")
    add("S4.16", "records whose adj_volume is uncertain (new shares of a delayed share increase arrive somewhere in the band)",
        "INFO", int(df["volume_adj_uncertain"].sum()),
        f"{int(df['volume_adj_uncertain'].sum())} records flagged volume_adj_uncertain=True")

    # ====================================================================== 6. compare volume-adjustment methods
    W_CENTER = np.array([k + N_WIN / 2 for k in OFFSETS])
    ev_eval = ev[(ev["cls_special"] == "")].copy()
    meth_rows = []
    group_defs = {
        "confirmed share increases": ev_eval["event_class"].isin(["SHARE_INCREASE_IMMEDIATE", "SHARE_INCREASE_DELAYED"]),
        "probable/ambiguous (ratio<0.9)": ev_eval["event_class"].isin(["SHARE_INCREASE_PROBABLE", "AMBIGUOUS_SHARE_OR_DIVIDEND", "AMBIGUOUS_NO_VOLUME_STEP"]),
        "price-only (ratio>=0.9)": ev_eval["event_class"] == "PRICE_ONLY_DIVIDEND_LIKE",
    }
    group_defs["all events"] = pd.Series(True, index=ev_eval.index)
    for gname, gm in group_defs.items():
        sub = ev_eval[gm]
        res = {m: [] for m in ["M0 raw volume", "M1 volume / adj_factor", "M2 volume x 1/ratio if ratio<0.9",
                               "M2t timing-aware share factor (chosen)", "M3 traded value (Rial)"]}
        for r in sub.itertuples():
            for w in range(len(OFFSETS)):
                x = getattr(r, f"ex{w}")
                if not np.isfinite(x):
                    continue
                e = r.exp_log
                res["M0 raw volume"].append(x)
                res["M1 volume / adj_factor"].append(x - e)
                res["M2 volume x 1/ratio if ratio<0.9"].append(x - (e if r.adj_ratio < SHARE_RATIO_MAX else 0.0))
                applied = r.share_event and (W_CENTER[w] > r.share_arrival_rank)
                res["M2t timing-aware share factor (chosen)"].append(x - (e if applied else 0.0))
                res["M3 traded value (Rial)"].append(getattr(r, f"exv{w}"))
        for m, v in res.items():
            v = np.array(v, dtype=float)
            v = v[np.isfinite(v)]
            meth_rows.append(dict(group=gname, method=m, n_obs=len(v), mean_residual_log=v.mean() if len(v) else np.nan,
                                  rmse_log=float(np.sqrt((v ** 2).mean())) if len(v) else np.nan))
    METH = pd.DataFrame(meth_rows)
    METH["placebo_noise_sd"] = float(SIG.mean())
    save_table(METH, OUT / "volume_method_comparison.csv")
    allg = METH[METH["group"] == "confirmed share increases"].set_index("method")
    add("S4.17", "volume-adjustment methods: residual jump at confirmed share-increase events (log units; placebo noise "
        f"sd = {SIG.mean():.2f})", "INFO", None,
        "; ".join(f"{m}: mean {r.mean_residual_log:+.2f}, rmse {r.rmse_log:.2f}" for m, r in allg.iterrows()))

    # ====================================================================== 7. save event tables
    final_cols = ["event_id", "symbol", "date", "date_jalali", "jal_month", "adj_ratio", "inv_ratio", "inv_ratio_is_round", "in_dividend_season", "on_halted_row", "halt_symbol_days",
                  "halt_records", "cluster_id", "n_in_cluster", "cluster_net_ratio", "adj_ret_across_halt", "ret_pctile_vs_nonevent", "price_outlier",
                  "n_pre", "exp_log", "ex0", "ex1", "ex2", "ex3", "ex4", "exv0", "z0_vs_expected", "z0_vs_none", "best_j", "support", "power",
                  "amp_hat", "volume_test", "event_class", "confidence", "share_event", "arrival", "share_arrival_rank", "share_step", "action",
                  "external_check"]
    ev_out = ev[final_cols].copy()
    ev_out["date"] = ev_out["date"].dt.date
    ev_out = ev_out.sort_values(["date", "symbol"]).reset_index(drop=True) if False else ev_out
    rv = pd.read_csv(OUT.parent / "step2_consistency" / "adj_event_review.csv", encoding="utf-8-sig")
    rv["date"] = pd.to_datetime(rv["date"]).dt.date
    ev_out = ev_out.merge(rv[["symbol", "date", "review_flags"]].rename(columns={"review_flags": "step2_flags"}), on=["symbol", "date"], how="left")
    save_table(ev_out, OUT / "adj_event_final.csv")
    save_table(pd.DataFrame(net_rows), OUT / "adj_events_corrected_net_events.csv")
    chk_list = ev_out[ev_out["external_check"] != ""][["event_id", "symbol", "date", "adj_ratio", "event_class", "confidence", "external_check"]]
    save_table(chk_list, OUT / "events_to_verify_externally.csv")
    corr_list = [dict(symbol=s, date=SYM[s]["g"]["date"].iloc[p].date(), ratio=r) for s, lst in corr_events.items() for p, r in lst]
    save_table(pd.DataFrame(corr_list).sort_values(["symbol", "date"]), OUT / "adj_events_corrected.csv")
    save_table(ev[["event_id", "symbol", "date"] + [f"{p}{w}" for w in range(len(OFFSETS)) for p in ("lv", "mv", "ex", "lval", "mu", "exv")]],
               OUT / "volume_paths.csv")

    # ====================================================================== 8. charts (English labels: Persian shaping libs are optional)
    plt.rcParams.update({"font.size": 9})
    colors = {"SHARE_INCREASE_IMMEDIATE": "#1b7837", "SHARE_INCREASE_DELAYED": "#762a83", "SHARE_INCREASE_PROBABLE": "#5aae61",
              "AMBIGUOUS_SHARE_OR_DIVIDEND": "#e08214", "AMBIGUOUS_NO_VOLUME_STEP": "#b2182b", "PRICE_ONLY_DIVIDEND_LIKE": "#8c8c8c"}
    fig, ax = plt.subplots(figsize=(7.5, 6))
    for c, col in colors.items():
        s = ev[(ev["event_class"] == c) & ev["ex0"].notna()]
        ax.scatter(s["exp_log"], s["ex0"], s=34, color=col, label=f"{c} (n={len(s)})", edgecolor="white", linewidth=0.5)
    for r in ev[(ev["exp_log"] > 0.55) & ev["ex0"].notna()].itertuples():
        ax.annotate(r.event_id, (r.exp_log, r.ex0), fontsize=7, xytext=(3, 3), textcoords="offset points")
    lim = [-1.3, 3.2]
    ax.plot(lim, lim, color="black", lw=0.8, ls="--", label="observed = expected (y = x)")
    ax.axhline(0, color="#999", lw=0.6)
    ax.axhspan(-SIG[0], SIG[0], color="#dddddd", alpha=0.4, label="placebo noise +/-1 sd")
    ax.set_xlim(-0.1, 3.0)
    ax.set_ylim(-1.3, 3.2)
    ax.set_xlabel("expected log volume step = log(1 / adj_ratio)")
    ax.set_ylabel("observed market-adjusted log volume step, window +3..+23")
    ax.set_title("Volume step after an adjustment event vs the exchange ratio")
    ax.legend(fontsize=7, loc="upper left")
    fig.tight_layout()
    fig.savefig(OUT / "volume_step_vs_expected.png", dpi=150)
    plt.close(fig)

    big = ev[(ev["exp_log"] >= 0.5) & (ev["cls_special"] == "") & (ev["n_windows"] >= 3)].sort_values("exp_log", ascending=False)
    ncol = 4
    nrow = int(np.ceil(len(big) / ncol))
    fig, axes = plt.subplots(nrow, ncol, figsize=(12, 2.6 * nrow), sharex=True)
    axes = np.atleast_2d(axes).ravel()
    xs = np.array([k + N_WIN / 2 for k in OFFSETS])
    for ax, r in zip(axes, big.itertuples()):
        y = np.array([getattr(r, f"ex{w}") for w in range(len(OFFSETS))], dtype=float)
        ax.plot(xs, y, "o-", color="#2166ac")
        ax.fill_between(xs, -SIG, SIG, color="#dddddd", alpha=0.5)
        ax.axhline(r.exp_log, color="#b2182b", ls="--", lw=1)
        ax.axhline(0, color="#999", lw=0.5)
        ax.set_title(f"{r.event_id} ratio {r.adj_ratio:.3f}\n{r.event_class.replace('SHARE_INCREASE_', '').replace('AMBIGUOUS_', 'AMB_')}", fontsize=8)
    for ax in axes[len(big):]:
        ax.axis("off")
    for ax in axes[-ncol:]:
        ax.set_xlabel("active rows after the event")
    fig.suptitle("Market-adjusted log volume path (blue) vs expected step (red dashed); grey band = placebo noise", fontsize=10)
    fig.tight_layout()
    fig.savefig(OUT / "volume_timing_paths.png", dpi=150)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(8, 4.8))
    order = ["1", "2-3", "4-10", ">10"]
    rng2 = np.random.default_rng(1)
    for k, b in enumerate(order):
        s = base[base["len_bin"] == b]
        ax.scatter(k + rng2.uniform(-0.25, 0.25, len(s)), s["abs_ret"].clip(lower=1e-3), s=10, color="#bbbbbb", label="no event" if k == 0 else None)
        e = ev[(ev["halt_symbol_days"].notna())]
        e = e.assign(b=pd.cut(e["halt_symbol_days"], [-1, 0, 1, 3, 10, 10_000], labels=["0", "1", "2-3", "4-10", ">10"]).astype(str))
        e = e[e["b"] == b]
        ax.scatter(k + rng2.uniform(-0.25, 0.25, len(e)), e["adj_ret_across_halt"].abs().clip(lower=1e-3), s=22, color="#b2182b", label="adjustment event" if k == 0 else None)
    ax.set_xticks(range(len(order)))
    ax.set_xticklabels(order)
    ax.set_yscale("log")
    ax.set_xlabel("halt length (symbol-specific days)")
    ax.set_ylabel("|adjusted return across the halt| (floored at 0.1%)")
    ax.set_title("Price-side check: halts that contain an adjustment event vs ordinary halts")
    ax.legend()
    fig.tight_layout()
    fig.savefig(OUT / "price_side_check.png", dpi=150)
    plt.close(fig)

    # ====================================================================== 9. wrap up
    chk = pd.DataFrame(checks).sort_values("id").reset_index(drop=True)
    save_table(chk, OUT / "checks_summary.csv")
    summary = dict(n_events=int(len(ev)), classes=cls_counts, share_events=int(ev["share_event"].sum()),
                   delayed=[f"{r.symbol} {r.date.date()}" for r in delayed.itertuples()],
                   uncertain_records=int(df["volume_adj_uncertain"].sum()), price_anomaly_rows=int(df["price_anomaly"].sum()),
                   pooled_slope_ratio_lt_09=float(g_lo["slope_through_origin"]), placebo_sd=SIG.tolist())
    (OUT / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    print(chk[["id", "status", "n_issues", "detail"]].to_string(max_colwidth=200))
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
