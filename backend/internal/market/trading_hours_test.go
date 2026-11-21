package market

import (
	"testing"
	"time"
)

func TestMarketOpenAt(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		open bool
	}{
		{"Saturday opening", time.Date(2026, time.November, 21, 9, 0, 0, 0, tehranLocation), true},
		{"UTC instant at Saturday opening", time.Date(2026, time.November, 21, 5, 30, 0, 0, time.UTC), true},
		{"Wednesday before close", time.Date(2026, time.November, 25, 12, 29, 59, 0, tehranLocation), true},
		{"before opening", time.Date(2026, time.November, 21, 8, 59, 59, 0, tehranLocation), false},
		{"at closing", time.Date(2026, time.November, 21, 12, 30, 0, 0, tehranLocation), false},
		{"Thursday", time.Date(2026, time.November, 26, 10, 0, 0, 0, tehranLocation), false},
		{"Friday", time.Date(2026, time.November, 27, 10, 0, 0, 0, tehranLocation), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MarketOpenAt(tt.time); got != tt.open {
				t.Errorf("MarketOpenAt(%s) = %t, want %t", tt.time, got, tt.open)
			}
		})
	}
}
