"""Shared loading for the analytical report: demo dataset + adjusted prices, with Arabic/Persian letters unified."""
from __future__ import annotations

import sys
from pathlib import Path

import os

ROOT = Path(__file__).resolve().parents[1]
# Put this folder at the root of the Finova repository (next to backtest/, backend/, data-pipeline/), or set FINOVA_REPO.
REPO = Path(os.environ.get("FINOVA_REPO", ROOT.parent))
ENGINE = REPO / "backtest"                                  # task 8 (branch feat/data-backtest-engine)
DEMO = REPO / "backend" / "testdata" / "demo"               # task 7 (branch feat/crawl-history-backfill)
PRICES = REPO / "data-pipeline" / "output" / "underlyings_daily.csv"
FEATURES = Path(os.environ.get("FEATURES_PARQUET", REPO / "EDA" / "outputs" / "features" / "features.parquet"))  # features task output
OUT, FIG = ROOT / "outputs", ROOT / "figures"
sys.path.insert(0, str(ENGINE))

from collections import defaultdict  # noqa: E402

from finova_backtest import data as D  # noqa: E402

_MAP = str.maketrans({"ي": "ی", "ك": "ک", "ۀ": "ه", "ة": "ه"})


def fa(s: str) -> str:
    return s.translate(_MAP).strip()


def load_market():
    names = {r["id"]: fa(r["symbol"]) for r in D._read_csv_gz(DEMO / "underlyings.csv.gz")}
    snaps = []
    for r in D._read_csv_gz(DEMO / "quote_snapshots.csv.gz"):
        r["underlying_symbol"], r["contract_symbol"] = fa(r["underlying_symbol"]), fa(r["contract_symbol"])
        snaps.append(r)
    daily = defaultdict(list)
    import csv
    with open(PRICES, encoding="utf-8-sig", newline="") as f:
        for r in csv.DictReader(f):
            last, final = D._num(r["adj_last"]), D._num(r["adj_final"])
            if last > 0 or final > 0:
                daily[fa(r["symbol"])].append((D._day(r["date"]), last or final, final or last))
    return D._rows_to_data(snaps, []).with_daily(dict(daily)), snaps
