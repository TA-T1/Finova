-- Calculated option metrics (task 4: calculation layer on live data).
--
-- One row per contract per collection pass, written by market.Collector right
-- after the quote it was calculated from, so (contract_id, ts) matches the
-- same key in option_quotes. See docs/calculation-layer.md for the rules.
--
-- Version 000008 is left free on purpose: the crawl-daily-snapshot branch
-- renames its migration to 000008 (task 2).
--
-- NULL means "not available", never zero: no usable price, no volatility, or no
-- 20-day price history yet. Money values are per share, not per contract.
-- Volatilities and the risk-free rate are decimal fractions (0.30 = 30%).
-- Columns ending in _percent are percentages (2.5 = 2.5%).

CREATE TABLE option_metrics (
    contract_id        INT NOT NULL REFERENCES option_contracts(id),
    ts                 TIMESTAMPTZ NOT NULL,

    -- Inputs the numbers were calculated from.
    underlying_price   DOUBLE PRECISION NOT NULL,  -- spot (S)
    days_to_expiry     INT NOT NULL,               -- calendar days, Asia/Tehran; 0 on expiry day
    risk_free_rate     DOUBLE PRECISION NOT NULL,  -- r
    market_price       DOUBLE PRECISION,           -- option price used (premium)
    price_source       TEXT CHECK (price_source IN ('last', 'mid', 'close')),

    -- Price-based metrics.
    moneyness          TEXT NOT NULL CHECK (moneyness IN ('ITM', 'ATM', 'OTM')),
    intrinsic_value    DOUBLE PRECISION NOT NULL,
    time_value         DOUBLE PRECISION,
    break_even         DOUBLE PRECISION,
    break_even_percent DOUBLE PRECISION,           -- move in the underlying needed to reach break_even
    leverage           DOUBLE PRECISION,           -- spot / premium
    effective_leverage DOUBLE PRECISION,           -- delta * leverage

    -- Volatility and Greeks.
    implied_vol        DOUBLE PRECISION,           -- from market_price; NULL if none exists or it is 0
    historical_vol     DOUBLE PRECISION,           -- HV20 of the underlying
    greeks_vol         DOUBLE PRECISION,           -- volatility the Greeks were evaluated at (implied, else HV20)
    delta              DOUBLE PRECISION,
    gamma              DOUBLE PRECISION,
    vega_per_pct       DOUBLE PRECISION,           -- price change per +1 volatility point
    theta_per_day      DOUBLE PRECISION,           -- price change per calendar day
    rho_per_pct        DOUBLE PRECISION,           -- price change per +1 rate point

    -- Black-Scholes at HV20 and the market's distance from it.
    theoretical_price  DOUBLE PRECISION,
    price_gap          DOUBLE PRECISION,           -- market_price - theoretical_price
    price_gap_percent  DOUBLE PRECISION,

    PRIMARY KEY (contract_id, ts)
);

-- Same pattern as idx_quotes_contract_ts: lets DISTINCT ON read the newest row
-- per contract without sorting the whole table.
CREATE INDEX idx_option_metrics_contract_ts ON option_metrics (contract_id, ts DESC);

CREATE VIEW option_metrics_latest AS
SELECT DISTINCT ON (contract_id) *
FROM option_metrics
ORDER BY contract_id, ts DESC;
