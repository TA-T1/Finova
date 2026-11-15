"""No-look-ahead checks for compute_features.
1) cut-the-future: features computed on data truncated at day T equal the full-data features for all days <= T.
2) perturb-the-future: scrambling all data after T leaves features for days <= T unchanged.
3) sanity: a deliberately leaky feature (uses shift(-1)) IS detected by the same test (so the test can fail).
"""
from __future__ import annotations

import numpy as np
import pandas as pd

from feature_engineering import FEATURES, compute_features

NAMES = list(FEATURES)


def _same(a: pd.DataFrame, b: pd.DataFrame, cols) -> list[str]:
    bad = []
    for c in cols:
        x, y = a[c].to_numpy(float), b[c].to_numpy(float)
        if not np.allclose(x, y, equal_nan=True, rtol=1e-9, atol=1e-12):
            bad.append(c)
    return bad


def check_cut(panel, cuts, features_fn=compute_features, cols=NAMES):
    full = features_fn(panel).set_index(["symbol", "date"])
    bad = {}
    for T in cuts:
        part = features_fn(panel[panel["date"] <= T]).set_index(["symbol", "date"])
        b = _same(full.loc[part.index], part, cols)
        if b:
            bad[str(T)[:10]] = b
    return bad


def check_perturb(panel, cuts, features_fn=compute_features, cols=NAMES, seed=0):
    rng = np.random.default_rng(seed)
    full = features_fn(panel).set_index(["symbol", "date"])
    bad = {}
    for T in cuts:
        p = panel.copy()
        fut = p["date"] > T
        for c in ["close", "high", "low", "value", "trade_count", "ind_buy_share", "ind_net_ratio", "ind_power_ratio"]:
            v = p.loc[fut, c]
            p.loc[fut, c] = v * rng.uniform(0.3, 3.0, len(v))
        part = features_fn(p).set_index(["symbol", "date"])
        idx = full.index[full.index.get_level_values("date") <= T]
        b = _same(full.loc[idx], part.loc[idx], cols)
        if b:
            bad[str(T)[:10]] = b
    return bad


def leaky_features(panel):
    out = compute_features(panel)
    out["leaky"] = panel.groupby("symbol")["close"].shift(-1).to_numpy() / panel["close"].to_numpy() - 1
    return out


def run_all(panel):
    dates = np.sort(panel["date"].unique())
    cuts = [dates[int(len(dates) * q)] for q in (0.3, 0.5, 0.7, 0.9)]
    res = {"cut": check_cut(panel, cuts), "perturb": check_perturb(panel, cuts)}
    leak = check_cut(panel, cuts, leaky_features, NAMES + ["leaky"])
    res["leaky_detected"] = all("leaky" in v for v in leak.values()) and len(leak) == len(cuts)
    return res
