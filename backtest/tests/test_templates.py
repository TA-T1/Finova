import pytest

from finova_backtest.templates import ATM, TEMPLATES, LegSpec, StrikeRule, itm, otm, pick_strike

GRID = [1800, 1900, 2000, 2100, 2200]


def test_atm_is_nearest_and_lower_on_a_tie():
    assert pick_strike(ATM, "call", GRID, 2030) == 2000
    assert pick_strike(ATM, "call", GRID, 2051) == 2100
    assert pick_strike(ATM, "call", GRID, 2050) == 2000  # tie -> lower


def test_otm_goes_up_for_calls_and_down_for_puts():
    assert pick_strike(otm(1), "call", GRID, 2000) == 2100
    assert pick_strike(otm(2), "call", GRID, 2000) == 2200
    assert pick_strike(otm(1), "put", GRID, 2000) == 1900


def test_itm_is_the_opposite():
    assert pick_strike(itm(1), "call", GRID, 2000) == 1900
    assert pick_strike(itm(1), "put", GRID, 2000) == 2100


def test_off_the_grid_is_none():
    assert pick_strike(otm(3), "call", GRID, 2000) is None
    assert pick_strike(ATM, "call", [], 2000) is None


def test_template_catalogue_matches_the_contract_list():
    assert set(TEMPLATES) == {"long-call", "long-put", "covered-call", "protective-put", "collar",
                              "bull-call-spread", "bear-put-spread", "long-straddle", "long-strangle"}
    bcs = TEMPLATES["bull-call-spread"]
    assert [(l.type, l.side, l.strike) for l in bcs.legs] == [("call", "buy", ATM), ("call", "sell", otm(1))]


def test_leg_validation():
    with pytest.raises(ValueError):
        LegSpec("underlying", "buy", ATM)
    with pytest.raises(ValueError):
        LegSpec("call", "buy")
    with pytest.raises(ValueError):
        StrikeRule("deep")
