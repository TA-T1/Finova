package options

import (
	"fmt"
	"math"
)

// Greeks are the first and second order sensitivities of the option price.
//
// The raw values use the natural units of the model: per one currency unit of
// spot, per one full point of volatility (1.00 = 100%) and per year. Traders
// usually read vega per 1% and theta per day, so the convenience methods
// VegaPerPercent, ThetaPerDay and RhoPerPercent do that scaling in one place
// instead of leaving every caller to remember the factor.
type Greeks struct {
	// Delta is the change in option price per unit change in spot.
	// Calls are in [0, 1], puts in [-1, 0].
	Delta float64
	// Gamma is the change in delta per unit change in spot.
	Gamma float64
	// Vega is the change in option price per 1.00 change in volatility.
	Vega float64
	// Theta is the change in option price per year of elapsed time.
	// It is normally negative: a long option loses value as expiry nears.
	Theta float64
	// Rho is the change in option price per 1.00 change in the risk-free rate.
	Rho float64
}

// VegaPerPercent restates vega per one percentage point of volatility.
func (g Greeks) VegaPerPercent() float64 { return g.Vega / 100 }

// ThetaPerDay restates theta per calendar day.
func (g Greeks) ThetaPerDay() float64 { return g.Theta / DaysPerYear }

// RhoPerPercent restates rho per one percentage point of interest rate.
func (g Greeks) RhoPerPercent() float64 { return g.Rho / 100 }

// CalcGreeks returns the sensitivities of the contract at the given volatility.
//
// At expiry, or at zero volatility, the payoff is deterministic: delta becomes
// a step function and the remaining Greeks are zero. The function returns that
// limit rather than dividing by zero.
func CalcGreeks(in Inputs, vol float64) (Greeks, error) {
	if err := in.Validate(); err != nil {
		return Greeks{}, err
	}
	if vol < 0 || math.IsNaN(vol) || math.IsInf(vol, 0) {
		return Greeks{}, fmt.Errorf("%w: volatility must be finite and non-negative, got %g", ErrInvalidInput, vol)
	}
	return calcGreeks(in, vol), nil
}

func calcGreeks(in Inputs, vol float64) Greeks {
	if isDegenerate(in, vol) {
		return Greeks{Delta: degenerateDelta(in)}
	}

	var (
		T       = in.TimeToExpiry
		sqrtT   = math.Sqrt(T)
		d1, d2  = d1d2(in, vol)
		pdf     = normPDF(d1)
		carryDF = math.Exp(-in.DividendYield * T)
		rateDF  = math.Exp(-in.Rate * T)
	)

	g := Greeks{
		Gamma: carryDF * pdf / (in.Spot * vol * sqrtT),
		Vega:  in.Spot * carryDF * pdf * sqrtT,
	}

	// Common to both kinds: the decay of the time value itself.
	decay := -in.Spot * pdf * vol * carryDF / (2 * sqrtT)

	if in.Kind == Call {
		g.Delta = carryDF * normCDF(d1)
		g.Theta = decay - in.Rate*in.Strike*rateDF*normCDF(d2) +
			in.DividendYield*in.Spot*carryDF*normCDF(d1)
		g.Rho = in.Strike * T * rateDF * normCDF(d2)
		return g
	}

	g.Delta = carryDF * (normCDF(d1) - 1)
	g.Theta = decay + in.Rate*in.Strike*rateDF*normCDF(-d2) -
		in.DividendYield*in.Spot*carryDF*normCDF(-d1)
	g.Rho = -in.Strike * T * rateDF * normCDF(-d2)
	return g
}

// degenerateDelta is the limit of delta when no optionality remains.
// Exactly at the money the left and right limits differ, so the midpoint is
// used as the conventional value.
func degenerateDelta(in Inputs) float64 {
	forward := in.Spot * math.Exp((in.Rate-in.DividendYield)*in.TimeToExpiry)
	switch {
	case forward > in.Strike:
		if in.Kind == Call {
			return 1
		}
		return 0
	case forward < in.Strike:
		if in.Kind == Call {
			return 0
		}
		return -1
	default:
		if in.Kind == Call {
			return 0.5
		}
		return -0.5
	}
}
