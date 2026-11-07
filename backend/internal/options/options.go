// Package options implements the option calculation engine: Black-Scholes
// pricing, the Greeks, implied volatility, break-even and leverage.
//
// The package is deliberately pure. It imports nothing but the standard
// library, touches no database and knows nothing about HTTP. Callers load
// quotes, convert them to Inputs.
//
// Conventions:
//
//   - All money values are float64 . Prices in the database are integers; convert at
//     the boundary, not inside the engine.
//   - Prices are per share, not per contract. Multiply by contract size at
//     the presentation layer.
//   - Rates and volatilities are decimal fractions per year: 0.30 means 30%.
//   - Time to expiry is in years. Use YearsToExpiry to derive it from dates.

package options

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Kind distinguishes the two option types.
type Kind uint8

const (
	// Call is the right to buy the underlying at the strike price.
	Call Kind = iota + 1
	// Put is the right to sell the underlying at the strike price.
	Put
)

func (k Kind) String() string {
	switch k {
	case Call:
		return "call"
	case Put:
		return "put"
	default:
		return "unknown"
	}
}

// reports whether k is a known option type.
func (k Kind) Valid() bool { return k == Call || k == Put }

// ParseKind converts the option_type values stored in Postgres into a Kind.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "call":
		return Call, nil
	case "put":
		return Put, nil
	default:
		return 0, fmt.Errorf("%w: option type %q", ErrInvalidInput, s)
	}
}

// Errors returned by this package. Callers should match with errors.Is rather
// than comparing strings, so the HTTP layer can map them to status codes.
var (
	// ErrInvalidInput means a parameter is outside its allowed domain, for
	// example a negative spot price or a volatility below zero.
	ErrInvalidInput = errors.New("options: invalid input")

	// ErrPriceOutOfRange means the observed market price lies outside the
	// no-arbitrage bounds of the model, so no implied volatility exists.
	// In live data this usually signals a stale or crossed quote.
	ErrPriceOutOfRange = errors.New("options: market price outside no-arbitrage bounds")

	// ErrNoConvergence means the solver hit its iteration limit. It should
	// not happen with the bracketed solver, but is returned instead of a
	// silently wrong number if it ever does.
	ErrNoConvergence = errors.New("options: implied volatility did not converge")
)

// Inputs holds the model parameters that do not depend on volatility.
// Volatility is passed separately because it is an input to pricing and an
// output of the implied-volatility solver.
type Inputs struct {
	Kind Kind
	// Spot is the current price of the underlying share (S).
	Spot float64
	// Strike is the exercise price of the contract (K).
	Strike float64
	// TimeToExpiry is the remaining life in years (T). Zero means expiry day.
	TimeToExpiry float64
	// Rate is the continuously compounded risk-free rate per year (r).
	Rate float64
	// DividendYield is the continuous dividend yield per year (q). Iranian
	// equities pay a single lump dividend at the annual meeting rather than a
	// continuous yield, so leave this at zero unless you are approximating a
	// known upcoming payout.
	DividendYield float64
}

// Validate checks the parameters that every calculation depends on.
func (in Inputs) Validate() error {
	switch {
	case !in.Kind.Valid():
		return fmt.Errorf("%w: unknown option kind", ErrInvalidInput)
	case in.Spot <= 0:
		return fmt.Errorf("%w: spot must be positive, got %g", ErrInvalidInput, in.Spot)
	case in.Strike <= 0:
		return fmt.Errorf("%w: strike must be positive, got %g", ErrInvalidInput, in.Strike)
	case in.TimeToExpiry < 0:
		return fmt.Errorf("%w: time to expiry must not be negative, got %g", ErrInvalidInput, in.TimeToExpiry)
	case math.IsNaN(in.Rate) || math.IsInf(in.Rate, 0):
		return fmt.Errorf("%w: rate must be finite", ErrInvalidInput)
	case math.IsNaN(in.DividendYield) || math.IsInf(in.DividendYield, 0):
		return fmt.Errorf("%w: dividend yield must be finite", ErrInvalidInput)
	}
	return nil
}

// DaysPerYear is the day-count basis (actual/365). The Tehran exchange trades
// roughly 240 days a year, but quoting volatility on a calendar basis is the
// market convention and keeps IV comparable with published figures.
const DaysPerYear = 365.0

// YearsToExpiry converts a settlement date into a year fraction, measured from
// now. It never returns a negative value: an expired contract yields zero,
// which the pricing functions handle as intrinsic value.
//
// Both times are truncated to their calendar day, so a contract expiring today
// has zero time left regardless of the hour.
func YearsToExpiry(now, expiry time.Time) float64 {
	d := truncateDay(expiry).Sub(truncateDay(now))
	if d <= 0 {
		return 0
	}
	return d.Hours() / 24 / DaysPerYear
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
