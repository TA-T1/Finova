-- Classify each option underlying so stock options (اختیار سهام) can be told
-- apart from fund/ETF and (future) index options. The classification is derived
-- from the instrument's market sector (TSETMC cSecVal); see internal/market.

CREATE TYPE underlying_kind AS ENUM ('stock', 'fund', 'index', 'other');

ALTER TABLE underlyings
    ADD COLUMN kind underlying_kind NOT NULL DEFAULT 'other';

CREATE INDEX idx_underlyings_kind ON underlyings (kind);
