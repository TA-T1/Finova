"""Phase 3 - symbol profile and returns (steps 5, 6a, 6b): profile, return/feature panel, distribution and risk statistics.

Run from the repository root:  python scripts/03_profile_and_returns.py
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import common
import step5_symbol_profile
import step6a_returns_features
import step6b_distribution_risk


def main() -> None:
    common.run_steps([step5_symbol_profile, step6a_returns_features, step6b_distribution_risk])


if __name__ == "__main__":
    main()
