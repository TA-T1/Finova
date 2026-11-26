ALTER TABLE option_quotes
    ADD COLUMN IF NOT EXISTS prev_close BIGINT;

CREATE OR REPLACE VIEW option_quotes_latest AS
SELECT DISTINCT ON (contract_id) *
FROM option_quotes
ORDER BY contract_id, ts DESC;
