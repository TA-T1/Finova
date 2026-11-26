import csv
import gzip
from datetime import date

from finova_backtest.data import load_dir


def write(path, rows):
    with gzip.open(path, "wt", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0]))
        w.writeheader()
        w.writerows(rows)


def test_load_dir_reads_a_backfill_export(tmp_path):
    write(tmp_path / "underlyings.csv.gz", [{"id": "1", "symbol": "خودرو"}])
    write(tmp_path / "underlying_daily_prices.csv.gz", [
        {"underlying_id": "1", "trade_date": "2026-11-22", "close": "2400", "final_price": "2390"},
        {"underlying_id": "1", "trade_date": "2026-11-23", "close": "2450", "final_price": ""},
    ])
    write(tmp_path / "quote_snapshots.csv.gz", [{
        "trade_date": "2026-11-23", "contract_symbol": "ضخود1", "underlying_symbol": "خودرو", "option_type": "call",
        "strike": "2000", "expiry": "2026-12-23", "close_price": "495", "last_price": "500", "bid_price": "0",
        "ask_price": "0", "volume": "7", "value": "0", "trade_count": "0", "open_interest": "0",
        "underlying_price": "", "captured_at": "2026-11-24 10:00:00+00"}])
    m = load_dir(tmp_path)
    assert m.underlyings() == ["خودرو"]
    q = m.quote("خودرو", date(2026, 12, 23), "call", 2000.0, date(2026, 11, 23))
    assert q.last == 500 and q.close == 495 and q.bid == 0 and q.underlying_price is None
    assert m.spot("خودرو", date(2026, 11, 23)) == 2450  # no snapshot price: the daily close
    assert m.finals_until("خودرو", date(2026, 11, 23), 5) == [2390, 2450]  # missing final -> close
    assert m.day_on_or_before("خودرو", date(2026, 11, 25)) == date(2026, 11, 23)
