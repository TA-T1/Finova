-- Strategy backtest runs (docs/strategy-backtest.md). Written by the Python
-- engine (backtest/), read by GET /api/v1/backtests.
ALTER TABLE backtest_runs
    ADD COLUMN IF NOT EXISTS metrics JSONB,
    ADD COLUMN IF NOT EXISTS trades  JSONB,
    ADD COLUMN IF NOT EXISTS skipped JSONB;

CREATE INDEX IF NOT EXISTS idx_backtest_runs_model_underlying
    ON backtest_runs (model_name, (params->>'underlying'));
