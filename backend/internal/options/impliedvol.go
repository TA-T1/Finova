package options

import (
	"fmt"
	"math"
)

// MaxVolatility caps the search range of the solver at 500% per year. Real
// contracts on the Tehran exchange trade far below this even in a panic, so a
// quote that implies more than the cap is almost certainly bad data.
const MaxVolatility = 5.0

const (
	// maxIterations bounds the solver. Pure bisection on [0, MaxVolatility]
	// reaches machine precision in about 55 steps, so this is generous.
	maxIterations = 100
	// volTolerance stops the search once the bracket is narrower than this.
	volTolerance = 1e-10
	// relPriceTolerance stops the search once the model price matches the
	// market price to this relative accuracy. It is relative because prices
	// are in rial and can be six or seven digits.
	relPriceTolerance = 1e-10
)

// PriceBounds returns the no-arbitrage range of an option price, before any
// volatility is assumed. A call is worth at least the discounted forward minus
// the discounted strike and never more than the discounted spot; the mirrored
// statement holds for puts.
//
// The watch list can use this directly to flag crossed or stale quotes.
func PriceBounds(in Inputs) (low, high float64, err error) {
	if err := in.Validate(); err != nil {
		return 0, 0, err
	}
	carry := in.Spot * math.Exp(-in.DividendYield*in.TimeToExpiry)
	strike := in.Strike * math.Exp(-in.Rate*in.TimeToExpiry)
	if in.Kind == Call {
		return math.Max(carry-strike, 0), carry, nil
	}
	return math.Max(strike-carry, 0), strike, nil
}

// ImpliedVolatility solves for the volatility that makes the Black-Scholes
// price equal the observed market price.
//
// It uses Newton's method, which converges quadratically near the solution,
// but keeps a bracket around the root and falls back to bisection whenever a
// Newton step would jump outside it or vega is too small to divide by. That
// combination is both fast for ordinary contracts and safe for the deep
// out-of-the-money ones where vega collapses and plain Newton diverges.
//
// It returns ErrPriceOutOfRange when no volatility can reproduce the price,
// which in live data means the quote should be skipped rather than displayed.
func ImpliedVolatility(in Inputs, marketPrice float64) (float64, error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	if marketPrice <= 0 || math.IsNaN(marketPrice) || math.IsInf(marketPrice, 0) {
		return 0, fmt.Errorf("%w: market price must be positive and finite, got %g", ErrInvalidInput, marketPrice)
	}
	if in.TimeToExpiry <= 0 {
		return 0, fmt.Errorf("%w: contract has expired, volatility is undefined", ErrInvalidInput)
	}

	low, high, err := PriceBounds(in)
	if err != nil {
		return 0, err
	}
	tol := relPriceTolerance * math.Max(1, marketPrice)

	switch {
	case marketPrice <= low+tol:
		// The option trades at or below intrinsic value: the only consistent
		// volatility is zero. Reporting zero is more useful to the watch list
		// than an error, because this happens routinely on deep in-the-money
		// contracts with a wide spread.
		return 0, nil
	case marketPrice >= high-tol:
		return 0, fmt.Errorf("%w: price %g is at or above the upper bound %g", ErrPriceOutOfRange, marketPrice, high)
	}

	if price(in, MaxVolatility) < marketPrice {
		return 0, fmt.Errorf("%w: price %g implies volatility above the %g cap", ErrPriceOutOfRange, marketPrice, MaxVolatility)
	}

	return solve(in, marketPrice, tol)
}

// solve runs the safeguarded Newton iteration. The caller guarantees that a
// root exists strictly inside (0, MaxVolatility).
func solve(in Inputs, marketPrice, tol float64) (float64, error) {
	lo, hi := 0.0, MaxVolatility
	vol := initialGuess(in, marketPrice)

	for range maxIterations {
		diff := price(in, vol) - marketPrice
		if math.Abs(diff) <= tol {
			return vol, nil
		}

		// Price rises monotonically with volatility, so the sign of the
		// residual tells us which side of the root we are on.
		if diff > 0 {
			hi = vol
		} else {
			lo = vol
		}
		if hi-lo < volTolerance {
			return 0.5 * (lo + hi), nil
		}

		next := math.NaN()
		if vega := calcGreeks(in, vol).Vega; vega > 1e-12 {
			next = vol - diff/vega
		}
		// Reject a Newton step that leaves the bracket or is not a number,
		// and bisect instead. This is what keeps the solver from diverging on
		// contracts with almost no vega.
		if math.IsNaN(next) || next <= lo || next >= hi {
			next = 0.5 * (lo + hi)
		}
		vol = next
	}
	return 0, fmt.Errorf("%w: after %d iterations", ErrNoConvergence, maxIterations)
}

// initialGuess uses the Brenner-Subrahmanyam approximation, which is exact for
// an at-the-money option and close enough elsewhere to save a few iterations.
func initialGuess(in Inputs, marketPrice float64) float64 {
	guess := math.Sqrt(2*math.Pi/in.TimeToExpiry) * marketPrice / in.Spot
	if math.IsNaN(guess) || guess <= 0 || guess >= MaxVolatility {
		return 0.5
	}
	return guess
}
