"""Market history the engine reads: end-of-day option rows and underlying prices.

``MarketData`` is an in-memory index. It is filled from the tables of the
database (``load_db``) or from the demo dataset files written by
``go run ./cmd/backfill export`` (``load_dir``), so the engine runs offline.

Option rows come from ``quote_snapshots``. Rows that were backfilled from
TSETMC history have ``bid = ask = 0`` (unknown) and no open interest.
"""
from __future__ import annotations

import bisect
import csv
import gzip
import math
from collections import defaultdict
from dataclasses import dataclass
from datetime import date
from pathlib import Path
from typing import Iterable, Mapping


@dataclass(frozen=True)
class Quote:
    """One option contract on one trading day (a quote_snapshots row)."""
    symbol: str
    underlying: str
    type: str  # "call" | "put"
    strike: float
    expiry: date
    date: date
    close: float  # settlement price ("قیمت پایانی")
    last: float  # last traded price
    bid: float = 0.0  # 0 = unknown
    ask: float = 0.0
    volume: int = 0
    underlying_price: float | None = None


def _day(v) -> date:
    return v if isinstance(v, date) else date.fromisoformat(str(v)[:10])


def _num(v, default=0.0) -> float:
    return default if v in (None, "") else float(v)


class MarketData:
    def __init__(self, quotes: Iterable[Quote], daily: Mapping[str, Iterable[tuple[date, float, float]]]):
        """``daily`` maps an underlying symbol to ``(date, close, final_price)`` rows."""
        self._by_key: dict[tuple[str, date, str, float, date], Quote] = {}
        self._chain: dict[tuple[str, date, date], list[Quote]] = defaultdict(list)
        self._snap_dates: dict[str, set[date]] = defaultdict(set)
        self._expiries: dict[str, set[date]] = defaultdict(set)
        self._strikes: dict[str, set[float]] = defaultdict(set)
        self._under_px: dict[tuple[str, date], float] = {}
        for q in quotes:
            self._by_key[(q.underlying, q.expiry, q.type, q.strike, q.date)] = q
            self._chain[(q.underlying, q.expiry, q.date)].append(q)
            self._snap_dates[q.underlying].add(q.date)
            self._expiries[q.underlying].add(q.expiry)
            self._strikes[q.underlying].add(q.strike)
            if q.underlying_price:
                self._under_px[(q.underlying, q.date)] = q.underlying_price

        self._days: dict[str, list[date]] = {}
        self._close: dict[str, list[float]] = {}
        self._final: dict[str, list[float]] = {}
        for sym, rows in daily.items():
            rows = sorted(set(rows), key=lambda r: r[0])
            self._days[sym] = [r[0] for r in rows]
            self._close[sym] = [r[1] for r in rows]
            self._final[sym] = [r[2] or r[1] for r in rows]

    def with_daily(self, daily: Mapping[str, Iterable[tuple[date, float, float]]]) -> "MarketData":
        """A copy of this data whose underlying prices come from ``daily``."""
        quotes = [q for qs in self._chain.values() for q in qs]
        return MarketData(quotes, daily)

    # -- underlyings and dates -------------------------------------------------
    def underlyings(self) -> list[str]:
        return sorted(set(self._days) | set(self._expiries))

    def trading_days(self, underlying: str) -> list[date]:
        """Trading days of the underlying (its own price history, else the snapshot days)."""
        return self._days.get(underlying) or sorted(self._snap_dates.get(underlying, ()))

    def first_day(self, underlying: str) -> date | None:
        d = self.trading_days(underlying)
        return d[0] if d else None

    def last_day(self, underlying: str) -> date | None:
        d = self.trading_days(underlying)
        return d[-1] if d else None

    def day_on_or_before(self, underlying: str, day: date) -> date | None:
        days = self.trading_days(underlying)
        i = bisect.bisect_right(days, day)
        return days[i - 1] if i else None

    def days_between(self, underlying: str, after: date, upto: date) -> list[date]:
        """Trading days d with ``after < d <= upto``."""
        days = self.trading_days(underlying)
        return days[bisect.bisect_right(days, after):bisect.bisect_right(days, upto)]

    # -- prices ------------------------------------------------------------------
    def spot(self, underlying: str, day: date) -> float | None:
        """Underlying last price on ``day``: the snapshot's value, else the daily close."""
        if (underlying, day) in self._under_px:
            return self._under_px[(underlying, day)]
        days = self._days.get(underlying)
        if days:
            i = bisect.bisect_left(days, day)
            if i < len(days) and days[i] == day and self._close[underlying][i] > 0:
                return self._close[underlying][i]
        return None

    def finals_until(self, underlying: str, day: date, n: int) -> list[float]:
        """The last ``n`` final prices up to and including ``day``, oldest first."""
        days = self._days.get(underlying, [])
        i = bisect.bisect_right(days, day)
        return self._final[underlying][max(0, i - n):i]

    def max_abs_log_return(self, underlying: str, after: date, upto: date) -> float:
        """Largest daily |log return| of the underlying's final price from the day
        after ``after`` through ``upto`` (0.0 when there is nothing to compare)."""
        days = self._days.get(underlying, [])
        lo, hi = bisect.bisect_right(days, after), bisect.bisect_right(days, upto)
        px = self._final[underlying][max(lo - 1, 0):hi]
        worst = 0.0
        for a, b in zip(px, px[1:]):
            if a > 0 and b > 0:
                worst = max(worst, abs(math.log(b / a)))
        return worst

    # -- option rows ---------------------------------------------------------------
    def quote(self, underlying: str, expiry: date, typ: str, strike: float, day: date) -> Quote | None:
        return self._by_key.get((underlying, expiry, typ, strike, day))

    def chain_quotes(self, underlying: str, expiry: date, day: date) -> list[Quote]:
        return self._chain.get((underlying, expiry, day), [])

    def observed_expiries(self, underlying: str) -> list[date]:
        return sorted(self._expiries.get(underlying, ()))

    def observed_strikes(self, underlying: str) -> list[float]:
        return sorted(self._strikes.get(underlying, ()))


# -- loading ------------------------------------------------------------------------

def _rows_to_data(snapshots: Iterable[Mapping], daily: Iterable[Mapping]) -> MarketData:
    quotes = [
        Quote(
            symbol=r["contract_symbol"], underlying=r["underlying_symbol"], type=r["option_type"],
            strike=_num(r["strike"]), expiry=_day(r["expiry"]), date=_day(r["trade_date"]),
            close=_num(r["close_price"]), last=_num(r["last_price"]),
            bid=_num(r["bid_price"]), ask=_num(r["ask_price"]), volume=int(_num(r["volume"])),
            underlying_price=_num(r["underlying_price"]) or None,
        )
        for r in snapshots
    ]
    by_sym: dict[str, list[tuple[date, float, float]]] = defaultdict(list)
    for r in daily:
        by_sym[r["symbol"]].append((_day(r["trade_date"]), _num(r["close"]), _num(r["final_price"])))
    return MarketData(quotes, by_sym)


def _read_csv_gz(path: Path):
    with gzip.open(path, "rt", encoding="utf-8", newline="") as f:
        yield from csv.DictReader(f)


def load_prices_csv(path: str | Path) -> dict[str, list[tuple[date, float, float]]]:
    """Back-adjusted daily prices from ``data-pipeline/output/underlyings_daily.csv``.

    A backtest should use these: the raw series that the database holds has one-day
    jumps at capital increases and dividends. Returns ``{symbol: [(date, last, final)]}``
    from the ``adj_last`` and ``adj_final`` columns.
    """
    out: dict[str, list[tuple[date, float, float]]] = defaultdict(list)
    with open(path, encoding="utf-8-sig", newline="") as f:
        for r in csv.DictReader(f):
            last, final = _num(r["adj_last"]), _num(r["adj_final"])
            if last > 0 or final > 0:
                out[r["symbol"]].append((_day(r["date"]), last or final, final or last))
    return dict(out)


def load_dir(directory: str | Path, prices_csv: str | Path | None = None) -> MarketData:
    """Load a demo dataset directory (``*.csv.gz`` from ``backfill export``).

    With ``prices_csv`` (see ``load_prices_csv``) the underlying prices come from
    that adjusted file instead of the database's raw ``underlying_daily_prices``.
    """
    d = Path(directory)
    names = {r["id"]: r["symbol"] for r in _read_csv_gz(d / "underlyings.csv.gz")}
    daily = ({**r, "symbol": names[r["underlying_id"]]}
             for r in _read_csv_gz(d / "underlying_daily_prices.csv.gz") if r["underlying_id"] in names)
    data = _rows_to_data(_read_csv_gz(d / "quote_snapshots.csv.gz"), daily)
    if prices_csv:
        data = data.with_daily(load_prices_csv(prices_csv))
    return data


def load_db(url: str) -> MarketData:
    """Load from PostgreSQL (needs ``pip install psycopg[binary]``)."""
    import psycopg  # imported here so the engine works without it

    with psycopg.connect(url) as conn:
        cur = conn.cursor()
        cur.execute("SELECT * FROM quote_snapshots")
        cols = [c.name for c in cur.description]
        snaps = [dict(zip(cols, row)) for row in cur.fetchall()]
        cur.execute("""SELECT u.symbol, p.trade_date, p.close, p.final_price
                       FROM underlying_daily_prices p JOIN underlyings u ON u.id = p.underlying_id""")
        daily = [dict(symbol=s, trade_date=d, close=c, final_price=f) for s, d, c, f in cur.fetchall()]
    return _rows_to_data(snaps, daily)
