// Package datasource defines the provider-neutral contract and data types used
// to crawl Iranian options-market data (اختیار معامله) from external sources.
//
// Each concrete provider (TSETMC, Tehran Stock Exchange, OptionBaaz, ...) lives
// in its own sub-package and implements the Source interface. The rest of the
// backend depends only on the types declared here, never on a specific site, so
// a provider can be swapped or added without touching the collector or the DB
// layer.
package datasource

import (
	"context"
	"errors"
	"time"
)

// ErrNotSupported is returned by a Source for a capability it does not provide.
// Callers should treat it as a soft "skip", not a hard failure.
var ErrNotSupported = errors.New("datasource: operation not supported by this source")

// OptionType is the kind of an option contract.
type OptionType string

const (
	Call OptionType = "call"
	Put  OptionType = "put"
)

// InstrumentInfo is the identity of a tradable instrument (an underlying or an
// option contract), independent of its live prices.
type InstrumentInfo struct {
	InsCode      string // provider instrument code (stable key)
	Symbol       string // trading symbol, e.g. "اهرم" or "ضهرم8024"
	Name         string // full descriptive name
	ISIN         string // ISIN code, may be empty if the provider omits it
	ContractSize int    // units per contract (0 for a plain underlying)
	SectorCode   string // market sector code (TSETMC cSecVal), used to classify kind
	SectorName   string // human-readable sector name (TSETMC lSecVal)
}

// DailyPrice is one end-of-day OHLC record for an instrument.
//
// Iranian equities distinguish the last traded price of the day ("آخرین") from
// the volume-weighted settlement price ("قیمت پایانی"). We map the former to
// Close and the latter to Final, matching the two columns in the schema.
type DailyPrice struct {
	Date   time.Time
	Open   int64 // first trade price
	High   int64
	Low    int64
	Close  int64 // last trade price of the day ("آخرین")
	Final  int64 // weighted settlement price ("قیمت پایانی")
	Volume int64 // traded quantity
	Value  int64 // traded value (Rial)
	// TradeCount is the number of trades (TSETMC zTotTran); 0 when unknown.
	TradeCount int64
}

// OptionLeg is one side (call or put) of an option row: its identity plus its
// current market snapshot.
type OptionLeg struct {
	InsCode      string
	Symbol       string
	Name         string
	Type         OptionType
	Last         int64 // last traded price
	Close        int64 // closing / settlement price
	Yesterday    int64 // previous day's price
	BestBid      int64 // best demand price
	BestAsk      int64 // best offer price
	BidVolume    int64
	AskVolume    int64
	Volume       int64 // traded quantity today
	Value        int64 // traded value today (Rial)
	OpenInterest int64 // open positions ("موقعیت‌های باز")
}

// OptionRow is one strike/expiry on an underlying, pairing the call and put
// legs (either may be nil if that side is not listed), together with a snapshot
// of the underlying's price.
type OptionRow struct {
	UnderlyingInsCode   string
	UnderlyingSymbol    string
	UnderlyingLast      int64
	UnderlyingClose     int64
	UnderlyingYesterday int64

	StrikePrice  int64
	BeginDate    time.Time
	ExpiryDate   time.Time
	RemainedDays int
	ContractSize int

	Call *OptionLeg
	Put  *OptionLeg
}

// Source is a provider of options-market data. A provider need not implement
// every method; unsupported ones return ErrNotSupported.
//
// New code that needs the whole option market at once (the daily snapshot, a
// collection service) should depend on MarketSource (market.go), also available
// as source.Source. This interface stays as it is, for the collector.
type Source interface {
	// Name is a short identifier for the provider, e.g. "tsetmc".
	Name() string

	// OptionMarketWatch returns the full live option chain (all listed
	// contracts with their current quotes and underlying prices).
	OptionMarketWatch(ctx context.Context) ([]OptionRow, error)

	// DailyPrices returns the end-of-day history for an instrument. When
	// adjusted is true the series is corrected for capital actions.
	DailyPrices(ctx context.Context, insCode string, adjusted bool) ([]DailyPrice, error)

	// InstrumentInfo resolves an instrument code to its identity.
	InstrumentInfo(ctx context.Context, insCode string) (*InstrumentInfo, error)
}
