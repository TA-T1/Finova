package datasource

import (
	"context"
	"time"
)

// MarketSource is the snapshot-style view of the option market: one call
// returns every active option contract plus its underlyings, consistent as of
// a single moment. The collection service should depend on this interface.
//
// Implementations: tsetmc.Client (live) and datasource.FakeSource (offline,
// loads a saved snapshot from testdata/).
type MarketSource interface {
	// FetchMarket returns all active option contracts with their latest quotes
	// and a quote for every underlying that has options. If the source needs
	// several requests, they all happen inside this call.
	FetchMarket(ctx context.Context) (*MarketSnapshot, error)
}

// MarketSnapshot is one consistent view of the market.
type MarketSnapshot struct {
	FetchedAt   time.Time         `json:"fetched_at"`
	Contracts   []OptionContract  `json:"contracts"`
	Underlyings []UnderlyingQuote `json:"underlyings"`
}

// OptionContract is one option instrument (a single call or put) with its
// latest quote. All text is normalized (Persian ی/ک, ASCII digits, no
// zero-width characters). Missing numeric fields are 0.
type OptionContract struct {
	SourceID         string    `json:"source_id"`         // the source's own id (TSETMC insCode)
	Symbol           string    `json:"symbol"`            // e.g. ضخود1234
	UnderlyingSymbol string    `json:"underlying_symbol"` // e.g. خودرو
	Type             string    `json:"type"`              // "call" | "put" (string(Call) / string(Put))
	Strike           float64   `json:"strike"`
	Expiry           time.Time `json:"expiry"` // Gregorian date at 00:00 Asia/Tehran
	ContractSize     int       `json:"contract_size"`

	LastPrice     float64    `json:"last_price"`
	ClosePrice    float64    `json:"close_price"`
	PrevClose     float64    `json:"prev_close"`
	BidPrice      float64    `json:"bid_price"`
	BidVolume     int64      `json:"bid_volume"`
	AskPrice      float64    `json:"ask_price"`
	AskVolume     int64      `json:"ask_volume"`
	Volume        int64      `json:"volume"`
	Value         float64    `json:"value"`
	TradeCount    int64      `json:"trade_count"`
	OpenInterest  int64      `json:"open_interest"`
	LastTradeTime *time.Time `json:"last_trade_time"` // nil when unknown (always nil for TSETMC; use MarketSnapshot.FetchedAt)
}

// UnderlyingQuote is the latest quote of an underlying that has options.
//
// Known limitation of the TSETMC source: its option-chain endpoint gives only
// the underlying's short symbol and its prices. So Name equals Symbol and
// Volume is always 0, which means "unknown", not "no trading". Do not store
// Volume, and do not overwrite a stored full name with Name. See "Known
// limitations" in docs/api.md.
type UnderlyingQuote struct {
	SourceID   string  `json:"source_id"`
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	LastPrice  float64 `json:"last_price"`
	ClosePrice float64 `json:"close_price"`
	PrevClose  float64 `json:"prev_close"`
	Volume     int64   `json:"volume"`
}
