CREATE TABLE signals (
    id             SERIAL PRIMARY KEY,
    underlying_id  INT NOT NULL REFERENCES underlyings(id),
    contract_id    INT REFERENCES option_contracts(id),
    model_name     TEXT NOT NULL,
    action         TEXT NOT NULL CHECK (action IN ('enter', 'exit')),
    direction      TEXT CHECK (direction IN ('long', 'short')),
    entry_price    BIGINT,
    target_price   BIGINT,
    stop_price     BIGINT,
    confidence     DOUBLE PRECISION,
    generated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason         JSONB
);
CREATE INDEX idx_signals_underlying_time ON signals (underlying_id, generated_at DESC);

CREATE TABLE backtest_runs (
    id            SERIAL PRIMARY KEY,
    model_name    TEXT NOT NULL,
    params        JSONB,
    start_date    DATE,
    end_date      DATE,
    win_rate      DOUBLE PRECISION,
    max_drawdown  DOUBLE PRECISION,
    total_return  DOUBLE PRECISION,
    equity_curve  JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);