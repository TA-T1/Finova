package market

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"Finova/internal/options"
)

// near fails the test unless got is within a relative tolerance of want.
func near(t *testing.T, label string, got *float64, want, tol float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want %g", label, want)
		return
	}
	if math.Abs(*got-want) > tol*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %.10g, want %.10g", label, *got, want)
	}
}

func isNil(t *testing.T, label string, got *float64) {
	t.Helper()
	if got != nil {
		t.Errorf("%s = %g, want nil (NULL)", label, *got)
	}
}

func notNil(t *testing.T, label string, got *float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want a value", label)
	}
}

func TestPickMarketPrice(t *testing.T) {
	tests := []struct {
		name                  string
		last, bid, ask, close float64
		want                  float64
		wantSource            string
	}{
		{"last trade wins", 1500, 1400, 1600, 1300, 1500, priceSourceLast},
		{"mid when no trade", 0, 1400, 1600, 1300, 1500, priceSourceMid},
		{"close when no trade and no book", 0, 0, 0, 1300, 1300, priceSourceClose},
		{"close when only a bid", 0, 1400, 0, 1300, 1300, priceSourceClose},
		{"crossed book is ignored", 0, 1600, 1400, 1300, 1300, priceSourceClose},
		{"nothing usable", 0, 0, 0, 0, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, source := pickMarketPrice(tc.last, tc.bid, tc.ask, tc.close)
			if got != tc.want || source != tc.wantSource {
				t.Errorf("pickMarketPrice = (%g, %q), want (%g, %q)", got, source, tc.want, tc.wantSource)
			}
		})
	}
}

func TestPickSpot(t *testing.T) {
	if got := pickSpot(2450, 2400); got != 2450 {
		t.Errorf("last trade should win, got %g", got)
	}
	if got := pickSpot(0, 2400); got != 2400 {
		t.Errorf("close should be the fallback, got %g", got)
	}
	if got := pickSpot(0, 0); got != 0 {
		t.Errorf("no price should give 0, got %g", got)
	}
}

// collectionTime is a Monday-style trading morning in Tehran. The contract
// tests only depend on the calendar day.
var collectionTime = time.Date(2026, time.November, 23, 10, 0, 0, 0, tehranLocation)

func TestComputeMetricsCallInTheMoney(t *testing.T) {
	hv := 0.40
	in := metricInput{
		ContractID: 7,
		Kind:       options.Call,
		Strike:     18000,
		Spot:       20000,
		// The source gives expiry as a UTC midnight date; 30 days after the
		// collection day.
		Expiry: time.Date(2026, time.December, 23, 0, 0, 0, 0, time.UTC),
		Last:   2500,
		HV:     &hv,
	}
	m, err := computeMetrics(in, collectionTime, 0.30)
	if err != nil {
		t.Fatalf("computeMetrics: %v", err)
	}

	if m.DaysToExpiry != 30 {
		t.Errorf("DaysToExpiry = %d, want 30", m.DaysToExpiry)
	}
	if m.Moneyness != "ITM" {
		t.Errorf("Moneyness = %q, want ITM", m.Moneyness)
	}
	if m.IntrinsicValue != 2000 {
		t.Errorf("IntrinsicValue = %g, want 2000", m.IntrinsicValue)
	}
	if m.PriceSource == nil || *m.PriceSource != priceSourceLast {
		t.Errorf("PriceSource = %v, want %q", m.PriceSource, priceSourceLast)
	}
	near(t, "MarketPrice", m.MarketPrice, 2500, 1e-12)
	near(t, "TimeValue", m.TimeValue, 500, 1e-12)
	near(t, "BreakEven", m.BreakEven, 20500, 1e-12)
	near(t, "BreakEvenPercent", m.BreakEvenPercent, 2.5, 1e-12)
	near(t, "Leverage", m.Leverage, 8, 1e-12)
	near(t, "HistoricalVol", m.HistoricalVol, 0.40, 1e-12)

	// 2500 is above the discounted intrinsic value, so an implied volatility
	// exists, and with it Greeks, a theoretical price and a price gap.
	notNil(t, "ImpliedVol", m.ImpliedVol)
	notNil(t, "GreeksVol", m.GreeksVol)
	notNil(t, "Gamma", m.Gamma)
	notNil(t, "VegaPerPct", m.VegaPerPct)
	notNil(t, "ThetaPerDay", m.ThetaPerDay)
	notNil(t, "RhoPerPct", m.RhoPerPct)
	notNil(t, "EffectiveLeverage", m.EffectiveLeverage)
	notNil(t, "TheoreticalPrice", m.TheoreticalPrice)
	notNil(t, "PriceGap", m.PriceGap)
	notNil(t, "PriceGapPercent", m.PriceGapPercent)
	if m.Delta == nil || *m.Delta <= 0 || *m.Delta > 1 {
		t.Errorf("Delta = %v, want a value in (0, 1]", m.Delta)
	}
}

func TestComputeMetricsWithoutPrice(t *testing.T) {
	hv := 0.40
	in := metricInput{
		ContractID: 8,
		Kind:       options.Call,
		Strike:     22000,
		Spot:       20000,
		Expiry:     time.Date(2026, time.December, 23, 0, 0, 0, 0, time.UTC),
		HV:         &hv,
	}
	m, err := computeMetrics(in, collectionTime, 0.30)
	if err != nil {
		t.Fatalf("computeMetrics: %v", err)
	}

	if m.Moneyness != "OTM" || m.IntrinsicValue != 0 || m.DaysToExpiry != 30 {
		t.Errorf("price-independent fields wrong: %+v", m)
	}
	// Nothing that needs an option price may be filled in.
	if m.PriceSource != nil {
		t.Errorf("PriceSource = %q, want nil", *m.PriceSource)
	}
	isNil(t, "MarketPrice", m.MarketPrice)
	isNil(t, "TimeValue", m.TimeValue)
	isNil(t, "BreakEven", m.BreakEven)
	isNil(t, "BreakEvenPercent", m.BreakEvenPercent)
	isNil(t, "Leverage", m.Leverage)
	isNil(t, "EffectiveLeverage", m.EffectiveLeverage)
	isNil(t, "ImpliedVol", m.ImpliedVol)
	isNil(t, "PriceGap", m.PriceGap)
	isNil(t, "PriceGapPercent", m.PriceGapPercent)
	// HV20 alone is enough for Greeks and the theoretical price.
	notNil(t, "Delta", m.Delta)
	notNil(t, "TheoreticalPrice", m.TheoreticalPrice)
	near(t, "GreeksVol", m.GreeksVol, 0.40, 1e-12)
}

func TestComputeMetricsWithoutPriceOrVolatilityHistory(t *testing.T) {
	in := metricInput{
		ContractID: 9,
		Kind:       options.Put,
		Strike:     22000,
		Spot:       20000,
		Expiry:     time.Date(2026, time.December, 23, 0, 0, 0, 0, time.UTC),
	}
	m, err := computeMetrics(in, collectionTime, 0.30)
	if err != nil {
		t.Fatalf("computeMetrics: %v", err)
	}
	if m.Moneyness != "ITM" || m.IntrinsicValue != 2000 {
		t.Errorf("price-independent fields wrong: %+v", m)
	}
	// With no volatility at all the Greeks are unknown, not step functions.
	isNil(t, "Delta", m.Delta)
	isNil(t, "Gamma", m.Gamma)
	isNil(t, "GreeksVol", m.GreeksVol)
	isNil(t, "HistoricalVol", m.HistoricalVol)
	isNil(t, "TheoreticalPrice", m.TheoreticalPrice)
}

func TestComputeMetricsOnExpiryDay(t *testing.T) {
	in := metricInput{
		ContractID: 10,
		Kind:       options.Call,
		Strike:     18000,
		Spot:       20000,
		Expiry:     time.Date(2026, time.November, 23, 0, 0, 0, 0, time.UTC),
		Last:       2000,
	}
	m, err := computeMetrics(in, collectionTime, 0.30)
	if err != nil {
		t.Fatalf("computeMetrics: %v", err)
	}
	if m.DaysToExpiry != 0 {
		t.Errorf("DaysToExpiry = %d, want 0", m.DaysToExpiry)
	}
	isNil(t, "ImpliedVol", m.ImpliedVol)
	// No time is left, so the in-the-money call has delta 1 even without any
	// volatility estimate.
	near(t, "Delta", m.Delta, 1, 1e-12)
	near(t, "TimeValue", m.TimeValue, 0, 1e-12)
}

func TestComputeMetricsRejectsInvalidInput(t *testing.T) {
	in := metricInput{
		Kind:   options.Call,
		Strike: 18000,
		Spot:   0,
		Expiry: time.Date(2026, time.December, 23, 0, 0, 0, 0, time.UTC),
	}
	_, err := computeMetrics(in, collectionTime, 0.30)
	if !errors.Is(err, options.ErrInvalidInput) {
		t.Errorf("error = %v, want options.ErrInvalidInput", err)
	}
}

func TestMetricStatementMatchesColumns(t *testing.T) {
	args := metricArgs(&OptionMetrics{})
	if len(args) != len(metricColumns) {
		t.Fatalf("metricArgs returns %d values for %d columns", len(args), len(metricColumns))
	}
	last := "$" + strconv.Itoa(len(metricColumns))
	if !strings.Contains(upsertMetricsSQL, last) || strings.Contains(upsertMetricsSQL, "$"+strconv.Itoa(len(metricColumns)+1)) {
		t.Errorf("placeholders do not match %d columns:\n%s", len(metricColumns), upsertMetricsSQL)
	}
	if !strings.Contains(upsertMetricsSQL, "ON CONFLICT (contract_id, ts) DO UPDATE") {
		t.Errorf("statement is not an upsert:\n%s", upsertMetricsSQL)
	}
}
