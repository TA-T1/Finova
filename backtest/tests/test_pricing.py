import math

import pytest

from finova_backtest.pricing import bs_price, historical_volatility, hv20


def test_known_black_scholes_value():
    # textbook: S=K=100, T=1, sigma=20%, r=5% -> call 10.4506, put 5.5735
    assert bs_price("call", 100, 100, 1, 0.2, rate=0.05) == pytest.approx(10.4506, abs=1e-3)
    assert bs_price("put", 100, 100, 1, 0.2, rate=0.05) == pytest.approx(5.5735, abs=1e-3)


def test_put_call_parity():
    s, k, t, r = 2450.0, 2300.0, 40 / 365, 0.30
    c, p = bs_price("call", s, k, t, 0.4, r), bs_price("put", s, k, t, 0.4, r)
    assert c - p == pytest.approx(s - k * math.exp(-r * t), rel=1e-9)


def test_expired_option_is_its_intrinsic_value():
    assert bs_price("call", 120, 100, 0, 0.3, rate=0) == 20
    assert bs_price("put", 120, 100, 0, 0.3, rate=0) == 0


def test_rejects_bad_input():
    with pytest.raises(ValueError):
        bs_price("straddle", 1, 1, 1, 1)
    with pytest.raises(ValueError):
        bs_price("call", 0, 1, 1, 1)


def test_hv_matches_hand_calculation():
    # closes 100 -> 101 -> 100: log returns +ln(1.01) and -ln(1.01); mean 0,
    # sample variance = 2*ln(1.01)^2 / 1
    r = math.log(1.01)
    assert historical_volatility([100, 101, 100], trading_days=240) == pytest.approx(math.sqrt(2 * r * r * 240))


def test_hv_skips_adjustment_jumps_and_needs_data():
    calm = [100, 101, 100, 101, 100]
    with_jump = [100, 101, 100, 50, 51, 50, 51]  # a 2:1 capital increase in a raw series
    assert historical_volatility(with_jump) is not None
    assert historical_volatility(with_jump, max_return=None) > 3 * historical_volatility(calm)
    assert historical_volatility([100, 101]) is None
    assert hv20([]) is None
