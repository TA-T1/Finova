package options

import (
	"fmt"
	"math"
)

// TradingDaysPerYear is the annualisation factor for historical volatility.
// The Tehran exchange trades roughly 240 days a year (see DaysPerYear), and a
// daily return series only has observations on trading days, so its variance
// is scaled by trading days, not by calendar days.
const TradingDaysPerYear = 240.0

// HVWindow is the number of daily returns behind HV20.
const HVWindow = 20

// HistoricalVolatility is the annualised realised volatility of a price series:
// the sample standard deviation of daily log returns, scaled by
// sqrt(tradingDays). The result is a decimal fraction per year (0.30 = 30%).
//
// closes must be ordered oldest first, one price per trading day. At least
// three prices (two returns) are needed because the sample standard deviation
// divides by n-1. Prices must be positive and finite.
//
// Pass back-adjusted prices. A capital increase or dividend adjustment in a raw
// series is a one-day jump that the model would read as volatility.
func HistoricalVolatility(closes []float64, tradingDays float64) (float64, error) {
	if len(closes) < 3 {
		return 0, fmt.Errorf("%w: need at least 3 prices, got %d", ErrInvalidInput, len(closes))
	}
	if !(tradingDays > 0) || math.IsInf(tradingDays, 0) {
		return 0, fmt.Errorf("%w: trading days per year must be positive and finite, got %g", ErrInvalidInput, tradingDays)
	}

	returns := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		prev, cur := closes[i-1], closes[i]
		if !(prev > 0) || !(cur > 0) || math.IsInf(prev, 0) || math.IsInf(cur, 0) {
			return 0, fmt.Errorf("%w: prices must be positive and finite (around index %d)", ErrInvalidInput, i)
		}
		returns = append(returns, math.Log(cur/prev))
	}

	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	var squares float64
	for _, r := range returns {
		d := r - mean
		squares += d * d
	}
	variance := squares / float64(len(returns)-1)
	return math.Sqrt(variance * tradingDays), nil
}

// HV20 is the 20-day historical volatility used for the theoretical price. It
// takes exactly HVWindow+1 closes (oldest first), which give HVWindow returns.
func HV20(closes []float64) (float64, error) {
	if len(closes) != HVWindow+1 {
		return 0, fmt.Errorf("%w: HV20 needs %d prices, got %d", ErrInvalidInput, HVWindow+1, len(closes))
	}
	return HistoricalVolatility(closes, TradingDaysPerYear)
}
