package options

import (
	"errors"
	"math"
	"testing"
	"time"
)

// closeTo compares floats with a relative tolerance, which is what matters
// when values range from a delta of 0.6 to a rial price of 3,791.
func closeTo(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	scale := math.Max(1, math.Abs(want))
	if math.Abs(got-want) > tol*scale {
		t.Errorf("%s = %.10g, want %.10g", label, got, want)
	}
}

// Reference values are from py_vollib-style Black-Scholes computed with
// scipy. They pin the engine to an independent implementation.
func TestPrice(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		vol  float64
		want float64
	}{
		{
			name: "atm call one year",
			in:   Inputs{Kind: Call, Spot: 100, Strike: 100, TimeToExpiry: 1, Rate: 0.05},
			vol:  0.2,
			want: 10.450583572185565,
		},
		{
			name: "atm put one year",
			in:   Inputs{Kind: Put, Spot: 100, Strike: 100, TimeToExpiry: 1, Rate: 0.05},
			vol:  0.2,
			want: 5.573526022256971,
		},
		{
			name: "itm call, tehran-scale rial prices",
			in:   Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30},
			vol:  0.45,
			want: 3791.671600068641,
		},
		{
			name: "otm put with dividend yield",
			in:   Inputs{Kind: Put, Spot: 95, Strike: 100, TimeToExpiry: 0.5, Rate: 0.04, DividendYield: 0.02},
			vol:  0.35,
			want: 11.571143415115046,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Price(tc.in, tc.vol)
			if err != nil {
				t.Fatalf("Price() error = %v", err)
			}
			closeTo(t, "price", got, tc.want, 1e-10)
		})
	}
}

func TestGreeks(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		vol  float64
		want Greeks
	}{
		{
			name: "atm call one year",
			in:   Inputs{Kind: Call, Spot: 100, Strike: 100, TimeToExpiry: 1, Rate: 0.05},
			vol:  0.2,
			want: Greeks{
				Delta: 0.6368306511756191,
				Gamma: 0.018762017345846895,
				Vega:  37.52403469169379,
				Theta: -6.414027546438197,
				Rho:   53.232481545376345,
			},
		},
		{
			name: "atm put one year",
			in:   Inputs{Kind: Put, Spot: 100, Strike: 100, TimeToExpiry: 1, Rate: 0.05},
			vol:  0.2,
			want: Greeks{
				Delta: -0.3631693488243809,
				Gamma: 0.018762017345846895,
				Vega:  37.52403469169379,
				Theta: -1.657880423934626,
				Rho:   -41.89046090469506,
			},
		},
		{
			name: "itm call, tehran-scale rial prices",
			in:   Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30},
			vol:  0.45,
			want: Greeks{
				Delta: 0.8196684508024988,
				Gamma: 5.8378510373133684e-05,
				Vega:  2627.032966791016,
				Theta: -6144.838894906314,
				Rho:   3150.4243539953336,
			},
		},
		{
			name: "otm put with dividend yield",
			in:   Inputs{Kind: Put, Spot: 95, Strike: 100, TimeToExpiry: 0.5, Rate: 0.04, DividendYield: 0.02},
			vol:  0.35,
			want: Greeks{
				Delta: -0.5120455058486251,
				Gamma: 0.016783671634776124,
				Vega:  26.507711388174545,
				Theta: -7.8419667881441,
				Rho:   -30.107733235367217,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CalcGreeks(tc.in, tc.vol)
			if err != nil {
				t.Fatalf("CalcGreeks() error = %v", err)
			}
			closeTo(t, "delta", got.Delta, tc.want.Delta, 1e-10)
			closeTo(t, "gamma", got.Gamma, tc.want.Gamma, 1e-10)
			closeTo(t, "vega", got.Vega, tc.want.Vega, 1e-10)
			closeTo(t, "theta", got.Theta, tc.want.Theta, 1e-10)
			closeTo(t, "rho", got.Rho, tc.want.Rho, 1e-10)
		})
	}
}

// Put-call parity holds for any correct European pricer, independently of the
// reference values above, so it catches sign and discounting mistakes.
func TestPutCallParity(t *testing.T) {
	base := Inputs{Spot: 18500, Strike: 20000, TimeToExpiry: 0.4, Rate: 0.28, DividendYield: 0.01}
	const vol = 0.55

	call := base
	call.Kind = Call
	put := base
	put.Kind = Put

	c, err := Price(call, vol)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Price(put, vol)
	if err != nil {
		t.Fatal(err)
	}

	lhs := c - p
	rhs := base.Spot*math.Exp(-base.DividendYield*base.TimeToExpiry) -
		base.Strike*math.Exp(-base.Rate*base.TimeToExpiry)
	closeTo(t, "call minus put", lhs, rhs, 1e-10)

	// Delta parity is the derivative of the same identity.
	gc, _ := CalcGreeks(call, vol)
	gp, _ := CalcGreeks(put, vol)
	closeTo(t, "delta parity", gc.Delta-gp.Delta,
		math.Exp(-base.DividendYield*base.TimeToExpiry), 1e-10)
}

// Vega and gamma are the same for a call and a put on the same contract.
func TestVegaGammaMatchAcrossKinds(t *testing.T) {
	base := Inputs{Spot: 12000, Strike: 13000, TimeToExpiry: 0.2, Rate: 0.25}
	call, put := base, base
	call.Kind, put.Kind = Call, Put

	gc, _ := CalcGreeks(call, 0.6)
	gp, _ := CalcGreeks(put, 0.6)
	closeTo(t, "vega", gc.Vega, gp.Vega, 1e-12)
	closeTo(t, "gamma", gc.Gamma, gp.Gamma, 1e-12)
}

// The analytic Greeks must agree with a numerical derivative of the price.
func TestGreeksAgainstFiniteDifference(t *testing.T) {
	in := Inputs{Kind: Call, Spot: 15000, Strike: 16000, TimeToExpiry: 0.3, Rate: 0.25}
	const vol = 0.5

	g, _ := CalcGreeks(in, vol)

	bump := func(mutate func(*Inputs, float64), h float64) (up, down float64) {
		u, d := in, in
		mutate(&u, h)
		mutate(&d, -h)
		pu, _ := Price(u, vol)
		pd, _ := Price(d, vol)
		return pu, pd
	}

	const hS = 0.01
	up, down := bump(func(i *Inputs, h float64) { i.Spot += h }, hS)
	base, _ := Price(in, vol)
	closeTo(t, "delta", (up-down)/(2*hS), g.Delta, 1e-6)
	closeTo(t, "gamma", (up-2*base+down)/(hS*hS), g.Gamma, 1e-4)

	const hVol = 1e-5
	pu, _ := Price(in, vol+hVol)
	pd, _ := Price(in, vol-hVol)
	closeTo(t, "vega", (pu-pd)/(2*hVol), g.Vega, 1e-6)

	// Theta is the derivative with respect to time passing, so the sign is
	// the negative of the derivative with respect to time to expiry.
	const hT = 1e-6
	up, down = bump(func(i *Inputs, h float64) { i.TimeToExpiry += h }, hT)
	closeTo(t, "theta", -(up-down)/(2*hT), g.Theta, 1e-5)

	const hR = 1e-7
	up, down = bump(func(i *Inputs, h float64) { i.Rate += h }, hR)
	closeTo(t, "rho", (up-down)/(2*hR), g.Rho, 1e-5)
}

// Round-tripping price to implied volatility and back is the real test of the
// solver: it must recover the volatility it was given, including in the deep
// wings where vega is almost zero and naive Newton diverges.
func TestImpliedVolatilityRoundTrip(t *testing.T) {
	kinds := []Kind{Call, Put}
	strikes := []float64{8000, 12000, 20000, 28000, 40000}
	expiries := []float64{1.0 / 365, 7.0 / 365, 0.25, 1.0}
	vols := []float64{0.05, 0.3, 0.8, 2.0}

	for _, kind := range kinds {
		for _, strike := range strikes {
			for _, expiry := range expiries {
				for _, vol := range vols {
					in := Inputs{
						Kind: kind, Spot: 20000, Strike: strike,
						TimeToExpiry: expiry, Rate: 0.30,
					}
					p, err := Price(in, vol)
					if err != nil {
						t.Fatal(err)
					}
					if p <= 0 {
						continue // value rounds to zero; nothing to invert
					}

					got, err := ImpliedVolatility(in, p)
					if errors.Is(err, ErrPriceOutOfRange) {
						continue // at the bounds within floating-point noise
					}
					if err != nil {
						t.Fatalf("kind=%v K=%g T=%g vol=%g: %v", kind, strike, expiry, vol, err)
					}
					// Compare prices rather than volatilities: in the deep
					// wings many volatilities give the same price to machine
					// precision, and price is what the user sees.
					back, _ := Price(in, got)
					closeTo(t, "reprice", back, p, 1e-6)
				}
			}
		}
	}
}

func TestImpliedVolatilityErrors(t *testing.T) {
	in := Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30}

	t.Run("above upper bound", func(t *testing.T) {
		if _, err := ImpliedVolatility(in, in.Spot*2); !errors.Is(err, ErrPriceOutOfRange) {
			t.Errorf("err = %v, want ErrPriceOutOfRange", err)
		}
	})

	t.Run("below intrinsic returns zero volatility", func(t *testing.T) {
		low, _, _ := PriceBounds(in)
		got, err := ImpliedVolatility(in, low/2)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if got != 0 {
			t.Errorf("vol = %g, want 0", got)
		}
	})

	t.Run("expired contract", func(t *testing.T) {
		expired := in
		expired.TimeToExpiry = 0
		if _, err := ImpliedVolatility(expired, 2000); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("non-positive market price", func(t *testing.T) {
		if _, err := ImpliedVolatility(in, 0); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func TestDegenerateCases(t *testing.T) {
	t.Run("expiry day pays intrinsic", func(t *testing.T) {
		in := Inputs{Kind: Call, Spot: 21000, Strike: 20000, TimeToExpiry: 0, Rate: 0.3}
		got, err := Price(in, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		closeTo(t, "price", got, 1000, 1e-12)

		g, _ := CalcGreeks(in, 0.5)
		if g.Delta != 1 {
			t.Errorf("delta = %g, want 1", g.Delta)
		}
		if g.Gamma != 0 || g.Vega != 0 || g.Theta != 0 || g.Rho != 0 {
			t.Errorf("non-delta Greeks should vanish at expiry, got %+v", g)
		}
	})

	t.Run("zero volatility pays discounted intrinsic", func(t *testing.T) {
		in := Inputs{Kind: Put, Spot: 15000, Strike: 20000, TimeToExpiry: 0.5, Rate: 0.3}
		got, err := Price(in, 0)
		if err != nil {
			t.Fatal(err)
		}
		want := in.Strike*math.Exp(-in.Rate*in.TimeToExpiry) - in.Spot
		closeTo(t, "price", got, want, 1e-12)
	})

	t.Run("zero volatility floors at zero when the forward is past the strike", func(t *testing.T) {
		// Carry pushes the forward above the strike, so this put expires
		// worthless rather than being worth a negative amount.
		in := Inputs{Kind: Put, Spot: 19000, Strike: 20000, TimeToExpiry: 0.5, Rate: 0.3}
		got, err := Price(in, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Errorf("price = %g, want 0", got)
		}
	})
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		vol  float64
	}{
		{"unknown kind", Inputs{Spot: 100, Strike: 100, TimeToExpiry: 1}, 0.2},
		{"zero spot", Inputs{Kind: Call, Spot: 0, Strike: 100, TimeToExpiry: 1}, 0.2},
		{"negative strike", Inputs{Kind: Call, Spot: 100, Strike: -1, TimeToExpiry: 1}, 0.2},
		{"negative expiry", Inputs{Kind: Call, Spot: 100, Strike: 100, TimeToExpiry: -1}, 0.2},
		{"negative volatility", Inputs{Kind: Call, Spot: 100, Strike: 100, TimeToExpiry: 1}, -0.2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Price(tc.in, tc.vol); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("Price() err = %v, want ErrInvalidInput", err)
			}
			if _, err := CalcGreeks(tc.in, tc.vol); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("CalcGreeks() err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestMetrics(t *testing.T) {
	in := Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30}
	const marketPrice, delta = 3800, 0.82

	m, err := CalcMetrics(in, marketPrice, delta)
	if err != nil {
		t.Fatal(err)
	}

	closeTo(t, "intrinsic", m.IntrinsicValue, 2000, 1e-12)
	closeTo(t, "time value", m.TimeValue, 1800, 1e-12)
	closeTo(t, "break-even", m.BreakEven, 21800, 1e-12)
	closeTo(t, "break-even percent", m.BreakEvenPercent, 9, 1e-12)
	closeTo(t, "leverage", m.Leverage, 20000.0/3800, 1e-12)
	closeTo(t, "effective leverage", m.EffectiveLeverage, 0.82*20000/3800, 1e-12)
	if m.Moneyness != InTheMoney {
		t.Errorf("moneyness = %v, want ITM", m.Moneyness)
	}
}

func TestMoneyness(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		want Moneyness
	}{
		{"call below spot", Inputs{Kind: Call, Spot: 20000, Strike: 18000}, InTheMoney},
		{"call above spot", Inputs{Kind: Call, Spot: 20000, Strike: 22000}, OutOfTheMoney},
		{"put above spot", Inputs{Kind: Put, Spot: 20000, Strike: 22000}, InTheMoney},
		{"put below spot", Inputs{Kind: Put, Spot: 20000, Strike: 18000}, OutOfTheMoney},
		{"call at spot", Inputs{Kind: Call, Spot: 20000, Strike: 20000}, AtTheMoney},
		{"put within band", Inputs{Kind: Put, Spot: 20000, Strike: 20050}, AtTheMoney},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyMoneyness(tc.in); got != tc.want {
				t.Errorf("moneyness = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	t.Run("full row", func(t *testing.T) {
		in := Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30}
		marketPrice, _ := Price(in, 0.45) // a quote consistent with 45% vol

		ev, err := Evaluate(Quote{Inputs: in, MarketPrice: marketPrice, HistoricalVolatility: 0.40})
		if err != nil {
			t.Fatal(err)
		}
		if !ev.HasImpliedVolatility {
			t.Fatal("expected an implied volatility")
		}
		closeTo(t, "implied volatility", ev.ImpliedVolatility, 0.45, 1e-8)
		closeTo(t, "greeks volatility", ev.GreeksVolatility, 0.45, 1e-8)

		if !ev.HasTheoreticalPrice {
			t.Fatal("expected a theoretical price")
		}
		// The market implies more volatility than history, so the market
		// price must sit above the theoretical one.
		if ev.PriceGap <= 0 {
			t.Errorf("price gap = %g, want positive", ev.PriceGap)
		}
		closeTo(t, "intrinsic", ev.Metrics.IntrinsicValue, 2000, 1e-12)
	})

	t.Run("bad quote falls back to historical volatility", func(t *testing.T) {
		in := Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30}

		ev, err := Evaluate(Quote{
			Inputs:               in,
			MarketPrice:          in.Spot * 3, // impossible: above the upper bound
			HistoricalVolatility: 0.40,
		})
		if err != nil {
			t.Fatalf("a bad quote must not fail the row: %v", err)
		}
		if ev.HasImpliedVolatility {
			t.Error("implied volatility should be unavailable")
		}
		closeTo(t, "greeks volatility", ev.GreeksVolatility, 0.40, 1e-12)
		if ev.Greeks.Delta == 0 {
			t.Error("delta should still be computed from historical volatility")
		}
	})

	t.Run("invalid input still errors", func(t *testing.T) {
		if _, err := Evaluate(Quote{Inputs: Inputs{Kind: Call, Spot: -1, Strike: 100}}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func TestYearsToExpiry(t *testing.T) {
	now := time.Date(2026, 11, 7, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		name   string
		expiry time.Time
		want   float64
	}{
		{"same day", now, 0},
		{"past", now.AddDate(0, 0, -5), 0},
		{"one day", now.AddDate(0, 0, 1), 1.0 / 365},
		{"ninety days", now.AddDate(0, 0, 90), 90.0 / 365},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			closeTo(t, "years", YearsToExpiry(now, tc.expiry), tc.want, 1e-12)
		})
	}
}

func BenchmarkEvaluate(b *testing.B) {
	in := Inputs{Kind: Call, Spot: 20000, Strike: 18000, TimeToExpiry: 0.25, Rate: 0.30}
	marketPrice, _ := Price(in, 0.45)
	q := Quote{Inputs: in, MarketPrice: marketPrice, HistoricalVolatility: 0.40}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Evaluate(q); err != nil {
			b.Fatal(err)
		}
	}
}
