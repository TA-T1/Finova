CREATE TABLE underlyings (
    id          SERIAL PRIMARY KEY,
    symbol      TEXT NOT NULL UNIQUE,
    name        TEXT,
    isin        TEXT UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE underlying_daily_prices (
    underlying_id  INT NOT NULL REFERENCES underlyings(id),
    trade_date     DATE NOT NULL,
    open           BIGINT,
    high           BIGINT,
    low            BIGINT,
    close          BIGINT NOT NULL,
    final_price    BIGINT,
    volume         BIGINT,
    value          BIGINT,
    PRIMARY KEY (underlying_id, trade_date)
);

CREATE TYPE option_type AS ENUM ('call', 'put');

CREATE TABLE option_contracts (
    id              SERIAL PRIMARY KEY,
    symbol          TEXT NOT NULL UNIQUE,
    underlying_id   INT NOT NULL REFERENCES underlyings(id),
    option_type     option_type NOT NULL,
    strike_price    BIGINT NOT NULL,
    expiry_date     DATE NOT NULL,
    contract_size   INT NOT NULL DEFAULT 1000,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_contracts_underlying_expiry
    ON option_contracts (underlying_id, expiry_date);

CREATE TABLE option_quotes (
    id             BIGSERIAL PRIMARY KEY,
    contract_id    INT NOT NULL REFERENCES option_contracts(id),
    ts             TIMESTAMPTZ NOT NULL,
    last_price     BIGINT,
    close_price    BIGINT,
    best_bid       BIGINT,
    best_ask       BIGINT,
    bid_volume     BIGINT,
    ask_volume     BIGINT,
    volume         BIGINT,
    value          BIGINT,
    open_interest  BIGINT,
    UNIQUE (contract_id, ts)
);
CREATE INDEX idx_quotes_contract_ts ON option_quotes (contract_id, ts DESC);

CREATE TABLE underlying_quotes (
    id             BIGSERIAL PRIMARY KEY,
    underlying_id  INT NOT NULL REFERENCES underlyings(id),
    ts             TIMESTAMPTZ NOT NULL,
    last_price     BIGINT,
    close_price    BIGINT,
    volume         BIGINT,
    UNIQUE (underlying_id, ts)
);
CREATE INDEX idx_uquotes_underlying_ts ON underlying_quotes (underlying_id, ts DESC);

CREATE VIEW option_quotes_latest AS
SELECT DISTINCT ON (contract_id) *
FROM option_quotes
ORDER BY contract_id, ts DESC;