"""Phase 2 - corporate-action adjustments (step 4): event review, corrected adjustment factor, volume adjustment.

Run from the repository root:  python scripts/02_adjustments.py
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import common
import step4_adjustment_events


def main() -> None:
    common.run_steps([step4_adjustment_events])


if __name__ == "__main__":
    main()
