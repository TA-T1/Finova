package market

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ChainOption is one listed side at a strike and expiry.
type ChainOption = MarketChainOption

// StrikePair groups the call and put contracts listed at one strike.
type StrikePair struct {
	StrikePrice int64        `json:"strikePrice"`
	Call        *ChainOption `json:"call"`
	Put         *ChainOption `json:"put"`
}

// OptionChain is a single underlying's chain for one expiry.
type OptionChain struct {
	UnderlyingSymbol string       `json:"underlyingSymbol"`
	ExpiryDate       string       `json:"expiryDate"`
	DaysToExpiry     *int         `json:"daysToExpiry"`
	UnderlyingPrice  *int64       `json:"underlyingPrice"`
	Strikes          []StrikePair `json:"strikes"`
}

// ExpirySummary describes the contracts available at one expiry.
type ExpirySummary struct {
	ExpiryDate    string `json:"expiryDate"`
	ContractCount int    `json:"contractCount"`
}

// OpenInterestSummary totals call and put open interest.
type OpenInterestSummary struct {
	Calls int64    `json:"calls"`
	Puts  int64    `json:"puts"`
	PCR   *float64 `json:"pcr"`
}

// StrikeOpenInterest is the call and put open interest at a strike, aggregated
// across active expiries.
type StrikeOpenInterest struct {
	StrikePrice      int64 `json:"strikePrice"`
	CallOpenInterest int64 `json:"callOpenInterest"`
	PutOpenInterest  int64 `json:"putOpenInterest"`
}

// MaxPainSummary is the minimum aggregate intrinsic payout among strikes for
// the nearest active expiry.
type MaxPainSummary struct {
	ExpiryDate  string  `json:"expiryDate"`
	StrikePrice int64   `json:"strikePrice"`
	TotalPayout float64 `json:"totalPayout"`
}

// SymbolPage contains the stock and options-market summary used by its page.
type SymbolPage struct {
	Symbol             string               `json:"symbol"`
	Name               *string              `json:"name"`
	Kind               string               `json:"kind"`
	Price              *int64               `json:"price"`
	PriceChange        *int64               `json:"priceChange"`
	PriceChangePercent *float64             `json:"priceChangePercent"`
	Expiries           []ExpirySummary      `json:"expiries"`
	OpenInterest       OpenInterestSummary  `json:"openInterest"`
	StrikeOpenInterest []StrikeOpenInterest `json:"strikeOpenInterest"`
	MaxPain            *MaxPainSummary      `json:"maxPain"`
}

// ListOptionChainForExpiry returns call/put pairs for one underlying and expiry.
func (r *Repository) ListOptionChainForExpiry(ctx context.Context, symbol string, expiry time.Time) ([]StrikePair, error) {
	options, err := r.ListMarketOptions(ctx, OptionListQuery{
		UnderlyingSymbol: symbol,
		Expiry:           &expiry,
		ActiveOnly:       true,
		Today:            tehranDate(time.Now()),
		Sort:             "strikePrice",
		Order:            "asc",
		Page:             1,
		PageSize:         5000,
	})
	if err != nil {
		return nil, err
	}
	var out []StrikePair
	for _, item := range options.Items {
		if len(out) == 0 || out[len(out)-1].StrikePrice != item.StrikePrice {
			out = append(out, StrikePair{StrikePrice: item.StrikePrice})
		}
		pair := &out[len(out)-1]
		option := chainOption(item)
		switch item.OptionType {
		case "call":
			pair.Call = option
		case "put":
			pair.Put = option
		default:
			return nil, fmt.Errorf("unknown option type %q for %s", item.OptionType, item.Symbol)
		}
	}
	return out, nil
}

type symbolContractRow struct {
	Underlying   Underlying
	Price        *int64
	PrevClose    *int64
	OptionType   string
	Strike       int64
	Expiry       time.Time
	Size         int
	OpenInterest int64
}

// GetSymbolPage computes a market summary from the most recent stored quotes.
func (r *Repository) GetSymbolPage(ctx context.Context, symbol string) (*SymbolPage, error) {
	const q = `
		SELECT u.id, u.symbol, u.name, u.isin, u.ins_code, u.kind,
		       uq.last_price, uq.prev_close,
		       c.option_type::text, c.strike_price, c.expiry_date,
		       c.contract_size, COALESCE(q.open_interest, 0)
		FROM underlyings u
		JOIN option_contracts c ON c.underlying_id = u.id
		LEFT JOIN option_quotes_latest q ON q.contract_id = c.id
		LEFT JOIN LATERAL (
			SELECT last_price, prev_close
			FROM underlying_quotes
			WHERE underlying_id = u.id
			ORDER BY ts DESC
			LIMIT 1
		) uq ON TRUE
		WHERE u.symbol = $1
		  AND c.expiry_date >= (now() AT TIME ZONE 'Asia/Tehran')::date
		ORDER BY c.expiry_date, c.strike_price, c.option_type`
	rows, err := r.db.Query(ctx, q, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []symbolContractRow
	for rows.Next() {
		var row symbolContractRow
		if err := rows.Scan(
			&row.Underlying.ID, &row.Underlying.Symbol, &row.Underlying.Name,
			&row.Underlying.ISIN, &row.Underlying.InsCode, &row.Underlying.Kind,
			&row.Price, &row.PrevClose, &row.OptionType, &row.Strike, &row.Expiry,
			&row.Size, &row.OpenInterest,
		); err != nil {
			return nil, err
		}
		contracts = append(contracts, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(contracts) == 0 {
		return nil, nil
	}
	return buildSymbolPage(contracts), nil
}

func buildSymbolPage(contracts []symbolContractRow) *SymbolPage {
	first := contracts[0]
	page := &SymbolPage{
		Symbol:             first.Underlying.Symbol,
		Name:               first.Underlying.Name,
		Kind:               first.Underlying.Kind,
		Expiries:           []ExpirySummary{},
		StrikeOpenInterest: []StrikeOpenInterest{},
	}
	page.Price = first.Price
	if first.Price != nil && first.PrevClose != nil {
		change := *first.Price - *first.PrevClose
		page.PriceChange = &change
		if *first.PrevClose != 0 {
			percent := float64(change) * 100 / float64(*first.PrevClose)
			page.PriceChangePercent = &percent
		}
	}

	expiryIndexes := make(map[string]int)
	strikeIndexes := make(map[int64]int)
	var nearestExpiry string
	var nearestExpiryContracts []symbolContractRow
	for _, contract := range contracts {
		expiry := contract.Expiry.Format("2006-01-02")
		index, ok := expiryIndexes[expiry]
		if !ok {
			index = len(page.Expiries)
			expiryIndexes[expiry] = index
			page.Expiries = append(page.Expiries, ExpirySummary{ExpiryDate: expiry})
		}
		page.Expiries[index].ContractCount++
		if nearestExpiry == "" {
			nearestExpiry = expiry
		}
		if expiry == nearestExpiry {
			nearestExpiryContracts = append(nearestExpiryContracts, contract)
		}

		strikeIndex, ok := strikeIndexes[contract.Strike]
		if !ok {
			strikeIndex = len(page.StrikeOpenInterest)
			strikeIndexes[contract.Strike] = strikeIndex
			page.StrikeOpenInterest = append(page.StrikeOpenInterest, StrikeOpenInterest{StrikePrice: contract.Strike})
		}
		strike := &page.StrikeOpenInterest[strikeIndex]
		switch contract.OptionType {
		case "call":
			page.OpenInterest.Calls += contract.OpenInterest
			strike.CallOpenInterest += contract.OpenInterest
		case "put":
			page.OpenInterest.Puts += contract.OpenInterest
			strike.PutOpenInterest += contract.OpenInterest
		}
	}
	if page.OpenInterest.Calls > 0 {
		pcr := float64(page.OpenInterest.Puts) / float64(page.OpenInterest.Calls)
		page.OpenInterest.PCR = &pcr
	}
	sort.Slice(page.StrikeOpenInterest, func(i, j int) bool {
		return page.StrikeOpenInterest[i].StrikePrice < page.StrikeOpenInterest[j].StrikePrice
	})
	page.MaxPain = calculateMaxPain(nearestExpiry, nearestExpiryContracts)
	return page
}

func calculateMaxPain(expiry string, contracts []symbolContractRow) *MaxPainSummary {
	if len(contracts) == 0 {
		return nil
	}
	strikes := make([]int64, 0)
	seen := make(map[int64]bool)
	for _, contract := range contracts {
		if !seen[contract.Strike] {
			seen[contract.Strike] = true
			strikes = append(strikes, contract.Strike)
		}
	}
	sort.Slice(strikes, func(i, j int) bool { return strikes[i] < strikes[j] })

	best := MaxPainSummary{ExpiryDate: expiry, StrikePrice: strikes[0]}
	bestPayout := 0.0
	for _, candidate := range strikes {
		payout := 0.0
		for _, contract := range contracts {
			intrinsic := int64(0)
			switch contract.OptionType {
			case "call":
				if candidate > contract.Strike {
					intrinsic = candidate - contract.Strike
				}
			case "put":
				if candidate < contract.Strike {
					intrinsic = contract.Strike - candidate
				}
			}
			payout += float64(intrinsic) * float64(contract.OpenInterest) * float64(contract.Size)
		}
		if candidate == strikes[0] || payout < bestPayout {
			bestPayout = payout
			best.StrikePrice = candidate
		}
	}
	best.TotalPayout = bestPayout
	return &best
}
