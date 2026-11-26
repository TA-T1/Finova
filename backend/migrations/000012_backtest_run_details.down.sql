DROP INDEX IF EXISTS idx_backtest_runs_model_underlying;

ALTER TABLE backtest_runs
    DROP COLUMN IF EXISTS skipped,
    DROP COLUMN IF EXISTS trades,
    DROP COLUMN IF EXISTS metrics;
