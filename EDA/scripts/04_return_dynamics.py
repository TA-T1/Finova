"""Phase 4 - return dynamics (step 6c): autocorrelation, volatility clustering, weekday effects, next-day predictive power.

Run from the repository root:  python scripts/04_return_dynamics.py
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import common
import step6c_return_dynamics


def main() -> None:
    common.run_steps([step6c_return_dynamics])


if __name__ == "__main__":
    main()
