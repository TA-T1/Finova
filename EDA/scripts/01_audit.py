"""Phase 1 - data audit (steps 0-3): load and reconcile, structure, price/volume consistency, halts and gaps.

Run from the repository root:  python scripts/01_audit.py
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import common
import step0_load_overview
import step1_structural_validation
import step2_price_volume_consistency
import step3_halts_and_gaps


def main() -> None:
    common.run_steps([step0_load_overview, step1_structural_validation, step2_price_volume_consistency, step3_halts_and_gaps])


if __name__ == "__main__":
    main()
