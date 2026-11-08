"""Shared helpers for the EDA pipeline (paths, loading, Persian text helpers)."""
from __future__ import annotations

import re
from pathlib import Path

import pandas as pd

ROOT = Path(__file__).resolve().parents[1]
RAW_DIR = ROOT / "data" / "raw"
OUT_DIR = ROOT / "outputs"

DAILY_CSV = RAW_DIR / "underlyings_daily.csv"
META_CSV = RAW_DIR / "underlyings.csv"
REPORT_MD = RAW_DIR / "quality_report.md"

RAW_PRICE_COLS = ["open", "high", "low", "last_price", "final_price"]
ADJ_PRICE_COLS = ["adj_open", "adj_high", "adj_low", "adj_last", "adj_final"]
IND_COLS = ["ind_buy_vol", "ind_sell_vol", "ind_buy_val", "ind_sell_val",
            "ind_buy_count", "ind_sell_count"]
CORP_COLS = ["corp_buy_vol", "corp_sell_vol", "corp_buy_val", "corp_sell_val",
             "corp_buy_count", "corp_sell_count"]
FLOW_COLS = IND_COLS + CORP_COLS + ["ind_net_value", "corp_net_value"]

# Iranian market week is Saturday..Wednesday. pandas weekday: Mon=0 ... Sun=6
MARKET_WEEKDAYS = {5, 6, 0, 1, 2}  # Sat, Sun, Mon, Tue, Wed

_ARABIC_TO_PERSIAN = str.maketrans({"ي": "ی", "ك": "ک", "ى": "ی", "ة": "ه"})
_ZWNJ = "‌"


def normalize_fa(text: str) -> str:
    """Unify Arabic/Persian letter variants and whitespace (ي->ی, ك->ک, ZWNJ->space)."""
    if text is None:
        return text
    text = str(text).translate(_ARABIC_TO_PERSIAN).replace(_ZWNJ, " ")
    return re.sub(r"\s+", " ", text).strip()


def load_daily() -> pd.DataFrame:
    """Load the daily file, parse dates, sort by (symbol, date). Raw symbol text is kept as-is."""
    df = pd.read_csv(DAILY_CSV, encoding="utf-8-sig")
    df["date"] = pd.to_datetime(df["date"])
    df = df.sort_values(["symbol", "date"]).reset_index(drop=True)
    return df


def load_meta() -> pd.DataFrame:
    meta = pd.read_csv(META_CSV, encoding="utf-8-sig")
    meta["first_date"] = pd.to_datetime(meta["first_date"])
    meta["last_date"] = pd.to_datetime(meta["last_date"])
    return meta


def market_calendar(df: pd.DataFrame) -> pd.DatetimeIndex:
    """Union of all dates present in the data = the observed market calendar."""
    return pd.DatetimeIndex(sorted(df["date"].unique()))


CLOSURE_SHARE = 0.90            # share of halted symbols that makes a day "market-wide closed"
MIN_SYMBOLS_FOR_CLOSURE = 10


def market_closure_days(df: pd.DataFrame) -> pd.DatetimeIndex:
    """Days on which >= CLOSURE_SHARE of the symbols with a record are halted (same rule as step 1)."""
    per_day = df.groupby("date").agg(n_rec=("symbol", "size"), n_halted=("is_halted", "sum"))
    share = per_day["n_halted"] / per_day["n_rec"]
    return per_day.index[(per_day["n_rec"] >= MIN_SYMBOLS_FOR_CLOSURE) & (share >= CLOSURE_SHARE)]


def out_dir(step_name: str) -> Path:
    path = OUT_DIR / step_name
    path.mkdir(parents=True, exist_ok=True)
    return path


def save_table(df: pd.DataFrame, path: Path, index: bool = False) -> None:
    """utf-8-sig so Excel opens Persian text correctly."""
    df.to_csv(path, index=index, encoding="utf-8-sig")


def fa_label(text: str) -> str:
    """Shape + reorder Persian text for matplotlib. Falls back to raw text if libs are missing."""
    try:
        import arabic_reshaper
        from bidi.algorithm import get_display
        return get_display(arabic_reshaper.reshape(normalize_fa(text)))
    except Exception:  # pragma: no cover
        return str(text)


def parse_md_table(md_text: str, section_regex: str) -> pd.DataFrame:
    """Parse the first markdown table that appears after a '## ...' header matching section_regex."""
    lines = md_text.splitlines()
    start = None
    for i, line in enumerate(lines):
        if line.startswith("##") and re.search(section_regex, line):
            start = i
            break
    if start is None:
        raise ValueError(f"section not found: {section_regex}")
    rows = []
    for line in lines[start + 1:]:
        if line.startswith("##"):
            break
        if line.startswith("|"):
            rows.append([c.strip() for c in line.strip().strip("|").split("|")])
    header, body = rows[0], [r for r in rows[2:]]
    return pd.DataFrame(body, columns=header)


def run_steps(modules) -> None:
    """Run the main() of each step module in order and print timing (used by the phase scripts)."""
    import time

    for m in modules:
        t0 = time.time()
        print(f"\n=== {m.__name__} ===", flush=True)
        m.main()
        print(f"--- {m.__name__} finished in {time.time() - t0:.1f}s", flush=True)
