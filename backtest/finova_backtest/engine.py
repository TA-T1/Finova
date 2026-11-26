"""Strategy backtest engine.

A *rule* says: on each past expiry of an underlying, ``entry_days`` calendar days
before it, open a strategy built from a template (for example a bull call
spread one strike apart), then close it at expiry or earlier on a profit target
or a stop. The engine prices every leg, settles the position, charges fees and
returns per-expiry results plus summary metrics and an equity curve.

Prices
------
Each leg is priced from ``quote_snapshots`` when that day has a usable row. When
it has not (an old expiry, an unlisted strike), the leg is priced with
Black-Scholes at the underlying's HV20 on that day, and the trade is marked
``price_source = "model"`` (or ``"mixed"``). A run can be forced to snapshot-only
prices with ``allow_model=False``.

Equity
------
Each trade risks ``allocation`` of the starting equity: it adds
``allocation * pnl / capital_at_risk`` to the equity (capital at risk = the
strategy's maximum loss at expiry, from its payoff). Trades are added in the order
they close (the account cannot go below 0), without compounding, because weekly expiries overlap and compounding
overlapping trades would count the same money twice. Equity starts at 1.0.
"""
from __future__ import annotations

import math
from dataclasses import asdict, dataclass, field
from datetime import date, timedelta
from typing import Sequence

from .data import MarketData, Quote
from .pricing import DAYS_PER_YEAR, DEFAULT_RATE, MAX_PLAUSIBLE_RETURN, bs_price, hv20, intrinsic
from .templates import TEMPLATES, Template, pick_strike

EXIT_EXPIRY = "expiry"


@dataclass(frozen=True)
class Fees:
    """Proportional fees, as a fraction of the traded value. These are placeholders
    close to the Tehran tariff, not a broker's real numbers: replace them."""
    option_commission: float = 0.001  # per side, on premium x size x quantity
    exercise_fee: float = 0.001  # on strike x size x quantity, for options that settle in the money
    share_buy: float = 0.004
    share_sell: float = 0.009  # includes the sales tax


@dataclass(frozen=True)
class Rule:
    template_id: str
    entry_days: int = 30  # calendar days before expiry
    quantity: int = 1  # contracts per leg unit
    exit: str = EXIT_EXPIRY  # "expiry" or "target_stop"
    target_pct: float | None = None  # close when pnl >= target_pct * capital_at_risk
    stop_pct: float | None = None  # close when pnl <= -stop_pct * capital_at_risk
    fill: str = "last"  # "last" | "mid" | "cross"
    allocation: float = 0.05  # fraction of the starting equity risked per trade
    contract_size: int = 1000
    rate: float = DEFAULT_RATE
    allow_model: bool = True
    min_premium: float = 1.0  # rials per share: the exchange's smallest price step; cheaper legs are not tradable
    max_jump: float = MAX_PLAUSIBLE_RETURN  # skip a trade if the underlying jumps more than this in one day while it is open
    max_entry_gap_days: int = 5  # the entry day may be at most this many days before the target day
    fees: Fees = field(default_factory=Fees)

    def __post_init__(self):
        if self.template_id not in TEMPLATES:
            raise ValueError(f"unknown template {self.template_id!r}; known: {sorted(TEMPLATES)}")
        if self.entry_days < 1:
            raise ValueError("entry_days must be >= 1")
        if self.exit not in (EXIT_EXPIRY, "target_stop"):
            raise ValueError("exit must be 'expiry' or 'target_stop'")
        if self.exit == "target_stop" and self.target_pct is None and self.stop_pct is None:
            raise ValueError("exit 'target_stop' needs target_pct and/or stop_pct")
        if self.fill not in ("last", "mid", "cross"):
            raise ValueError("fill must be 'last', 'mid' or 'cross'")
        if not (0 < self.allocation <= 1):
            raise ValueError("allocation must be in (0, 1]")
        if self.quantity < 1:
            raise ValueError("quantity must be >= 1")

    @property
    def template(self) -> Template:
        return TEMPLATES[self.template_id]


@dataclass
class OpenLeg:
    type: str  # "call" | "put" | "underlying"
    side: str
    qty: float  # option contracts, or shares for an underlying leg
    strike: float | None
    entry_price: float
    price_source: str  # "snapshot" | "model"

    @property
    def sign(self) -> int:
        return 1 if self.side == "buy" else -1


@dataclass
class Trade:
    expiry: date
    entry_date: date
    exit_date: date
    exit_reason: str  # "expiry" | "target" | "stop"
    spot_entry: float
    spot_exit: float
    legs: list[dict]
    net_premium: float  # rials; > 0 = debit paid, < 0 = credit received
    capital_at_risk: float
    max_profit: float | None
    max_loss: float | None
    gross_pnl: float
    fees: float
    pnl: float
    return_on_risk: float
    price_source: str  # "snapshot" | "model" | "mixed"
    chain_source: str  # where the strikes came from


@dataclass
class Skipped:
    expiry: date
    reason: str


@dataclass
class Result:
    underlying: str
    rule: Rule
    trades: list[Trade]
    skipped: list[Skipped]
    metrics: dict
    equity_curve: list[dict]  # [{"date": "YYYY-MM-DD", "equity": float}]


# --- pricing helpers ----------------------------------------------------------------

def fill_price(q: Quote, side: str, mode: str) -> float | None:
    """Price to trade ``q`` at, or None when the row has no usable price."""
    if mode == "cross" and q.bid > 0 and q.ask > 0:
        return q.ask if side == "buy" else q.bid
    if mode == "mid" and q.bid > 0 and q.ask > 0 and q.bid <= q.ask:
        return (q.bid + q.ask) / 2
    if q.last > 0:
        return q.last
    if q.close > 0:
        return q.close
    return None


def _mark_price(q: Quote) -> float | None:
    return q.last if q.last > 0 else (q.close if q.close > 0 else None)


def _model_price(data: MarketData, underlying: str, typ: str, strike: float, expiry: date,
                 day: date, spot: float, rate: float) -> float | None:
    sigma = hv20(data.finals_until(underlying, day, 21))
    if not sigma or sigma <= 0:
        return None
    t = (expiry - day).days / DAYS_PER_YEAR
    return bs_price(typ, spot, strike, t, sigma, rate)


def _strike_step(strikes: Sequence[float]) -> float | None:
    s = sorted(set(strikes))
    diffs = [b - a for a, b in zip(s, s[1:]) if b > a]
    if not diffs:
        return None
    counts: dict[float, int] = {}
    for d in diffs:
        counts[d] = counts.get(d, 0) + 1
    return min(counts, key=lambda d: (-counts[d], d))


def _nice_step(x: float) -> float:
    """The nearest of 1, 2, 2.5, 5 times a power of ten (the usual strike steps)."""
    mag = 10 ** math.floor(math.log10(x))
    return min((m * mag for m in (1, 2, 2.5, 5, 10)), key=lambda c: abs(c - x))


def _model_strikes(data: MarketData, underlying: str, spot: float) -> list[float]:
    """A strike grid for a day with no listed chain. The step keeps the same
    proportion of the price as today's listed strikes do, rounded to a usual
    strike step, laid on multiples of that step around ``spot``."""
    observed = data.observed_strikes(underlying)
    step = _strike_step(observed)
    ref = observed[len(observed) // 2] if observed else None
    rel = step / ref if step and ref else 0.05
    step_d = _nice_step(rel * spot)
    centre = round(spot / step_d)
    return [k * step_d for k in range(max(centre - 8, 1), centre + 9)]


def payoff_at_expiry(legs: Sequence[OpenLeg], spot: float, size: int) -> float:
    """Gross P&L of the position if the underlying settles at ``spot`` (no fees)."""
    total = 0.0
    for l in legs:
        if l.type == "underlying":
            total += l.sign * l.qty * (spot - l.entry_price)
        else:
            total += l.sign * l.qty * size * (intrinsic(l.type, spot, l.strike) - l.entry_price)
    return total


def payoff_bounds(legs: Sequence[OpenLeg], spot0: float, size: int) -> tuple[float | None, float | None]:
    """(max profit, max loss) over every settlement price from 0 up. Max loss is a
    positive amount. A bound is None when it is unlimited."""
    strikes = [l.strike for l in legs if l.strike]
    far = max(strikes + [spot0]) * 10
    pts = sorted({0.0, far, *strikes})
    vals = [payoff_at_expiry(legs, p, size) for p in pts]
    # slope beyond the last strike: contributions of calls and shares
    slope = sum(l.sign * l.qty * (1 if l.type == "underlying" else size)
                for l in legs if l.type in ("call", "underlying"))
    best, worst = max(vals), min(vals)
    max_profit = None if slope > 0 else best
    max_loss = None if slope < 0 else max(-worst, 0.0)
    return max_profit, max_loss


def _leg_fee(rate: Fees, leg: OpenLeg, price: float, size: int, closing: bool) -> float:
    side = leg.side if not closing else ("sell" if leg.side == "buy" else "buy")
    if leg.type == "underlying":
        return leg.qty * price * (rate.share_buy if side == "buy" else rate.share_sell)
    return leg.qty * size * price * rate.option_commission


# --- the engine -----------------------------------------------------------------------

def expiries_weekly(start: date, end: date, weekday: int = 2) -> list[date]:
    """Every ``weekday`` (Monday = 0; Wednesday = 2) from ``start`` to ``end``: a
    synthetic expiry calendar for model-priced backtests of old periods."""
    d = start + timedelta(days=(weekday - start.weekday()) % 7)
    out = []
    while d <= end:
        out.append(d)
        d += timedelta(days=7)
    return out


def completed_expiries(data: MarketData, underlying: str) -> list[date]:
    """Observed expiries that have passed with data on the underlying's price history."""
    last = data.last_day(underlying)
    return [e for e in data.observed_expiries(underlying) if last and e <= last]


def run_backtest(data: MarketData, underlying: str, rule: Rule, expiries: Sequence[date] | None = None) -> Result:
    expiries = sorted(expiries) if expiries is not None else completed_expiries(data, underlying)
    trades: list[Trade] = []
    skipped: list[Skipped] = []
    for exp in expiries:
        out = _run_one(data, underlying, rule, exp)
        if isinstance(out, Skipped):
            skipped.append(out)
        else:
            trades.append(out)
    trades.sort(key=lambda t: (t.exit_date, t.expiry))
    curve = _equity_curve(trades, rule.allocation)
    return Result(underlying, rule, trades, skipped, summarize(trades, skipped, curve), curve)


def _run_one(data: MarketData, u: str, rule: Rule, exp: date) -> Trade | Skipped:
    tpl = rule.template
    last_day = data.last_day(u)
    if last_day is None:
        return Skipped(exp, "no price history for the underlying")
    if exp > last_day:
        return Skipped(exp, "expiry is after the last day of data")

    target = exp - timedelta(days=rule.entry_days)
    entry = data.day_on_or_before(u, target)
    if entry is None or (target - entry).days > rule.max_entry_gap_days:
        return Skipped(exp, "no trading day near the entry date")
    spot = data.spot(u, entry)
    if not spot:
        return Skipped(exp, "no underlying price on the entry day")

    # --- the chain: listed strikes ---
    listed = data.chain_quotes(u, exp, entry)
    if listed:
        chain_source = "snapshot"
        strikes = {"call": [q.strike for q in listed if q.type == "call"],
                   "put": [q.strike for q in listed if q.type == "put"]}
    elif rule.allow_model:
        chain_source = "model"
        grid = _model_strikes(data, u, spot)
        strikes = {"call": grid, "put": grid}
    else:
        return Skipped(exp, "no listed chain on the entry day")

    # --- build and price the legs ---
    legs: list[OpenLeg] = []
    for spec in tpl.legs:
        n = rule.quantity * spec.ratio
        if spec.type == "underlying":
            legs.append(OpenLeg("underlying", spec.side, n * rule.contract_size, None, spot, "snapshot"))
            continue
        k = pick_strike(spec.strike, spec.type, strikes[spec.type], spot)
        if k is None:
            return Skipped(exp, f"no {spec.strike.rule} strike for the {spec.type} leg")
        price, src = _entry_price(data, rule, u, exp, spec.type, spec.side, k, entry, spot)
        if price is None:
            return Skipped(exp, f"no price for the {spec.type} {k:g} leg")
        if price < rule.min_premium:
            return Skipped(exp, f"the {spec.type} {k:g} leg is priced below the minimum premium")
        legs.append(OpenLeg(spec.type, spec.side, n, k, price, src))
    # a template must not use the same strike twice on one side (a degenerate grid)
    seen = set()
    for l in legs:
        key = (l.type, l.side, l.strike)
        if l.type != "underlying" and key in seen:
            return Skipped(exp, "the strike grid is too thin for this template")
        seen.add(key)

    size = rule.contract_size
    max_profit, max_loss = payoff_bounds(legs, spot, size)
    net = sum(l.sign * l.qty * (l.entry_price if l.type == "underlying" else l.entry_price * size) for l in legs)
    capital = max_loss if max_loss else abs(net)
    if not capital or capital <= 0:
        return Skipped(exp, "the position has no capital at risk")

    entry_fees = sum(_leg_fee(rule.fees, l, l.entry_price, size, closing=False) for l in legs)

    # --- exit ---
    exit_day, reason, exit_spot, marks = _find_exit(data, rule, u, exp, entry, legs, capital)
    if data.max_abs_log_return(u, entry, exit_day) > rule.max_jump:
        return Skipped(exp, "the underlying price jumped (capital action?) while the trade was open")
    gross = sum(
        l.sign * l.qty * (m - l.entry_price) * (1 if l.type == "underlying" else size)
        for l, m in zip(legs, marks)
    )
    exit_fees = 0.0
    for l, m in zip(legs, marks):
        if reason == "expiry" and l.type != "underlying":
            # not a trade: no commission, only the exercise fee when it settles in the money
            if m > 0:
                exit_fees += l.qty * size * l.strike * rule.fees.exercise_fee
        else:
            exit_fees += _leg_fee(rule.fees, l, m, size, closing=True)
    fees = entry_fees + exit_fees
    pnl = gross - fees
    sources = {l.price_source for l in legs if l.type != "underlying"}
    price_source = sources.pop() if len(sources) == 1 else "mixed"
    return Trade(
        expiry=exp, entry_date=entry, exit_date=exit_day, exit_reason=reason,
        spot_entry=spot, spot_exit=exit_spot,
        legs=[dict(type=l.type, side=l.side, qty=l.qty, strike=l.strike,
                   entry_price=round(l.entry_price, 2), exit_price=round(m, 2), price_source=l.price_source)
              for l, m in zip(legs, marks)],
        net_premium=net, capital_at_risk=capital, max_profit=max_profit, max_loss=max_loss,
        gross_pnl=gross, fees=fees, pnl=pnl, return_on_risk=pnl / capital,
        price_source=price_source, chain_source=chain_source,
    )


def _entry_price(data, rule, u, exp, typ, side, strike, day, spot):
    q = data.quote(u, exp, typ, strike, day)
    if q:
        p = fill_price(q, side, rule.fill)
        if p:
            return p, "snapshot"
    if rule.allow_model:
        p = _model_price(data, u, typ, strike, exp, day, spot, rule.rate)
        if p is not None and p > 0:
            return p, "model"
    return None, ""


def _marks_on(data, rule, u, exp, legs, day, spot) -> list[float] | None:
    """Value of every leg on ``day``. None if a leg cannot be valued."""
    marks = []
    for l in legs:
        if l.type == "underlying":
            marks.append(spot)
        elif day >= exp:
            marks.append(intrinsic(l.type, spot, l.strike))
        else:
            q = data.quote(u, exp, l.type, l.strike, day)
            p = _mark_price(q) if q else None
            if p is None and rule.allow_model:
                p = _model_price(data, u, l.type, l.strike, exp, day, spot, rule.rate)
            if p is None:
                return None
            marks.append(p)
    return marks


def _find_exit(data, rule, u, exp, entry, legs, capital):
    size = rule.contract_size
    settle_day = data.day_on_or_before(u, exp) or entry
    settle_spot = data.spot(u, settle_day) or data.spot(u, entry)

    if rule.exit == "target_stop":
        for d in data.days_between(u, entry, settle_day):
            if d >= exp:
                break
            spot = data.spot(u, d)
            if not spot:
                continue
            marks = _marks_on(data, rule, u, exp, legs, d, spot)
            if marks is None:
                continue
            pnl = sum(l.sign * l.qty * (m - l.entry_price) * (1 if l.type == "underlying" else size)
                      for l, m in zip(legs, marks))
            if rule.target_pct is not None and pnl >= rule.target_pct * capital:
                return d, "target", spot, marks
            if rule.stop_pct is not None and pnl <= -rule.stop_pct * capital:
                return d, "stop", spot, marks

    marks = [settle_spot if l.type == "underlying" else intrinsic(l.type, settle_spot, l.strike) for l in legs]
    return settle_day, "expiry", settle_spot, marks


# --- metrics --------------------------------------------------------------------------

def _equity_curve(trades: Sequence[Trade], allocation: float) -> list[dict]:
    if not trades:
        return []
    eq = 1.0
    curve = [{"date": trades[0].entry_date.isoformat(), "equity": 1.0}]
    for t in trades:
        eq = max(eq + allocation * max(t.return_on_risk, -1.0), 0.0)  # an empty account stays empty
        curve.append({"date": t.exit_date.isoformat(), "equity": eq})
    return curve


def max_drawdown(curve: Sequence[dict]) -> float:
    """Largest peak-to-trough fall of the equity curve, as a positive fraction."""
    peak, worst = 0.0, 0.0
    for p in curve:
        peak = max(peak, p["equity"])
        if peak > 0:
            worst = max(worst, 1.0 - p["equity"] / peak)
    return worst


def summarize(trades: Sequence[Trade], skipped: Sequence[Skipped], curve: Sequence[dict]) -> dict:
    n = len(trades)
    base = {"trades": n, "skipped": len(skipped)}
    if not n:
        return {**base, "win_rate": None, "avg_pnl": None, "avg_return_on_risk": None, "total_pnl": 0.0,
                "total_fees": 0.0, "best_pnl": None, "worst_pnl": None, "profit_factor": None,
                "max_drawdown": 0.0, "total_return": 0.0, "model_priced_share": None}
    pnls = [t.pnl for t in trades]
    wins = [p for p in pnls if p > 0]
    losses = [-p for p in pnls if p < 0]
    return {
        **base,
        "win_rate": len(wins) / n,
        "avg_pnl": sum(pnls) / n,
        "avg_return_on_risk": sum(t.return_on_risk for t in trades) / n,
        "total_pnl": sum(pnls),
        "total_fees": sum(t.fees for t in trades),
        "best_pnl": max(pnls),
        "worst_pnl": min(pnls),
        "profit_factor": (sum(wins) / sum(losses)) if losses else None,
        "max_drawdown": max_drawdown(curve),
        "total_return": curve[-1]["equity"] - 1.0,
        "model_priced_share": sum(1 for t in trades if t.price_source != "snapshot") / n,
    }


# --- output ---------------------------------------------------------------------------

def to_backtest_run(res: Result) -> dict:
    """The row for ``backtest_runs`` (docs/api-contract.md, 3.3), plus the per-expiry
    ``trades``. ``win_rate`` and ``total_return`` are fractions, ``max_drawdown`` is a
    positive fraction, and ``equity_curve`` is ``[{date, equity}]`` starting at 1.0."""
    m = res.metrics
    return {
        "model_name": f"strategy:{res.rule.template_id}",
        "params": {"underlying": res.underlying, **_rule_dict(res.rule)},
        "start_date": res.equity_curve[0]["date"] if res.equity_curve else None,
        "end_date": res.equity_curve[-1]["date"] if res.equity_curve else None,
        "win_rate": m["win_rate"],
        "max_drawdown": m["max_drawdown"],
        "total_return": m["total_return"],
        "equity_curve": res.equity_curve,
        "metrics": m,
        "trades": [_trade_dict(t) for t in res.trades],
        "skipped": [{"expiry": s.expiry.isoformat(), "reason": s.reason} for s in res.skipped],
    }


def _rule_dict(rule: Rule) -> dict:
    d = asdict(rule)
    return d


def _trade_dict(t: Trade) -> dict:
    d = asdict(t)
    for k in ("expiry", "entry_date", "exit_date"):
        d[k] = d[k].isoformat()
    return d
