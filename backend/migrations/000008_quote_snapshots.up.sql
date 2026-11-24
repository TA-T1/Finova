-- One end-of-day row per option contract per trading day, taken by cmd/snapshot.
-- option_quotes only keeps intraday ticks and quotes_latest-style views only the
-- current state, so this table is our history of what each option cost on past
-- days (used by the backtest). It may be the only history we ever have.
--
-- Prices are BIGINT rials, like the rest of the schema. option_type is TEXT with
-- a CHECK (not the option_type enum from 000001) so the table stands alone.
-- underlying_price is NULL when the underlying had no usable last price, which
-- is different from a real price.

CREATE TABLE quote_snapshots (
    trade_date        DATE        NOT NULL,   -- Tehran calendar date of the trading day
    contract_symbol   TEXT        NOT NULL,
    underlying_symbol TEXT        NOT NULL,
    option_type       TEXT        NOT NULL CHECK (option_type IN ('call', 'put')),
    strike            BIGINT      NOT NULL,
    expiry            DATE        NOT NULL,
    close_price       BIGINT      NOT NULL DEFAULT 0,
    last_price        BIGINT      NOT NULL DEFAULT 0,
    bid_price         BIGINT      NOT NULL DEFAULT 0,   -- as of captured_at
    ask_price         BIGINT      NOT NULL DEFAULT 0,   -- as of captured_at
    volume            BIGINT      NOT NULL DEFAULT 0,
    value             BIGINT      NOT NULL DEFAULT 0,
    trade_count       BIGINT      NOT NULL DEFAULT 0,
    open_interest     BIGINT      NOT NULL DEFAULT 0,
    underlying_price  BIGINT,                           -- underlying last price at captured_at
    captured_at       TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (trade_date, contract_symbol)           -- upsert target: one row per contract per day
);

-- History of a single contract / of all contracts on one underlying.
CREATE INDEX idx_quote_snapshots_contract   ON quote_snapshots (contract_symbol, trade_date);
CREATE INDEX idx_quote_snapshots_underlying ON quote_snapshots (underlying_symbol, trade_date);
