package market

import (
	"math"
	"time"

	"Finova/internal/options"
)

// DefaultRiskFreeRate is the annual risk-free rate used when the collector is
// built without WithRiskFreeRate. It is a decimal fraction (0.30 = 30%).
// config.Load applies the same default; override it with RISK_FREE_RATE.
const DefaultRiskFreeRate = 0.30

// Values of option_metrics.price_source.
const (
	priceSourceLast  = "last"
	priceSourceMid   = "mid"
	priceSourceClose = "close"
)

// MetricInput is everything needed to calculate and store metrics for one quote.
type MetricInput struct {
	ContractID   int
	UnderlyingID int
	Kind         options.Kind
	Strike       float64
	Expiry       time.Time
	Spot         float64
	// TS is the quote timestamp, or an explicit timestamp override.
	TS           time.Time

	// Option prices as reported by the source; 0 means missing.
	Last  float64
	Bid   float64
	Ask   float64
	Close float64

	// HV is the HV20 of the underlying, or nil when it is unknown.
	HV *float64
}

type metricInput = MetricInput

// OptionMetrics is one calculated row of option_metrics. A nil pointer means
// "not available" and is stored as NULL.
type OptionMetrics struct {
	ContractID int
	TS         time.Time

	UnderlyingPrice float64
	DaysToExpiry    int
	RiskFreeRate    float64
	MarketPrice     *float64
	PriceSource     *string

	Moneyness         string
	IntrinsicValue    float64
	TimeValue         *float64
	BreakEven         *float64
	BreakEvenPercent  *float64
	Leverage          *float64
	EffectiveLeverage *float64

	ImpliedVol    *float64
	HistoricalVol *float64
	GreeksVol     *float64
	Delta         *float64
	Gamma         *float64
	VegaPerPct    *float64
	ThetaPerDay   *float64
	RhoPerPct     *float64

	TheoreticalPrice *float64
	PriceGap         *float64
	PriceGapPercent  *float64
}

// pickMarketPrice chooses the option price the metrics are based on:
//
//  1. the last traded price, if there was a trade (last > 0);
//  2. otherwise the middle of a valid bid/ask pair;
//  3. otherwise the closing (settlement) price.
//
// It returns 0 and an empty source when none of them is usable. The source is
// stored next to the numbers so a reader can tell a traded price from a quote.
func pickMarketPrice(last, bid, ask, closePrice float64) (float64, string) {
	switch {
	case last > 0:
		return last, priceSourceLast
	case bid > 0 && ask >= bid:
		return (bid + ask) / 2, priceSourceMid
	case closePrice > 0:
		return closePrice, priceSourceClose
	default:
		return 0, ""
	}
}

// pickSpot chooses the underlying price: last trade, else the closing price.
// It returns 0 when neither is available. The previous day's close is not used,
// because a stale spot would make moneyness and every Greek look valid when
// they are not.
func pickSpot(last, closePrice float64) float64 {
	switch {
	case last > 0:
		return last
	case closePrice > 0:
		return closePrice
	default:
		return 0
	}
}

// computeMetrics calculates every stored column for one contract. ts is the
// collection time. It only returns an error for inputs the engine rejects
// (for example a non-positive spot); an unusable quote just leaves the
// dependent columns nil.
func computeMetrics(in metricInput, ts time.Time, rate float64) (OptionMetrics, error) {
	// Work in whole Tehran calendar days, the same way the chain endpoints
	// decide what is still active.
	local := ts.In(tehranLocation)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tehranLocation)
	ey, em, ed := in.Expiry.Date()
	expiryDay := time.Date(ey, em, ed, 0, 0, 0, 0, tehranLocation)

	days := int(math.Round(expiryDay.Sub(today).Hours() / 24))
	if days < 0 {
		days = 0
	}
	years := options.YearsToExpiry(local, expiryDay)

	price, source := pickMarketPrice(in.Last, in.Bid, in.Ask, in.Close)
	hv := 0.0
	if in.HV != nil {
		hv = *in.HV
	}

	q := options.Quote{
		Inputs: options.Inputs{
			Kind:         in.Kind,
			Spot:         in.Spot,
			Strike:       in.Strike,
			TimeToExpiry: years,
			Rate:         rate,
		},
		MarketPrice:          price,
		HistoricalVolatility: hv,
	}
	ev, err := options.Evaluate(q)
	if err != nil {
		return OptionMetrics{}, err
	}

	m := OptionMetrics{
		ContractID:      in.ContractID,
		TS:              ts,
		UnderlyingPrice: in.Spot,
		DaysToExpiry:    days,
		RiskFreeRate:    rate,
		Moneyness:       string(ev.Metrics.Moneyness),
		IntrinsicValue:  ev.Metrics.IntrinsicValue,
		HistoricalVol:   positive(hv),
	}

	// Metrics that need an option price.
	if price > 0 {
		m.MarketPrice = finite(price)
		m.PriceSource = &source
		m.TimeValue = finite(ev.Metrics.TimeValue)
		m.BreakEven = finite(ev.Metrics.BreakEven)
		m.BreakEvenPercent = finite(ev.Metrics.BreakEvenPercent)
		m.Leverage = finite(ev.Metrics.Leverage)
	}
	if ev.HasImpliedVolatility {
		m.ImpliedVol = positive(ev.ImpliedVolatility)
	}

	// Greeks. Evaluate uses implied volatility and falls back to HV20. With
	// neither, the "Greeks" it returns are the step-function limits, which
	// would look like real values, so they are only kept on expiry day.
	greeks := ev.Greeks
	greeksVol := ev.GreeksVolatility
	effectiveLeverage := ev.Metrics.EffectiveLeverage
	greeksKnown := greeksVol > 0 || years == 0

	// A price at or below intrinsic value gives an implied volatility of 0,
	// and Evaluate then returns step-function Greeks. Use HV20 instead when
	// there is one; this is common for deep in-the-money contracts.
	if years > 0 && ev.HasImpliedVolatility && ev.ImpliedVolatility <= 0 && hv > 0 {
		if g, err := options.CalcGreeks(q.Inputs, hv); err == nil {
			greeks, greeksVol, greeksKnown = g, hv, true
			effectiveLeverage = g.Delta * ev.Metrics.Leverage
		}
	}

	if greeksKnown {
		m.GreeksVol = positive(greeksVol)
		m.Delta = finite(greeks.Delta)
		m.Gamma = finite(greeks.Gamma)
		m.VegaPerPct = finite(greeks.VegaPerPercent())
		m.ThetaPerDay = finite(greeks.ThetaPerDay())
		m.RhoPerPct = finite(greeks.RhoPerPercent())
		if price > 0 {
			m.EffectiveLeverage = finite(effectiveLeverage)
		}
	}

	// Theoretical price: Black-Scholes at HV20.
	if ev.HasTheoreticalPrice {
		m.TheoreticalPrice = finite(ev.TheoreticalPrice)
		if price > 0 {
			m.PriceGap = finite(ev.PriceGap)
			if ev.TheoreticalPrice > 0 {
				m.PriceGapPercent = finite(ev.PriceGapPercent)
			}
		}
	}
	return m, nil
}

// finite returns a pointer to v, or nil when v is NaN or infinite.
func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// positive returns a pointer to v, or nil unless v is a finite number above 0.
func positive(v float64) *float64 {
	if !(v > 0) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}
