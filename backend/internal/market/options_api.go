package market

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type OptionListQuery struct {
	UnderlyingSymbol string
	UnderlyingKind   string
	OptionType       string
	Expiry           *time.Time
	ActiveOnly       bool
	Today            time.Time
	Sort             string
	Order            string
	Page             int
	PageSize         int
}

type OptionQuoteFields struct {
	LastPrice    *int64     `json:"lastPrice"`
	ClosePrice   *int64     `json:"closePrice"`
	BestBid      *int64     `json:"bestBid"`
	BestAsk      *int64     `json:"bestAsk"`
	Volume       *int64     `json:"volume"`
	OpenInterest *int64     `json:"openInterest"`
	QuoteTS      *time.Time `json:"quoteTs"`
	Value        *int64     `json:"value"`
	PrevClose    *int64     `json:"prevClose"`
}

type OptionCalculatedFields struct {
	UnderlyingPrice      *float64   `json:"underlyingPrice"`
	DaysToExpiry         *int       `json:"daysToExpiry"`
	Moneyness            *string    `json:"moneyness"`
	MarketPrice          *float64   `json:"marketPrice"`
	PriceSource          *string    `json:"priceSource"`
	IntrinsicValue       *float64   `json:"intrinsicValue"`
	TimeValue            *float64   `json:"timeValue"`
	BreakEven            *float64   `json:"breakEven"`
	BreakEvenPercent     *float64   `json:"breakEvenPercent"`
	Leverage             *float64   `json:"leverage"`
	EffectiveLeverage    *float64   `json:"effectiveLeverage"`
	ImpliedVolatility    *float64   `json:"impliedVolatility"`
	HistoricalVolatility *float64   `json:"historicalVolatility"`
	GreeksVolatility     *float64   `json:"greeksVolatility"`
	Delta                *float64   `json:"delta"`
	Gamma                *float64   `json:"gamma"`
	Vega                 *float64   `json:"vega"`
	Theta                *float64   `json:"theta"`
	Rho                  *float64   `json:"rho"`
	TheoreticalPrice     *float64   `json:"theoreticalPrice"`
	PriceGap             *float64   `json:"priceGap"`
	PriceGapPercent      *float64   `json:"priceGapPercent"`
	MetricsTS            *time.Time `json:"metricsTs"`
}

type MarketOptionIdentity struct {
	ContractID     int    `json:"contractId"`
	Symbol         string `json:"symbol"`
	UnderlyingID   int    `json:"underlyingId"`
	UnderlyingSym  string `json:"underlyingSymbol"`
	UnderlyingKind string `json:"underlyingKind"`
	OptionType     string `json:"optionType"`
	StrikePrice    int64  `json:"strikePrice"`
	ExpiryDate     string `json:"expiryDate"`
	ContractSize   int    `json:"contractSize"`
}

type MarketOption struct {
	MarketOptionIdentity
	OptionQuoteFields
	OptionCalculatedFields
}

type MarketOptionsPage struct {
	Items      []MarketOption `json:"items"`
	Count      int            `json:"count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	Total      int            `json:"total"`
	TotalPages int            `json:"totalPages"`
}

type ChainCalculatedFields struct {
	Moneyness            *string    `json:"moneyness"`
	MarketPrice          *float64   `json:"marketPrice"`
	PriceSource          *string    `json:"priceSource"`
	IntrinsicValue       *float64   `json:"intrinsicValue"`
	TimeValue            *float64   `json:"timeValue"`
	BreakEven            *float64   `json:"breakEven"`
	BreakEvenPercent     *float64   `json:"breakEvenPercent"`
	Leverage             *float64   `json:"leverage"`
	EffectiveLeverage    *float64   `json:"effectiveLeverage"`
	ImpliedVolatility    *float64   `json:"impliedVolatility"`
	HistoricalVolatility *float64   `json:"historicalVolatility"`
	GreeksVolatility     *float64   `json:"greeksVolatility"`
	Delta                *float64   `json:"delta"`
	Gamma                *float64   `json:"gamma"`
	Vega                 *float64   `json:"vega"`
	Theta                *float64   `json:"theta"`
	Rho                  *float64   `json:"rho"`
	TheoreticalPrice     *float64   `json:"theoreticalPrice"`
	PriceGap             *float64   `json:"priceGap"`
	PriceGapPercent      *float64   `json:"priceGapPercent"`
	MetricsTS            *time.Time `json:"metricsTs"`
}

type MarketChainOption struct {
	ContractID int    `json:"contractId"`
	Symbol     string `json:"symbol"`
	OptionQuoteFields
	ChainCalculatedFields
}

func ParseOptionListQuery(values url.Values) (OptionListQuery, error) {
	q := OptionListQuery{
		UnderlyingSymbol: strings.TrimSpace(values.Get("underlying")),
		UnderlyingKind:   strings.TrimSpace(values.Get("underlyingKind")),
		OptionType:       strings.TrimSpace(values.Get("type")),
		Sort:             strings.TrimSpace(values.Get("sort")),
		Order:            strings.ToLower(strings.TrimSpace(values.Get("order"))),
		Page:             1,
		PageSize:         50,
	}
	if q.UnderlyingKind != "" && q.UnderlyingKind != "stock" && q.UnderlyingKind != "fund" &&
		q.UnderlyingKind != "index" && q.UnderlyingKind != "other" {
		return q, fmt.Errorf("underlyingKind must be stock, fund, index, or other")
	}
	if q.OptionType != "" && q.OptionType != "call" && q.OptionType != "put" {
		return q, fmt.Errorf("type must be call or put")
	}
	if expiry := values.Get("expiry"); expiry != "" {
		parsed, err := time.Parse("2006-01-02", expiry)
		if err != nil || parsed.Format("2006-01-02") != expiry {
			return q, fmt.Errorf("expiry must use YYYY-MM-DD")
		}
		q.Expiry = &parsed
	}
	if q.Sort != "" {
		if _, ok := optionSortColumns[q.Sort]; !ok {
			return q, fmt.Errorf("unknown sort field")
		}
	}
	if q.Order == "" {
		q.Order = "asc"
	}
	if q.Order != "asc" && q.Order != "desc" {
		return q, fmt.Errorf("order must be asc or desc")
	}
	if page := values.Get("page"); page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 {
			return q, fmt.Errorf("page must be a positive integer")
		}
		q.Page = n
	}
	pageSize := values.Get("pageSize")
	if pageSize == "" {
		pageSize = values.Get("limit")
	}
	if pageSize != "" {
		n, err := strconv.Atoi(pageSize)
		if err != nil || n < 1 || n > 500 {
			return q, fmt.Errorf("pageSize must be an integer from 1 to 500")
		}
		q.PageSize = n
	}
	if q.Page-1 > math.MaxInt/q.PageSize {
		return q, fmt.Errorf("page is too large")
	}
	return q, nil
}

var optionSortColumns = map[string]string{
	"symbol":            "c.symbol",
	"underlyingSymbol":  "u.symbol",
	"optionType":        "c.option_type::text",
	"strikePrice":       "c.strike_price",
	"expiryDate":        "c.expiry_date",
	"daysToExpiry":      "m.days_to_expiry",
	"lastPrice":         "NULLIF(q.last_price, 0)",
	"volume":            "q.volume",
	"openInterest":      "q.open_interest",
	"impliedVolatility": "m.implied_vol",
	"delta":             "m.delta",
	"leverage":          "m.leverage",
	"effectiveLeverage": "m.effective_leverage",
	"timeValue":         "m.time_value",
	"intrinsicValue":    "m.intrinsic_value",
	"priceGapPercent":   "m.price_gap_percent",
	"breakEvenPercent":  "m.break_even_percent",
	"bestBid":           "NULLIF(q.best_bid, 0)",
	"value":             "q.value",
	"breakEven":         "m.break_even",
	"moneyness":         "CASE m.moneyness WHEN 'ITM' THEN 1 WHEN 'ATM' THEN 2 WHEN 'OTM' THEN 3 END",
}

const marketOptionSelect = `
	SELECT c.id, c.symbol, u.id, u.symbol, u.kind::text, c.option_type::text,
	       c.strike_price, c.expiry_date, c.contract_size,
	       NULLIF(q.last_price, 0), NULLIF(q.close_price, 0),
	       NULLIF(q.best_bid, 0), NULLIF(q.best_ask, 0),
	       q.volume, q.open_interest, q.ts, q.value, NULLIF(q.prev_close, 0),
	       m.underlying_price, m.days_to_expiry, m.moneyness, m.market_price,
	       m.price_source, m.intrinsic_value, m.time_value, m.break_even,
	       m.break_even_percent, m.leverage, m.effective_leverage,
	       m.implied_vol, m.historical_vol, m.greeks_vol, m.delta, m.gamma,
	       m.vega_per_pct, m.theta_per_day, m.rho_per_pct,
	       m.theoretical_price, m.price_gap, m.price_gap_percent, m.ts
	FROM option_contracts c
	JOIN underlyings u ON u.id = c.underlying_id
	LEFT JOIN option_quotes_latest q ON q.contract_id = c.id
	LEFT JOIN option_metrics_latest m ON m.contract_id = c.id
	WHERE ($1 = '' OR u.symbol = $1)
	  AND ($2 = '' OR u.kind::text = $2)
	  AND ($3 = '' OR c.option_type::text = $3)
	  AND ($4::date IS NULL OR c.expiry_date = $4::date)`

func (r *Repository) ListMarketOptions(ctx context.Context, q OptionListQuery) (MarketOptionsPage, error) {
	var out MarketOptionsPage
	out.Items = []MarketOption{}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 50
	}
	if q.Today.IsZero() {
		q.Today = tehranDate(time.Now())
	}
	out.Page, out.PageSize = q.Page, q.PageSize
	var expiry any
	if q.Expiry != nil {
		expiry = *q.Expiry
	}
	filterArgs := []any{q.UnderlyingSymbol, q.UnderlyingKind, q.OptionType, expiry, q.ActiveOnly, q.Today}
	const countSQL = `
		SELECT count(*)
		FROM option_contracts c
		JOIN underlyings u ON u.id = c.underlying_id
		WHERE ($1 = '' OR u.symbol = $1)
		  AND ($2 = '' OR u.kind::text = $2)
		  AND ($3 = '' OR c.option_type::text = $3)
		  AND ($4::date IS NULL OR c.expiry_date = $4::date)
		  AND (NOT $5::boolean OR c.expiry_date >= $6::date)`
	if err := r.db.QueryRow(ctx, countSQL, filterArgs...).Scan(&out.Total); err != nil {
		return out, err
	}
	direction := strings.ToUpper(q.Order)
	if direction != "DESC" {
		direction = "ASC"
	}
	var orderSQL string
	if q.Sort == "" {
		orderSQL = fmt.Sprintf(
			" ORDER BY u.symbol %s NULLS LAST, c.expiry_date %s NULLS LAST, "+
				"c.strike_price %s NULLS LAST, c.option_type::text %s NULLS LAST, c.id ASC",
			direction, direction, direction, direction)
	} else {
		orderSQL = fmt.Sprintf(" ORDER BY %s %s NULLS LAST, c.id ASC", optionSortColumns[q.Sort], direction)
	}
	query := marketOptionSelect +
		"\n\t  AND (NOT $5::boolean OR c.expiry_date >= $6::date)" +
		orderSQL + " LIMIT $7 OFFSET $8"
	rows, err := r.db.Query(ctx, query, append(filterArgs, q.PageSize, (q.Page-1)*q.PageSize)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item MarketOption
		if err := scanMarketOption(rows, &item); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.Count = len(out.Items)
	if out.Total > 0 {
		out.TotalPages = (out.Total + q.PageSize - 1) / q.PageSize
	}
	return out, nil
}

func scanMarketOption(rows interface{ Scan(...any) error }, item *MarketOption) error {
	var expiry time.Time
	if err := rows.Scan(
		&item.ContractID, &item.Symbol, &item.UnderlyingID, &item.UnderlyingSym, &item.UnderlyingKind,
		&item.OptionType, &item.StrikePrice, &expiry, &item.ContractSize,
		&item.LastPrice, &item.ClosePrice, &item.BestBid, &item.BestAsk, &item.Volume, &item.OpenInterest,
		&item.QuoteTS, &item.Value, &item.PrevClose,
		&item.UnderlyingPrice, &item.DaysToExpiry, &item.Moneyness, &item.MarketPrice,
		&item.PriceSource, &item.IntrinsicValue, &item.TimeValue, &item.BreakEven,
		&item.BreakEvenPercent, &item.Leverage, &item.EffectiveLeverage,
		&item.ImpliedVolatility, &item.HistoricalVolatility, &item.GreeksVolatility,
		&item.Delta, &item.Gamma, &item.Vega, &item.Theta, &item.Rho,
		&item.TheoreticalPrice, &item.PriceGap, &item.PriceGapPercent, &item.MetricsTS,
	); err != nil {
		return err
	}
	item.ExpiryDate = expiry.Format("2006-01-02")
	return nil
}

func (r *Repository) LatestUnderlyingPrice(ctx context.Context, symbol string) (*int64, error) {
	var price *int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(last_price, 0), NULLIF(close_price, 0))
		FROM underlying_quotes uq
		JOIN underlyings u ON u.id = uq.underlying_id
		WHERE u.symbol = $1
		ORDER BY uq.ts DESC
		LIMIT 1`, symbol).Scan(&price)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return price, nil
}

func chainOption(item MarketOption) *MarketChainOption {
	m := item.OptionCalculatedFields
	return &MarketChainOption{
		ContractID:        item.ContractID,
		Symbol:            item.Symbol,
		OptionQuoteFields: item.OptionQuoteFields,
		ChainCalculatedFields: ChainCalculatedFields{
			Moneyness: m.Moneyness, MarketPrice: m.MarketPrice, PriceSource: m.PriceSource,
			IntrinsicValue: m.IntrinsicValue, TimeValue: m.TimeValue, BreakEven: m.BreakEven,
			BreakEvenPercent: m.BreakEvenPercent, Leverage: m.Leverage,
			EffectiveLeverage: m.EffectiveLeverage, ImpliedVolatility: m.ImpliedVolatility,
			HistoricalVolatility: m.HistoricalVolatility, GreeksVolatility: m.GreeksVolatility,
			Delta: m.Delta, Gamma: m.Gamma, Vega: m.Vega, Theta: m.Theta, Rho: m.Rho,
			TheoreticalPrice: m.TheoreticalPrice, PriceGap: m.PriceGap,
			PriceGapPercent: m.PriceGapPercent, MetricsTS: m.MetricsTS,
		},
	}
}

type MarketExpiry struct {
	ExpiryDate    string `json:"expiryDate"`
	DaysToExpiry  int    `json:"daysToExpiry"`
	ContractCount int    `json:"contractCount"`
}

type MarketExpiries struct {
	UnderlyingSymbol *string        `json:"underlyingSymbol"`
	Items            []MarketExpiry `json:"items"`
}

func (r *Repository) UnderlyingExists(ctx context.Context, symbol string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM underlyings WHERE symbol = $1)`, symbol).Scan(&exists)
	return exists, err
}

func (r *Repository) ListExpiries(ctx context.Context, symbol *string, now time.Time) (MarketExpiries, error) {
	today := tehranDate(now)
	out := MarketExpiries{Items: []MarketExpiry{}}
	if symbol != nil {
		out.UnderlyingSymbol = symbol
	}
	var symbolFilter string
	if symbol != nil {
		symbolFilter = *symbol
	}
	rows, err := r.db.Query(ctx, `
		SELECT c.expiry_date, count(*)
		FROM option_contracts c
		JOIN underlyings u ON u.id = c.underlying_id
		WHERE c.expiry_date >= $1
		  AND ($2 = '' OR u.symbol = $2)
		GROUP BY c.expiry_date
		ORDER BY c.expiry_date`, today, symbolFilter)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var expiry time.Time
		var count int
		if err := rows.Scan(&expiry, &count); err != nil {
			return out, err
		}
		out.Items = append(out.Items, MarketExpiry{
			ExpiryDate:    expiry.Format("2006-01-02"),
			DaysToExpiry:  dateDaysBetween(today, expiry),
			ContractCount: count,
		})
	}
	return out, rows.Err()
}

func tehranDate(now time.Time) time.Time {
	local := now.In(tehranLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tehranLocation)
}

func dateDaysBetween(from, to time.Time) int {
	fy, fm, fd := from.Date()
	ty, tm, td := to.Date()
	start := time.Date(fy, fm, fd, 0, 0, 0, 0, time.UTC)
	end := time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC)
	return int(end.Sub(start).Hours() / 24)
}
