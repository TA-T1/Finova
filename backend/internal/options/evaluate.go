package options

import "errors"

// Quote is everything known about one contract at one instant: its model
// parameters plus the two observed prices the engine needs.
type Quote struct {
	Inputs
	// MarketPrice is the traded or mid price of the option, per share.
	MarketPrice float64
	// HistoricalVolatility is the realised volatility of the underlying,
	// produced by the data team. It is optional: leave it at zero and the
	// theoretical price is simply not reported.
	HistoricalVolatility float64
}

// Evaluation is one fully computed watch-list row.
//
// Availability flags are used instead of errors because a single unusable
// quote must not fail a request covering a thousand contracts. A row with
// HasImpliedVolatility false still carries valid intrinsic value, break-even
// and leverage, so the table stays useful.
type Evaluation struct {
	// ImpliedVolatility is the volatility implied by MarketPrice.
	ImpliedVolatility float64
	// HasImpliedVolatility is false when the quote lies outside the
	// no-arbitrage bounds, which usually means stale or crossed data.
	HasImpliedVolatility bool

	// Greeks are evaluated at GreeksVolatility.
	Greeks Greeks
	// GreeksVolatility records which volatility the Greeks were computed
	// from: the implied one when available, otherwise the historical one.
	// Exposing it keeps the table honest about what the numbers mean.
	GreeksVolatility float64

	// Metrics are derived from MarketPrice and do not depend on the model.
	Metrics Metrics

	// TheoreticalPrice is the Black-Scholes value at the historical
	// volatility, and PriceGap is how far the market price sits above it.
	TheoreticalPrice float64
	PriceGap         float64
	PriceGapPercent  float64
	// HasTheoreticalPrice is false when no historical volatility was given.
	HasTheoreticalPrice bool
}

// Evaluate computes every derived column for one contract in a single pass.
// It is the entry point the API handlers and the collector should use; the
// individual functions remain exported for tests and for callers that need
// only one number.
//
// An error is returned only for inputs that cannot be sensible under any
// market data, such as a negative strike. Quotes that are merely inconsistent
// are reported through the availability flags.
func Evaluate(q Quote) (Evaluation, error) {
	if err := q.Validate(); err != nil {
		return Evaluation{}, err
	}

	var ev Evaluation

	if q.MarketPrice > 0 && q.TimeToExpiry > 0 {
		iv, err := ImpliedVolatility(q.Inputs, q.MarketPrice)
		switch {
		case err == nil:
			ev.ImpliedVolatility, ev.HasImpliedVolatility = iv, true
		case errors.Is(err, ErrPriceOutOfRange):
			// Expected for bad quotes; fall through to the historical volatility.
		default:
			return Evaluation{}, err
		}
	}

	// Prefer implied volatility for the Greeks, because it reflects what the
	// market is actually paying. Fall back to historical volatility so that a
	// contract with no trades still shows a usable delta.
	switch {
	case ev.HasImpliedVolatility:
		ev.GreeksVolatility = ev.ImpliedVolatility
	case q.HistoricalVolatility > 0:
		ev.GreeksVolatility = q.HistoricalVolatility
	}
	ev.Greeks = calcGreeks(q.Inputs, ev.GreeksVolatility)

	metrics, err := CalcMetrics(q.Inputs, q.MarketPrice, ev.Greeks.Delta)
	if err != nil {
		return Evaluation{}, err
	}
	ev.Metrics = metrics

	if q.HistoricalVolatility > 0 {
		ev.TheoreticalPrice = price(q.Inputs, q.HistoricalVolatility)
		ev.HasTheoreticalPrice = true
		ev.PriceGap = q.MarketPrice - ev.TheoreticalPrice
		if ev.TheoreticalPrice > 0 {
			ev.PriceGapPercent = ev.PriceGap / ev.TheoreticalPrice * 100
		}
	}

	return ev, nil
}
