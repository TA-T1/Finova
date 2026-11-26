"""Black-Scholes pricing and historical volatility.

The conventions are the same as backend/internal/options (Go), so a price the
engine computes is the price the site shows:

* European option, no dividends, risk-free rate ``DEFAULT_RATE`` (0.30);
* time to expiry = calendar days / 365;
* HV20 = sample standard deviation of the last 20 daily log returns, times
  sqrt(240) (about 240 trading days a year in Tehran).

Prices are per share, in rials, like the exchange prices.
"""
from __future__ import annotations

import math
from typing import Sequence

DEFAULT_RATE = 0.30
DAYS_PER_YEAR = 365.0
TRADING_DAYS_PER_YEAR = 240.0
HV_WINDOW = 20
# The Tehran exchange limits a stock to +-5% a day (+-10% for some funds), so a
# bigger move in a raw price series is a capital increase or a dividend, not
# volatility. HV20 drops such returns instead of reading them as risk.
MAX_PLAUSIBLE_RETURN = 0.12


def norm_cdf(x: float) -> float:
    return 0.5 * math.erfc(-x / math.sqrt(2.0))


def intrinsic(kind: str, spot: float, strike: float) -> float:
    return max(spot - strike, 0.0) if kind == "call" else max(strike - spot, 0.0)


def bs_price(kind: str, spot: float, strike: float, t_years: float, sigma: float,
             rate: float = DEFAULT_RATE) -> float:
    """Black-Scholes value of one share of a call or put."""
    if kind not in ("call", "put"):
        raise ValueError(f"kind must be 'call' or 'put', got {kind!r}")
    if spot <= 0 or strike <= 0:
        raise ValueError("spot and strike must be positive")
    if t_years <= 0 or sigma <= 0:
        # No optionality left: the discounted payoff of the forward.
        t = max(t_years, 0.0)
        disc = strike * math.exp(-rate * t)
        return max(spot - disc, 0.0) if kind == "call" else max(disc - spot, 0.0)
    sd = sigma * math.sqrt(t_years)
    d1 = (math.log(spot / strike) + (rate + 0.5 * sigma * sigma) * t_years) / sd
    d2 = d1 - sd
    disc = strike * math.exp(-rate * t_years)
    if kind == "call":
        return spot * norm_cdf(d1) - disc * norm_cdf(d2)
    return disc * norm_cdf(-d2) - spot * norm_cdf(-d1)


def historical_volatility(closes: Sequence[float], trading_days: float = TRADING_DAYS_PER_YEAR,
                          max_return: float | None = MAX_PLAUSIBLE_RETURN) -> float | None:
    """Annualised volatility of ``closes`` (oldest first), or None if unusable.

    Needs at least two usable returns. Returns bigger than ``max_return`` in
    absolute log terms are skipped (see MAX_PLAUSIBLE_RETURN); pass None to keep all.
    """
    rets = []
    for prev, cur in zip(closes, closes[1:]):
        if not (prev and cur and prev > 0 and cur > 0):
            continue
        r = math.log(cur / prev)
        if max_return is not None and abs(r) > max_return:
            continue
        rets.append(r)
    if len(rets) < 2:
        return None
    mean = sum(rets) / len(rets)
    var = sum((r - mean) ** 2 for r in rets) / (len(rets) - 1)
    return math.sqrt(var * trading_days)


def hv20(closes: Sequence[float]) -> float | None:
    """HV20 from the last ``HV_WINDOW + 1`` closes; None with fewer than 3 closes."""
    return historical_volatility(list(closes)[-(HV_WINDOW + 1):])
