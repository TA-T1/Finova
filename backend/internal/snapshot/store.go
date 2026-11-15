package snapshot

import (
	"context"
	"fmt"
	"time"

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

const upsertSQL = `
INSERT INTO quote_snapshots (
	trade_date, contract_symbol, underlying_symbol, option_type, strike, expiry,
	close_price, last_price, bid_price, ask_price, volume, value, trade_count,
	open_interest, underlying_price, captured_at)
SELECT t.trade_date::date, t.contract_symbol, t.underlying_symbol, t.option_type, t.strike, t.expiry::date,
	t.close_price, t.last_price, t.bid_price, t.ask_price, t.volume, t.value, t.trade_count,
	t.open_interest, t.underlying_price, t.captured_at::timestamptz
FROM unnest(
	$1::text[], $2::text[], $3::text[], $4::text[], $5::bigint[], $6::text[],
	$7::bigint[], $8::bigint[], $9::bigint[], $10::bigint[], $11::bigint[], $12::bigint[], $13::bigint[],
	$14::bigint[], $15::bigint[], $16::timestamptz[]
) AS t(trade_date, contract_symbol, underlying_symbol, option_type, strike, expiry,
	close_price, last_price, bid_price, ask_price, volume, value, trade_count,
	open_interest, underlying_price, captured_at)
ON CONFLICT (trade_date, contract_symbol) DO UPDATE SET
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

// upsert writes all rows in one transaction with a single bulk statement
// (unnest over column arrays: one round trip for ~2,000 rows).
func upsert(ctx context.Context, db *pgxpool.Pool, rows []Row) (int, error) {
	n := len(rows)
	var (
		tradeDate, symbol, under, typ, expiry = make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
		strike, closeP, lastP, bid, ask       = make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n)
		volume, value, trades, oi             = make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n)
		underPrice                            = make([]*int64, n)
		captured                              = make([]time.Time, n)
	)
	for i, r := range rows {
		tradeDate[i], symbol[i], under[i], typ[i], expiry[i] = r.TradeDate, r.ContractSymbol, r.UnderlyingSymbol, r.OptionType, r.Expiry
		strike[i], closeP[i], lastP[i], bid[i], ask[i] = r.Strike, r.ClosePrice, r.LastPrice, r.BidPrice, r.AskPrice
		volume[i], value[i], trades[i], oi[i] = r.Volume, r.Value, r.TradeCount, r.OpenInterest
		underPrice[i], captured[i] = r.UnderlyingPrice, r.CapturedAt
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("snapshot: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	tag, err := tx.Exec(ctx, upsertSQL, tradeDate, symbol, under, typ, strike, expiry,
		closeP, lastP, bid, ask, volume, value, trades, oi, underPrice, captured)
	if err != nil {
		return 0, fmt.Errorf("snapshot: upsert %d rows: %w", n, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("snapshot: commit: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
