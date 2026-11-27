"""feature engineering for underlying stocks: builds the `features` table and proves that no feature reads future data.

Run from the EDA folder (after EDA scripts 01-04):
    python scripts/05_features.py                       # parquet + csv in outputs/features/
    python scripts/05_features.py --db sqlite:///features.db   # also load the table `features` into a database
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "features"))

import pandas as pd

import lookahead_check
import feature_engineering
from common import out_dir


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default=None, help="SQLAlchemy URL; if given the table `features` is (re)created there")
    a = ap.parse_args()
    feature_engineering.main(a.db)
    res = lookahead_check.run_all(feature_engineering.build_panel())
    rep = pd.DataFrame([
        {"test": "cut the future (truncate data, features of past days unchanged)", "status": "PASS" if not res["cut"] else "FAIL", "detail": str(res["cut"])},
        {"test": "perturb the future (scramble later data, past features unchanged)", "status": "PASS" if not res["perturb"] else "FAIL", "detail": str(res["perturb"])},
        {"test": "sanity: a deliberately leaky feature is caught", "status": "PASS" if res["leaky_detected"] else "FAIL", "detail": ""},
    ])
    rep.to_csv(out_dir("features") / "lookahead_report.csv", index=False, encoding="utf-8-sig")
    print(rep.to_string(index=False))
    if (rep["status"] != "PASS").any():
        raise SystemExit(1)


if __name__ == "__main__":
    main()
