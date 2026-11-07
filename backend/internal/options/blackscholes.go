package options

import (
	"fmt"
	"math"
)

// invSqrt2Pi is 1/sqrt(2*pi), the normalising constant of the standard normal
// density.
const invSqrt2Pi = 0.3989422804014327

// normCDF is the cumulative distribution function of the standard normal.
// It is written with Erfc rather than 1-Erf because Erfc keeps its precision
// in the far tails, which is where deep out-of-the-money contracts live.
func normCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// normPDF is the probability density function of the standard normal.
func normPDF(x float64) float64 {
	return invSqrt2Pi * math.Exp(-0.5*x*x)
}

// d1d2 returns the two Black-Scholes auxiliary terms. It assumes the caller has
// already ruled out the degenerate cases where volatility or time is zero.
func d1d2(in Inputs, vol float64) (d1, d2 float64) {
	stdDev := vol * math.Sqrt(in.TimeToExpiry)
	d1 = (math.Log(in.Spot/in.Strike) +
		(in.Rate-in.DividendYield+0.5*vol*vol)*in.TimeToExpiry) / stdDev
	return d1, d1 - stdDev
}

// isDegenerate reports whether the contract has no remaining optionality,
// either because it expires today or because volatility is zero. In both cases
// the payoff is deterministic and the Greeks (except delta) vanish.
func isDegenerate(in Inputs, vol float64) bool {
	return in.TimeToExpiry <= 0 || vol <= 0
}

// discountedIntrinsic prices a contract with no remaining optionality: the
// forward price is known, so the option is worth the discounted payoff of that
// forward.
func discountedIntrinsic(in Inputs) float64 {
	forward := in.Spot * math.Exp(-in.DividendYield*in.TimeToExpiry)
	strike := in.Strike * math.Exp(-in.Rate*in.TimeToExpiry)
	if in.Kind == Call {
		return math.Max(forward-strike, 0)
	}
	return math.Max(strike-forward, 0)
}

// Price returns the Black-Scholes theoretical value of one share of the
// contract, given a volatility.
//
// Pass historical volatility to get the theoretical price shown next to the
// market price in the watch list, or pass implied volatility to recover the
// market price itself.
func Price(in Inputs, vol float64) (float64, error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	if vol < 0 || math.IsNaN(vol) || math.IsInf(vol, 0) {
		return 0, fmt.Errorf("%w: volatility must be finite and non-negative, got %g", ErrInvalidInput, vol)
	}
	return price(in, vol), nil
}

// price is the unvalidated core, shared with the implied-volatility solver so
// that its inner loop does not revalidate on every iteration.
func price(in Inputs, vol float64) float64 {
	if isDegenerate(in, vol) {
		return discountedIntrinsic(in)
	}

	d1, d2 := d1d2(in, vol)
	carry := in.Spot * math.Exp(-in.DividendYield*in.TimeToExpiry)
	strike := in.Strike * math.Exp(-in.Rate*in.TimeToExpiry)

	if in.Kind == Call {
		return carry*normCDF(d1) - strike*normCDF(d2)
	}
	return strike*normCDF(-d2) - carry*normCDF(-d1)
}

// Forward returns the forward price of the underlying at expiry. It is the
// reference point for moneyness once carry is taken into account.
func Forward(in Inputs) (float64, error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	return in.Spot * math.Exp((in.Rate-in.DividendYield)*in.TimeToExpiry), nil
}
