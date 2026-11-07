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
	const q = `
		INSERT INTO option_quotes
			(contract_id, ts, last_price, close_price, best_bid, best_ask,
			 bid_volume, ask_volume, volume, value, open_interest)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (contract_id, ts) DO UPDATE SET
			last_price    = EXCLUDED.last_price,
			close_price   = EXCLUDED.close_price,
			best_bid      = EXCLUDED.best_bid,
			best_ask      = EXCLUDED.best_ask,
			bid_volume    = EXCLUDED.bid_volume,
			ask_volume    = EXCLUDED.ask_volume,
			volume        = EXCLUDED.volume,
			value         = EXCLUDED.value,
			open_interest = EXCLUDED.open_interest`
	_, err := r.db.Exec(ctx, q,
		contractID, ts, leg.Last, leg.Close, leg.BestBid, leg.BestAsk,
		leg.BidVolume, leg.AskVolume, leg.Volume, leg.Value, leg.OpenInterest)
	return err
}

// InsertUnderlyingQuote writes a quote snapshot for an underlying at ts.
func (r *Repository) InsertUnderlyingQuote(ctx context.Context, underlyingID int, ts time.Time, last, close int64) error {
	const q = `
		INSERT INTO underlying_quotes (underlying_id, ts, last_price, close_price, volume)
		VALUES ($1, $2, $3, $4, NULL)
		ON CONFLICT (underlying_id, ts) DO UPDATE SET
			last_price  = EXCLUDED.last_price,
			close_price = EXCLUDED.close_price`
	_, err := r.db.Exec(ctx, q, underlyingID, ts, last, close)
	return err
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

// OptionChainRow is a contract plus its latest quote, for API clients.
type OptionChainRow struct {
	ContractID     int        `json:"contractId"`
	Symbol         string     `json:"symbol"`
	UnderlyingID   int        `json:"underlyingId"`
	UnderlyingSym  string     `json:"underlyingSymbol"`
	UnderlyingKind string     `json:"underlyingKind"`
	OptionType     string     `json:"optionType"`
	StrikePrice    int64      `json:"strikePrice"`
	ExpiryDate     time.Time  `json:"expiryDate"`
	ContractSize   int        `json:"contractSize"`
	LastPrice      *int64     `json:"lastPrice"`
	ClosePrice     *int64     `json:"closePrice"`
	BestBid        *int64     `json:"bestBid"`
	BestAsk        *int64     `json:"bestAsk"`
	Volume         *int64     `json:"volume"`
	OpenInterest   *int64     `json:"openInterest"`
	QuoteTS        *time.Time `json:"quoteTs"`
}

// ChainFilter narrows a chain query. Zero-value fields are ignored.
type ChainFilter struct {
	UnderlyingSymbol string
	UnderlyingKind   string
	OptionType       string
	Limit            int
}

// ListOptionChain returns contracts joined with their most recent quote.
func (r *Repository) ListOptionChain(ctx context.Context, f ChainFilter) ([]OptionChainRow, error) {
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	const q = `
		SELECT c.id, c.symbol, u.id, u.symbol, u.kind, c.option_type, c.strike_price,
		       c.expiry_date, c.contract_size,
		       q.last_price, q.close_price, q.best_bid, q.best_ask,
		       q.volume, q.open_interest, q.ts
		FROM option_contracts c
		JOIN underlyings u ON u.id = c.underlying_id
		LEFT JOIN option_quotes_latest q ON q.contract_id = c.id
		WHERE ($1 = '' OR u.symbol = $1)
		  AND ($2 = '' OR c.option_type::text = $2)
		  AND ($3 = '' OR u.kind::text = $3)
		ORDER BY u.symbol, c.expiry_date, c.strike_price, c.option_type
		LIMIT $4`
	rows, err := r.db.Query(ctx, q, f.UnderlyingSymbol, f.OptionType, f.UnderlyingKind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionChainRow
	for rows.Next() {
		var o OptionChainRow
		if err := rows.Scan(
			&o.ContractID, &o.Symbol, &o.UnderlyingID, &o.UnderlyingSym, &o.UnderlyingKind, &o.OptionType,
			&o.StrikePrice, &o.ExpiryDate, &o.ContractSize,
			&o.LastPrice, &o.ClosePrice, &o.BestBid, &o.BestAsk,
			&o.Volume, &o.OpenInterest, &o.QuoteTS,
		); err != nil {
			return nil, err
		}
		out = append(out, o)
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
