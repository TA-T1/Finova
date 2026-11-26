// Package source is the market-data entry point named in the project brief
// (docs/01-crawl-adapter.md): Source, MarketSnapshot, OptionContract,
// UnderlyingQuote and FakeSource.
//
// Everything here is an alias of the types in internal/datasource, which is
// where the repository's existing provider code lives (datasource.Source,
// OptionRow, the tsetmc client...). The two names are interchangeable, nothing
// is duplicated and the older datasource API, which the collector depends on,
// is unchanged. New code (the daily snapshot, the collection service) can
// depend on this package.
//
//	var src source.Source = tsetmc.New(...) // live
//	src, _ := source.NewFake("testdata/market_sample.json") // offline
package source

import "Finova/internal/datasource"

type (
	// Source returns one consistent snapshot of the option market per call.
	Source = datasource.MarketSource
	// MarketSnapshot is one consistent view of the market.
	MarketSnapshot = datasource.MarketSnapshot
	// OptionContract is one option instrument with its latest quote.
	OptionContract = datasource.OptionContract
	// UnderlyingQuote is the latest quote of an underlying that has options.
	UnderlyingQuote = datasource.UnderlyingQuote
	// FakeSource serves a saved snapshot, for offline development and tests.
	FakeSource = datasource.FakeSource
)

// NewFake loads a MarketSnapshot JSON file (e.g. testdata/market_sample.json).
func NewFake(path string) (*FakeSource, error) { return datasource.NewFake(path) }

// NewFakeFromJSON builds a FakeSource from raw snapshot JSON.
func NewFakeFromJSON(b []byte) (*FakeSource, error) { return datasource.NewFakeFromJSON(b) }

// NewFakeFromSnapshot builds a FakeSource from an in-memory snapshot.
func NewFakeFromSnapshot(s *MarketSnapshot) (*FakeSource, error) {
	return datasource.NewFakeFromSnapshot(s)
}
