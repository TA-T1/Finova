CREATE TABLE portfolio_trades (
    id             SERIAL PRIMARY KEY,
    user_id        INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contract_id    INT REFERENCES option_contracts(id),
    underlying_id  INT REFERENCES underlyings(id),
    side           leg_side NOT NULL,
    quantity       INT NOT NULL CHECK (quantity > 0),
    price          BIGINT NOT NULL,
    fee            BIGINT NOT NULL DEFAULT 0,
    traded_at      TIMESTAMPTZ NOT NULL,
    signal_id      INT REFERENCES signals(id),
    CHECK (contract_id IS NOT NULL OR underlying_id IS NOT NULL)
);
CREATE INDEX idx_trades_user_time ON portfolio_trades (user_id, traded_at DESC);