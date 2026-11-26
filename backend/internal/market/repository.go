// Package market persists crawled options-market data into Postgres and exposes
// it over HTTP. It depends only on the provider-neutral datasource types.
package market

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"Finova/internal/datasource"
)

// Repository reads and writes market data.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository builds a Repository over the given connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// UpsertUnderlying inserts or updates an underlying keyed by its TSETMC
// instrument code and returns its id. It never clobbers an existing name/ISIN
// with an empty value.
func (r *Repository) UpsertUnderlying(ctx context.Context, info datasource.InstrumentInfo, kind Kind) (int, error) {
	symbol := firstNonEmpty(info.Symbol, info.Name, info.InsCode)
	var name, isin *string
	if info.Name != "" {
		name = &info.Name
	}
	if info.ISIN != "" {
		isin = &info.ISIN
	}
	if !kind.Valid() {
		kind = KindOther
	}
	// On re-collection, never downgrade an already-known kind back to 'other'
	// (which is what an enrichment failure yields).
	const q = `
		INSERT INTO underlyings (symbol, name, isin, ins_code, kind)
		VALUES ($1, $2, $3, $4, $5::underlying_kind)
		ON CONFLICT (ins_code) DO UPDATE SET
			symbol = EXCLUDED.symbol,
			name   = COALESCE(EXCLUDED.name, underlyings.name),
			isin   = COALESCE(EXCLUDED.isin, underlyings.isin),
			kind   = CASE WHEN EXCLUDED.kind = 'other'
			              THEN underlyings.kind ELSE EXCLUDED.kind END
		RETURNING id`
	var id int
	err := r.db.QueryRow(ctx, q, symbol, name, isin, info.InsCode, string(kind)).Scan(&id)
	return id, err
}

// GetUnderlyingIDByInsCode returns the id of the underlying with the given
// instrument code, or ok=false if none exists.
func (r *Repository) GetUnderlyingIDByInsCode(ctx context.Context, insCode string) (id int, ok bool, err error) {
	err = r.db.QueryRow(ctx, `SELECT id FROM underlyings WHERE ins_code = $1`, insCode).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// ContractParams identifies and describes an option contract to upsert.
type ContractParams struct {
	InsCode      string
	Symbol       string
	UnderlyingID int
	Type         datasource.OptionType
	StrikePrice  int64
	ExpiryDate   time.Time
	ContractSize int
}

// UpsertContract inserts or updates an option contract keyed by its instrument
// code and returns its id.
func (r *Repository) UpsertContract(ctx context.Context, p ContractParams) (int, error) {
	size := p.ContractSize
	if size <= 0 {
		size = 1000
	}
	const q = `
		INSERT INTO option_contracts
			(symbol, underlying_id, option_type, strike_price, expiry_date, contract_size, ins_code)
		VALUES ($1, $2, $3::option_type, $4, $5, $6, $7)
		ON CONFLICT (ins_code) DO UPDATE SET
			symbol        = EXCLUDED.symbol,
			underlying_id = EXCLUDED.underlying_id,
			strike_price  = EXCLUDED.strike_price,
			expiry_date   = EXCLUDED.expiry_date,
			contract_size = EXCLUDED.contract_size
		RETURNING id`
	var id int
	err := r.db.QueryRow(ctx, q,
		p.Symbol, p.UnderlyingID, string(p.Type), p.StrikePrice, p.ExpiryDate, size, p.InsCode,
	).Scan(&id)
	return id, err
}

// InsertOptionQuote writes a quote snapshot for a contract at ts. Re-collecting
// within the same timestamp overwrites that snapshot rather than erroring.
func (r *Repository) InsertOptionQuote(ctx context.Context, contractID int, ts time.Time, leg datasource.OptionLeg) error {
	var prevClose *int64
	if leg.Yesterday != 0 {
		prevClose = &leg.Yesterday
	}
	const q = `
		INSERT INTO option_quotes
			(contract_id, ts, last_price, close_price, best_bid, best_ask,
			 bid_volume, ask_volume, volume, value, open_interest, prev_close)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (contract_id, ts) DO UPDATE SET
			last_price    = EXCLUDED.last_price,
			close_price   = EXCLUDED.close_price,
			best_bid      = EXCLUDED.best_bid,
			best_ask      = EXCLUDED.best_ask,
			bid_volume    = EXCLUDED.bid_volume,
			ask_volume    = EXCLUDED.ask_volume,
			volume        = EXCLUDED.volume,
			value         = EXCLUDED.value,
			open_interest = EXCLUDED.open_interest,
			prev_close    = EXCLUDED.prev_close`
	_, err := r.db.Exec(ctx, q,
		contractID, ts, leg.Last, leg.Close, leg.BestBid, leg.BestAsk,
		leg.BidVolume, leg.AskVolume, leg.Volume, leg.Value, leg.OpenInterest, prevClose)
	return err
}

// InsertUnderlyingQuote writes a quote snapshot for an underlying at ts.
func (r *Repository) InsertUnderlyingQuote(ctx context.Context, underlyingID int, ts time.Time, last, close, prevClose int64) error {
	const q = `
		INSERT INTO underlying_quotes (underlying_id, ts, last_price, close_price, prev_close, volume)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (underlying_id, ts) DO UPDATE SET
			last_price  = EXCLUDED.last_price,
			close_price = EXCLUDED.close_price,
			prev_close  = EXCLUDED.prev_close`
	_, err := r.db.Exec(ctx, q, underlyingID, ts, last, close, prevClose)
	return err
}

// LastMarketUpdate returns the timestamp of the most recently persisted quote.
func (r *Repository) LastMarketUpdate(ctx context.Context) (*time.Time, error) {
	const q = `
		SELECT MAX(ts)
		FROM (
			SELECT ts FROM option_quotes
			UNION ALL
			SELECT ts FROM underlying_quotes
		) market_quotes`
	var latest *time.Time
	if err := r.db.QueryRow(ctx, q).Scan(&latest); err != nil {
		return nil, err
	}
	return latest, nil
}

// UpsertDailyPrice writes one end-of-day record for an underlying.
func (r *Repository) UpsertDailyPrice(ctx context.Context, underlyingID int, d datasource.DailyPrice) error {
	const q = `
		INSERT INTO underlying_daily_prices
			(underlying_id, trade_date, open, high, low, close, final_price, volume, value)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (underlying_id, trade_date) DO UPDATE SET
			open        = EXCLUDED.open,
			high        = EXCLUDED.high,
			low         = EXCLUDED.low,
			close       = EXCLUDED.close,
			final_price = EXCLUDED.final_price,
			volume      = EXCLUDED.volume,
			value       = EXCLUDED.value`
	_, err := r.db.Exec(ctx, q,
		underlyingID, d.Date, d.Open, d.High, d.Low, d.Close, d.Final, d.Volume, d.Value)
	return err
}

// Underlying is a row returned to API clients.
type Underlying struct {
	ID      int     `json:"id"`
	Symbol  string  `json:"symbol"`
	Name    *string `json:"name"`
	ISIN    *string `json:"isin"`
	InsCode *string `json:"insCode"`
	Kind    string  `json:"kind"`
}

// ListUnderlyings returns known underlyings ordered by symbol. When kind is
// non-empty, only underlyings of that kind are returned.
func (r *Repository) ListUnderlyings(ctx context.Context, kind string) ([]Underlying, error) {
	const q = `
		SELECT id, symbol, name, isin, ins_code, kind
		FROM underlyings
		WHERE ($1 = '' OR kind::text = $1)
		ORDER BY symbol`
	rows, err := r.db.Query(ctx, q, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Underlying
	for rows.Next() {
		var u Underlying
		if err := rows.Scan(&u.ID, &u.Symbol, &u.Name, &u.ISIN, &u.InsCode, &u.Kind); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
