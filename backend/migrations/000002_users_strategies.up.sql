CREATE TABLE users (
    id             SERIAL PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE leg_side AS ENUM ('buy', 'sell');

CREATE TABLE strategies (
    id          SERIAL PRIMARY KEY,
    user_id     INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_strategies_user ON strategies (user_id);

CREATE TABLE strategy_legs (
    id             SERIAL PRIMARY KEY,
    strategy_id    INT NOT NULL REFERENCES strategies(id) ON DELETE CASCADE,
    contract_id    INT REFERENCES option_contracts(id),
    underlying_id  INT REFERENCES underlyings(id),
    side           leg_side NOT NULL,
    quantity       INT NOT NULL CHECK (quantity > 0),
    price          BIGINT NOT NULL,
    CHECK (contract_id IS NOT NULL OR underlying_id IS NOT NULL)
);
CREATE INDEX idx_legs_strategy ON strategy_legs (strategy_id);