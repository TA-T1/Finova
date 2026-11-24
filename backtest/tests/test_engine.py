from datetime import date, timedelta

import pytest

from finova_backtest.data import MarketData, Quote
from finova_backtest.engine import (Fees, OpenLeg, Rule, expiries_weekly, max_drawdown, payoff_bounds,
                                    run_backtest, to_backtest_run)
from finova_backtest.pricing import DAYS_PER_YEAR, bs_price, hv20

U = "X"
EXPIRY = date(2026, 12, 23)
ENTRY = date(2026, 11, 23)  # 30 days before
NOFEES = Fees(0, 0, 0, 0)


def weekdays(start, end):
    d = start
    while d <= end:
        if d.weekday() not in (3, 4):  # Thursday, Friday: no trading in Tehran
            yield d
        d += timedelta(days=1)


def q(typ, strike, day, last, under=None):
    return Quote(f"{typ[0]}{strike}", U, typ, strike, EXPIRY, day, close=last, last=last, underlying_price=under)


def daily(spots: dict, start=date(2026, 9, 1), end=EXPIRY, default=2000.0):
    """Daily closes: ``default`` unless a date is in ``spots``."""
    return {U: [(d, spots.get(d, default), spots.get(d, default)) for d in weekdays(start, end)]}


def spread_data(settle):
    """Snapshot on ENTRY: calls 1900..2200, spot 2000. call2000=150, call2100=80."""
    rows = [q("call", 1900, ENTRY, 230, 2000), q("call", 2000, ENTRY, 150, 2000),
            q("call", 2100, ENTRY, 80, 2000), q("call", 2200, ENTRY, 40, 2000)]
    return MarketData(rows, daily({EXPIRY: settle}))


def test_bull_call_spread_profit_hand_calculated():
    # buy call 2000 @150, sell call 2100 @80 -> debit 70/share = 70,000 per contract.
    # underlying settles 2150: long leg 150 (pnl 0), short leg 50 (pnl +30) -> 30,000.
    res = run_backtest(spread_data(2150), U, Rule("bull-call-spread", fees=NOFEES), [EXPIRY])
    t = res.trades[0]
    assert (t.entry_date, t.exit_date, t.exit_reason) == (ENTRY, EXPIRY, "expiry")
    assert t.net_premium == 70_000
    assert t.max_profit == 30_000 and t.max_loss == 70_000 and t.capital_at_risk == 70_000
    assert t.gross_pnl == 30_000 and t.fees == 0 and t.pnl == 30_000
    assert t.return_on_risk == pytest.approx(30_000 / 70_000)
    assert t.price_source == "snapshot" and t.chain_source == "snapshot"


def test_bull_call_spread_fees_hand_calculated():
    res = run_backtest(spread_data(2150), U, Rule("bull-call-spread"), [EXPIRY])
    t = res.trades[0]
    entry_fees = 1000 * 150 * 0.001 + 1000 * 80 * 0.001  # 150 + 80
    exercise = 1000 * 2000 * 0.001 + 1000 * 2100 * 0.001  # both legs settle in the money
    assert t.fees == pytest.approx(entry_fees + exercise)
    assert t.pnl == pytest.approx(30_000 - (entry_fees + exercise))


def test_bull_call_spread_max_loss_when_both_legs_expire_worthless():
    res = run_backtest(spread_data(1950), U, Rule("bull-call-spread", fees=NOFEES), [EXPIRY])
    t = res.trades[0]
    assert t.gross_pnl == -70_000 and t.pnl == -70_000
    assert t.return_on_risk == pytest.approx(-1.0)


def test_payoff_bounds_of_known_shapes():
    legs = [OpenLeg("call", "buy", 1, 2000, 150, "snapshot"), OpenLeg("call", "sell", 1, 2100, 80, "snapshot")]
    assert payoff_bounds(legs, 2000, 1000) == (30_000, 70_000)
    long_call = [OpenLeg("call", "buy", 1, 2000, 150, "snapshot")]
    assert payoff_bounds(long_call, 2000, 1000) == (None, 150_000)  # profit unlimited
    covered = [OpenLeg("underlying", "buy", 1000, None, 2000, "snapshot"), OpenLeg("call", "sell", 1, 2100, 80, "snapshot")]
    assert payoff_bounds(covered, 2000, 1000) == (180_000, 1_920_000)  # (2100-2000+80)*1000; 2000-80 per share


def test_straddle_has_two_break_even_sides():
    rows = [q("call", 2000, ENTRY, 100, 2000), q("put", 2000, ENTRY, 90, 2000)]
    data = MarketData(rows, daily({EXPIRY: 2300}))
    t = run_backtest(data, U, Rule("long-straddle", fees=NOFEES, max_jump=1.0), [EXPIRY]).trades[0]
    assert t.net_premium == 190_000
    assert t.gross_pnl == (300 - 190) * 1000


def test_covered_call_includes_the_shares():
    rows = [q("call", 2000, ENTRY, 120, 2000), q("call", 2100, ENTRY, 60, 2000)]
    data = MarketData(rows, daily({EXPIRY: 2200}))
    t = run_backtest(data, U, Rule("covered-call", fees=NOFEES), [EXPIRY]).trades[0]
    # shares: +200 per share (bought 2000, worth 2200) ; short call 2100: sold 60, owes 100 -> -40
    assert t.gross_pnl == pytest.approx(1000 * 200 - 1000 * 40)
    assert t.net_premium == pytest.approx(1000 * 2000 - 1000 * 60)


def test_missing_snapshot_falls_back_to_black_scholes_with_hv():
    spots = {d: 2000 * (1.01 if i % 2 else 1.0) for i, d in enumerate(weekdays(date(2026, 9, 1), EXPIRY))}
    data = MarketData([q("call", 2000, date(2026, 11, 1), 1, 2000)], {U: [(d, p, p) for d, p in spots.items()]})
    res = run_backtest(data, U, Rule("long-call", fees=NOFEES), [EXPIRY])
    t = res.trades[0]
    assert t.price_source == "model" and t.chain_source == "model"
    leg = t.legs[0]
    closes = [spots[d] for d in sorted(spots) if d <= ENTRY][-21:]
    expected = bs_price("call", spots[ENTRY], leg["strike"], (EXPIRY - ENTRY).days / DAYS_PER_YEAR, hv20(closes))
    assert leg["entry_price"] == pytest.approx(expected, abs=0.01)


def test_no_model_skips_instead_of_guessing():
    data = MarketData([], daily({}))
    res = run_backtest(data, U, Rule("long-call", allow_model=False), [EXPIRY])
    assert not res.trades and "no listed chain" in res.skipped[0].reason


def test_expiry_after_the_data_is_skipped():
    data = MarketData([], daily({}, end=date(2026, 12, 1)))
    res = run_backtest(data, U, Rule("long-call"), [EXPIRY])
    assert res.skipped[0].reason == "expiry is after the last day of data"


def test_target_exit_closes_before_expiry():
    day2 = date(2026, 11, 30)
    rows = [q("call", 2000, ENTRY, 150, 2000), q("call", 2100, ENTRY, 80, 2000),
            q("call", 2000, day2, 250, 2100), q("call", 2100, day2, 160, 2100)]
    # mark-to-market pnl on day2 = (250-150) - (160-80) = 20/share = 20,000 = 28.6% of the 70,000 at risk
    data = MarketData(rows, daily({day2: 2100, EXPIRY: 1900}))
    rule = Rule("bull-call-spread", exit="target_stop", target_pct=0.25, stop_pct=0.9, fees=NOFEES)
    t = run_backtest(data, U, rule, [EXPIRY]).trades[0]
    assert (t.exit_date, t.exit_reason) == (day2, "target")
    assert t.gross_pnl == pytest.approx(20_000)


def test_stop_exit():
    day2 = date(2026, 11, 30)
    rows = [q("call", 2000, ENTRY, 150, 2000), q("call", 2100, ENTRY, 80, 2000),
            q("call", 2000, day2, 40, 1900), q("call", 2100, day2, 10, 1900)]
    data = MarketData(rows, daily({day2: 1900}))  # settles at 2000 later
    rule = Rule("bull-call-spread", exit="target_stop", target_pct=0.9, stop_pct=0.5, fees=NOFEES)
    t = run_backtest(data, U, rule, [EXPIRY]).trades[0]
    assert t.exit_reason == "stop" and t.exit_date == day2
    assert t.gross_pnl == pytest.approx(((40 - 150) - (10 - 80)) * 1000)  # -40,000 = -57% of the 70,000 at risk


def test_equity_curve_and_metrics():
    win = run_backtest(spread_data(2150), U, Rule("bull-call-spread", fees=NOFEES, allocation=0.5), [EXPIRY])
    assert [p["equity"] for p in win.equity_curve] == pytest.approx([1.0, 1 + 0.5 * 30 / 70])
    assert win.metrics["win_rate"] == 1.0 and win.metrics["max_drawdown"] == 0.0
    assert win.metrics["total_return"] == pytest.approx(0.5 * 30 / 70)
    assert win.metrics["profit_factor"] is None  # no losing trade


def test_max_drawdown_is_peak_to_trough():
    curve = [{"equity": e} for e in (1.0, 1.2, 0.9, 1.1, 0.6, 0.8)]
    assert max_drawdown(curve) == pytest.approx(1 - 0.6 / 1.2)
    assert max_drawdown([]) == 0.0


def test_empty_result_has_null_metrics_not_a_crash():
    res = run_backtest(MarketData([], daily({})), U, Rule("long-call"), [])
    assert res.metrics["trades"] == 0 and res.metrics["win_rate"] is None and res.equity_curve == []


def test_backtest_run_row_matches_the_table():
    res = run_backtest(spread_data(2150), U, Rule("bull-call-spread"), [EXPIRY])
    row = to_backtest_run(res)
    assert row["model_name"] == "strategy:bull-call-spread"
    assert row["params"]["underlying"] == U and row["params"]["entry_days"] == 30
    assert row["start_date"] == "2026-11-23" and row["end_date"] == "2026-12-23"
    assert row["equity_curve"][0] == {"date": "2026-11-23", "equity": 1.0}
    assert row["trades"][0]["expiry"] == "2026-12-23"
    import json
    json.dumps(row)  # must be plain JSON


def test_rule_validation():
    for bad in (dict(template_id="nope"), dict(template_id="long-call", entry_days=0),
                dict(template_id="long-call", exit="target_stop"), dict(template_id="long-call", allocation=0),
                dict(template_id="long-call", fill="best")):
        with pytest.raises(ValueError):
            Rule(**bad)


def test_weekly_expiries_are_wednesdays():
    days = expiries_weekly(date(2026, 11, 1), date(2026, 11, 30))
    assert days == [date(2026, 11, 4), date(2026, 11, 11), date(2026, 11, 18), date(2026, 11, 25)]


def test_price_jump_while_open_skips_the_trade():
    # a 2:1 capital increase in a raw price series: 2000 -> 1000 overnight
    jump = {d: 1000 for d in weekdays(date(2026, 12, 1), EXPIRY)}
    data = MarketData([q("call", 2000, ENTRY, 150, 2000), q("call", 2100, ENTRY, 80, 2000)], daily(jump))
    res = run_backtest(data, U, Rule("bull-call-spread"), [EXPIRY])
    assert not res.trades and "jumped" in res.skipped[0].reason
    ok = run_backtest(data, U, Rule("bull-call-spread", max_jump=1.0), [EXPIRY])
    assert len(ok.trades) == 1


def test_leg_below_the_minimum_premium_is_skipped():
    data = MarketData([q("call", 2000, ENTRY, 150, 2000), q("call", 2100, ENTRY, 0.5, 2000)], daily({}))
    res = run_backtest(data, U, Rule("bull-call-spread"), [EXPIRY])
    assert "minimum premium" in res.skipped[0].reason


def test_account_cannot_go_below_zero():
    from finova_backtest.engine import _equity_curve

    class T:  # only the fields the curve reads
        def __init__(self, ror, i):
            self.return_on_risk, self.entry_date, self.exit_date = ror, date(2026, 1, 1), date(2026, 1, 2 + i)

    curve = _equity_curve([T(-1.0, i) for i in range(5)], allocation=0.5)
    assert [p["equity"] for p in curve] == [1.0, 0.5, 0.0, 0.0, 0.0, 0.0]
