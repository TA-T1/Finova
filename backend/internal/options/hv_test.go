package options

import (
	"errors"
	"math"
	"testing"
)

func TestHistoricalVolatilityConstantSeriesIsZero(t *testing.T) {
	closes := make([]float64, HVWindow+1)
	for i := range closes {
		closes[i] = 12500
	}
	got, err := HV20(closes)
	if err != nil {
		t.Fatalf("HV20: %v", err)
	}
	if got != 0 {
		t.Errorf("HV20 of a flat series = %g, want 0", got)
	}
}

func TestHistoricalVolatilityAlternatingReturns(t *testing.T) {
	// Daily log returns alternate +1% and -1%, so their mean is 0 and the
	// sample variance is 20 * 0.01^2 / 19.
	closes := make([]float64, HVWindow+1)
	closes[0] = 10000
	for i := 1; i < len(closes); i++ {
		r := 0.01
		if i%2 == 0 {
			r = -0.01
		}
		closes[i] = closes[i-1] * math.Exp(r)
	}
	want := 0.01 * math.Sqrt(20.0/19.0) * math.Sqrt(TradingDaysPerYear)

	got, err := HV20(closes)
	if err != nil {
		t.Fatalf("HV20: %v", err)
	}
	closeTo(t, "HV20", got, want, 1e-9)
}

func TestHistoricalVolatilityRejectsBadInput(t *testing.T) {
	tests := []struct {
		name   string
		closes []float64
		days   float64
	}{
		{"too few prices", []float64{100, 101}, TradingDaysPerYear},
		{"zero price", []float64{100, 0, 102}, TradingDaysPerYear},
		{"negative price", []float64{100, -5, 102}, TradingDaysPerYear},
		{"nan price", []float64{100, math.NaN(), 102}, TradingDaysPerYear},
		{"infinite price", []float64{100, math.Inf(1), 102}, TradingDaysPerYear},
		{"zero trading days", []float64{100, 101, 102}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := HistoricalVolatility(tc.closes, tc.days)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestHV20RequiresFullWindow(t *testing.T) {
	_, err := HV20(make([]float64, HVWindow))
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("error = %v, want ErrInvalidInput", err)
	}
}
