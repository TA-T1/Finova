DROP INDEX IF EXISTS idx_underlyings_kind;
ALTER TABLE underlyings DROP COLUMN IF EXISTS kind;
DROP TYPE IF EXISTS underlying_kind;
