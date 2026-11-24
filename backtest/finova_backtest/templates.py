"""Strategy templates and strike selection.

The ids, legs and strike rules are the draft list of docs/api-contract.md (2.3),
so the backtest, the strategy builder API and the front end speak about the same
strategies. Strikes are picked among the strikes **listed** for the chosen
expiry, never by percentage:

* ``atm``          the listed strike closest to the underlying price (the lower one on a tie);
* ``otm`` n steps  n listed strikes further out of the money than ATM (up for a call, down for a put);
* ``itm`` n steps  n listed strikes further into the money than ATM.
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Sequence


@dataclass(frozen=True)
class StrikeRule:
    rule: str  # "atm" | "otm" | "itm"
    steps: int = 0

    def __post_init__(self):
        if self.rule not in ("atm", "otm", "itm"):
            raise ValueError(f"unknown strike rule {self.rule!r}")
        if self.steps < 0:
            raise ValueError("steps must be >= 0")


ATM = StrikeRule("atm")


def otm(n: int = 1) -> StrikeRule:
    return StrikeRule("otm", n)


def itm(n: int = 1) -> StrikeRule:
    return StrikeRule("itm", n)


@dataclass(frozen=True)
class LegSpec:
    type: str  # "call" | "put" | "underlying"
    side: str  # "buy" | "sell"
    strike: StrikeRule | None = None  # None for an underlying leg
    ratio: int = 1

    def __post_init__(self):
        if self.type not in ("call", "put", "underlying"):
            raise ValueError(f"unknown leg type {self.type!r}")
        if self.side not in ("buy", "sell"):
            raise ValueError(f"unknown side {self.side!r}")
        if (self.type == "underlying") != (self.strike is None):
            raise ValueError("an underlying leg has no strike rule, an option leg needs one")


@dataclass(frozen=True)
class Template:
    id: str
    name: str
    name_fa: str
    outlook: str  # bullish | bearish | neutral | volatile
    legs: tuple[LegSpec, ...]


def _t(id, name, fa, outlook, *legs):
    return Template(id, name, fa, outlook, tuple(legs))


TEMPLATES: dict[str, Template] = {t.id: t for t in (
    _t("long-call", "Long call", "خرید کال", "bullish",
       LegSpec("call", "buy", ATM)),
    _t("long-put", "Long put", "خرید پوت", "bearish",
       LegSpec("put", "buy", ATM)),
    _t("covered-call", "Covered call", "کاورد کال", "neutral",
       LegSpec("underlying", "buy"), LegSpec("call", "sell", otm(1))),
    _t("protective-put", "Protective put", "پوت محافظ", "bullish",
       LegSpec("underlying", "buy"), LegSpec("put", "buy", ATM)),
    _t("collar", "Collar", "کولار", "neutral",
       LegSpec("underlying", "buy"), LegSpec("put", "buy", otm(1)), LegSpec("call", "sell", otm(1))),
    _t("bull-call-spread", "Bull call spread", "بول کال اسپرد", "bullish",
       LegSpec("call", "buy", ATM), LegSpec("call", "sell", otm(1))),
    _t("bear-put-spread", "Bear put spread", "بیر پوت اسپرد", "bearish",
       LegSpec("put", "buy", ATM), LegSpec("put", "sell", otm(1))),
    _t("long-straddle", "Long straddle", "استرادل", "volatile",
       LegSpec("call", "buy", ATM), LegSpec("put", "buy", ATM)),
    _t("long-strangle", "Long strangle", "استرانگل", "volatile",
       LegSpec("call", "buy", otm(1)), LegSpec("put", "buy", otm(1))),
)}


def pick_strike(rule: StrikeRule, option_type: str, strikes: Sequence[float], spot: float) -> float | None:
    """Apply ``rule`` to the listed ``strikes``; None when the grid has no such strike."""
    grid = sorted(set(strikes))
    if not grid:
        return None
    atm = min(range(len(grid)), key=lambda i: (abs(grid[i] - spot), grid[i]))
    if rule.rule == "atm":
        return grid[atm]
    # "further out of the money": up for a call, down for a put; itm is the opposite.
    up = (option_type == "call") == (rule.rule == "otm")
    i = atm + rule.steps if up else atm - rule.steps
    return grid[i] if 0 <= i < len(grid) else None
