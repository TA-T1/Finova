package market

import (
	"errors"
	"time"
	_ "time/tzdata"
)

const tehranTimezone = "Asia/Tehran"

var (
	ErrMarketClosed = errors.New("market is closed")
	tehranLocation  = mustLoadTehranLocation()
)

func mustLoadTehranLocation() *time.Location {
	location, err := time.LoadLocation(tehranTimezone)
	if err != nil {
		panic(err)
	}
	return location
}

// MarketOpenAt reports whether the Tehran market is scheduled to be open.
// The schedule is Saturday through Wednesday, 09:00 inclusive to 12:30 exclusive.
func MarketOpenAt(t time.Time) bool {
	local := t.In(tehranLocation)
	switch local.Weekday() {
	case time.Saturday, time.Sunday, time.Monday, time.Tuesday, time.Wednesday:
	default:
		return false
	}
	open := time.Date(local.Year(), local.Month(), local.Day(), 9, 0, 0, 0, tehranLocation)
	close := time.Date(local.Year(), local.Month(), local.Day(), 12, 30, 0, 0, tehranLocation)
	return !local.Before(open) && local.Before(close)
}
