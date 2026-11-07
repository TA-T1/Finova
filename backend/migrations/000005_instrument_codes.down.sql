DROP INDEX IF EXISTS idx_option_contracts_ins_code;
DROP INDEX IF EXISTS idx_underlyings_ins_code;

ALTER TABLE option_contracts DROP COLUMN IF EXISTS ins_code;
ALTER TABLE underlyings      DROP COLUMN IF EXISTS ins_code;
