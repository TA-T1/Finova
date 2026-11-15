-- TSETMC instrument codes (insCode). These are the stable per-instrument
-- identifiers the crawler uses to (a) link option rows to their underlying and
-- (b) fetch historical daily prices. Stored as TEXT because the codes are large
-- (up to ~17 digits) and are treated as opaque identifiers, not numbers.

ALTER TABLE underlyings      ADD COLUMN ins_code TEXT;
ALTER TABLE option_contracts ADD COLUMN ins_code TEXT;

-- NULLs are distinct in Postgres, so these allow many rows without a code yet
-- while still enforcing uniqueness once a code is assigned.
CREATE UNIQUE INDEX idx_underlyings_ins_code      ON underlyings (ins_code);
CREATE UNIQUE INDEX idx_option_contracts_ins_code ON option_contracts (ins_code);
