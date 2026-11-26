DROP VIEW IF EXISTS option_quotes_latest;

ALTER TABLE option_quotes
    DROP COLUMN IF EXISTS prev_close;

CREATE VIEW option_quotes_latest AS
SELECT DISTINCT ON (contract_id) *
FROM option_quotes
ORDER BY contract_id, ts DESC;
