package options

import "math"

// Moneyness classifies a contract against the current spot price.
type Moneyness string

const (
	InTheMoney    Moneyness = "ITM"
	AtTheMoney    Moneyness = "ATM"
	OutOfTheMoney Moneyness = "OTM"
)

// atmBand is the half-width, as a fraction of spot, within which a contract is
// reported as at the money. Strikes on the Tehran exchange are spaced widely
// enough that an exact equality test would almost never fire.
const atmBand = 0.005

// Metrics are the decision numbers a trader reads off the watch list. They
// depend on the market price of the contract, not on the model, which is why
// they are separate from Greeks.
type Metrics struct {
	// IntrinsicValue is what the contract would pay if exercised now.
	IntrinsicValue float64
	// TimeValue is the premium above intrinsic value. It decays to zero at
	// expiry and can be slightly negative on a stale or crossed quote.
	TimeValue float64
	// Moneyness classifies the strike against spot.
	Moneyness Moneyness
	// BreakEven is the underlying price at which the position stops losing
	// money at expiry, per share and ignoring fees.
	BreakEven float64
	// BreakEvenPercent is the move in the underlying needed to reach
	// BreakEven, as a percentage of spot. Negative means spot is already past
	// the break-even point.
	BreakEvenPercent float64
	// Leverage is spot divided by premium: how many shares' worth of exposure
	// one unit of premium controls, before accounting for delta.
	Leverage float64
	// EffectiveLeverage (elasticity, sometimes lambda or omega) is the
	// percentage change in option value per one percent change in the
	// underlying. It is the number that actually describes the risk of the
	// position, because it discounts raw leverage by delta.
	EffectiveLeverage float64
}

// CalcMetrics derives the trader-facing numbers from the contract parameters
// and its market price. Delta comes from CalcGreeks at the implied volatility;
// pass zero if you only need the non-delta fields.
func CalcMetrics(in Inputs, marketPrice, delta float64) (Metrics, error) {
	if err := in.Validate(); err != nil {
		return Metrics{}, err
	}

	m := Metrics{
		IntrinsicValue: IntrinsicValue(in),
		Moneyness:      ClassifyMoneyness(in),
	}
	m.TimeValue = marketPrice - m.IntrinsicValue

	if in.Kind == Call {
		m.BreakEven = in.Strike + marketPrice
	} else {
		m.BreakEven = in.Strike - marketPrice
	}
	m.BreakEvenPercent = (m.BreakEven - in.Spot) / in.Spot * 100

	// Leverage is undefined for a contract with no premium; leaving it at
	// zero is safer than returning an infinity that would poison a JSON
	// response or a sort order in the watch list.
	if marketPrice > 0 {
		m.Leverage = in.Spot / marketPrice
		m.EffectiveLeverage = delta * m.Leverage
	}
	return m, nil
}

// IntrinsicValue is the immediate exercise value of one share of the contract.
func IntrinsicValue(in Inputs) float64 {
	if in.Kind == Call {
		return math.Max(in.Spot-in.Strike, 0)
	}
	return math.Max(in.Strike-in.Spot, 0)
}

// ClassifyMoneyness compares the strike with spot, treating strikes within
// atmBand of spot as at the money.
func ClassifyMoneyness(in Inputs) Moneyness {
	diff := (in.Spot - in.Strike) / in.Spot
	switch {
	case math.Abs(diff) <= atmBand:
		return AtTheMoney
	case in.Kind == Call && diff > 0, in.Kind == Put && diff < 0:
		return InTheMoney
	default:
		return OutOfTheMoney
	}
}
