package snapshot

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// querier is the read side of a pool or transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// loadPrevious returns the figures of the latest snapshot dated before
// tradeDate, or nil if there is none.
func loadPrevious(ctx context.Context, q querier, tradeDate string) (*previousDay, error) {
	const sql = `
		SELECT trade_date::text, contract_symbol, volume, trade_count, last_price, close_price
		FROM quote_snapshots
		WHERE trade_date = (SELECT max(trade_date) FROM quote_snapshots WHERE trade_date < $1::date)`
	rows, err := q.Query(ctx, sql, tradeDate)
	if err != nil {
		return nil, fmt.Errorf("snapshot: load previous snapshot: %w", err)
	}
	defer rows.Close()

	var prev *previousDay
	for rows.Next() {
		var date, symbol string
		var k tradedKey
		if err := rows.Scan(&date, &symbol, &k.volume, &k.tradeCount, &k.lastPrice, &k.closePrice); err != nil {
			return nil, fmt.Errorf("snapshot: scan previous snapshot: %w", err)
		}
		if prev == nil {
			prev = &previousDay{date: date, keys: map[string]tradedKey{}}
		}
		prev.keys[symbol] = k
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("snapshot: read previous snapshot: %w", err)
	}
	return prev, nil
}

// The rows are bulk-loaded with COPY into a temporary staging table and then
// merged into quote_snapshots with one upsert, all inside one transaction. The
// staging table is dropped on commit or rollback.
const stageSQL = `
CREATE TEMP TABLE snapshot_stage (
	trade_date text, contract_symbol text, underlying_symbol text, option_type text,
	strike bigint, expiry text, close_price bigint, last_price bigint, bid_price bigint,
	ask_price bigint, volume bigint, value bigint, trade_count bigint,
	open_interest bigint, underlying_price bigint, captured_at timestamptz
) ON COMMIT DROP`

var stageColumns = []string{
	"trade_date", "contract_symbol", "underlying_symbol", "option_type",
	"strike", "expiry", "close_price", "last_price", "bid_price",
	"ask_price", "volume", "value", "trade_count",
	"open_interest", "underlying_price", "captured_at",
}

const mergeHead = `
INSERT INTO quote_snapshots (
	trade_date, contract_symbol, underlying_symbol, option_type, strike, expiry,
	close_price, last_price, bid_price, ask_price, volume, value, trade_count,
	open_interest, underlying_price, captured_at)
SELECT trade_date::date, contract_symbol, underlying_symbol, option_type, strike, expiry::date,
	close_price, last_price, bid_price, ask_price, volume, value, trade_count,
	open_interest, underlying_price, captured_at
FROM snapshot_stage
ON CONFLICT (trade_date, contract_symbol) `

const mergeUpdate = `DO UPDATE SET
	underlying_symbol = EXCLUDED.underlying_symbol,
	option_type       = EXCLUDED.option_type,
	strike            = EXCLUDED.strike,
	expiry            = EXCLUDED.expiry,
	close_price       = EXCLUDED.close_price,
	last_price        = EXCLUDED.last_price,
	bid_price         = EXCLUDED.bid_price,
	ask_price         = EXCLUDED.ask_price,
	volume            = EXCLUDED.volume,
	value             = EXCLUDED.value,
	trade_count       = EXCLUDED.trade_count,
	open_interest     = EXCLUDED.open_interest,
	underlying_price  = EXCLUDED.underlying_price,
	captured_at       = EXCLUDED.captured_at`

const mergeSkip = `DO NOTHING`

// upsert writes all rows in one transaction (COPY + one merge statement) and
// returns the number of rows inserted or updated.
func upsert(ctx context.Context, db *pgxpool.Pool, rows []Row) (int, error) {
	return write(ctx, db, rows, mergeHead+mergeUpdate)
}

// InsertMissing writes the rows that do not exist yet and leaves existing
// (trade_date, contract_symbol) rows untouched. The history backfill uses it so
// that it never overwrites a live snapshot, which has bid, ask and open interest.
// It returns the number of rows inserted.
func InsertMissing(ctx context.Context, db *pgxpool.Pool, rows []Row) (int, error) {
	return write(ctx, db, rows, mergeHead+mergeSkip)
}

func write(ctx context.Context, db *pgxpool.Pool, rows []Row, mergeSQL string) (int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("snapshot: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	if _, err := tx.Exec(ctx, stageSQL); err != nil {
		return 0, fmt.Errorf("snapshot: create staging table: %w", err)
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"snapshot_stage"}, stageColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			var under any // NULL when the underlying price is unknown
			if r.UnderlyingPrice != nil {
				under = *r.UnderlyingPrice
			}
			return []any{r.TradeDate, r.ContractSymbol, r.UnderlyingSymbol, r.OptionType,
				r.Strike, r.Expiry, r.ClosePrice, r.LastPrice, r.BidPrice,
				r.AskPrice, r.Volume, r.Value, r.TradeCount,
				r.OpenInterest, under, r.CapturedAt}, nil
		}))
	if err != nil {
		return 0, fmt.Errorf("snapshot: copy %d rows: %w", len(rows), err)
	}
	tag, err := tx.Exec(ctx, mergeSQL)
	if err != nil {
		return 0, fmt.Errorf("snapshot: upsert %d rows: %w", len(rows), err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("snapshot: commit: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
