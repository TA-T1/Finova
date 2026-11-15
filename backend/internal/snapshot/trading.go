package snapshot

import (
	"context"
	"fmt"
	"time"

	"Finova/internal/datasource"
)

// tradedKey is what we compare between two days to see whether a contract
// traded: cumulative figures that change whenever anything trades.
type tradedKey struct{ volume, tradeCount, lastPrice, closePrice int64 }

func keyOf(c datasource.OptionContract) tradedKey {
	return tradedKey{c.Volume, c.TradeCount, toInt(c.LastPrice), toInt(c.ClosePrice)}
}

// previousDay is the latest stored snapshot before today.
type previousDay struct {
	date string
	keys map[string]tradedKey // contract symbol -> figures
}

// hasTradedToday decides whether the market traded on today's Tehran date, so
// that Thursdays, Fridays and holidays write nothing without a holiday
// calendar. It reports a short reason for the log.
//
//  1. If any contract carries a LastTradeTime, we use the direct rule: trading
//     happened iff some LastTradeTime falls on today's Tehran date.
//  2. TSETMC's option-chain endpoint gives no last trade time, so normally we
//     compare with the latest earlier snapshot. On a closed day the source
//     keeps serving the last trading day's final numbers, so if no contract's
//     volume, trade count or prices differ from that snapshot, nothing traded.
//  3. With no earlier snapshot (first ever run) we cannot compare. We then
//     skip Thursday and Friday (the Iranian weekend) and any day on which no
//     contract has volume.
func hasTradedToday(ctx context.Context, db querier, snap *datasource.MarketSnapshot, now time.Time) (bool, string, error) {
	today := now.Format(dateLayout)

	hasTime := false
	for _, c := range snap.Contracts {
		if c.LastTradeTime == nil {
			continue
		}
		hasTime = true
		if c.LastTradeTime.In(datasource.Tehran).Format(dateLayout) == today {
			return true, "contract has a last trade time today", nil
		}
	}
	if hasTime {
		return false, "no contract has a last trade time on " + today, nil
	}

	prev, err := loadPrevious(ctx, db, today)
	if err != nil {
		return false, "", err
	}
	if prev == nil {
		return decideWithoutHistory(snap, now)
	}
	return decideAgainst(snap, prev)
}

// decideAgainst implements rule 2.
func decideAgainst(snap *datasource.MarketSnapshot, prev *previousDay) (bool, string, error) {
	for _, c := range snap.Contracts {
		if c.Volume <= 0 && c.TradeCount <= 0 {
			continue // never traded: no information either way
		}
		if k, ok := prev.keys[c.Symbol]; !ok || k != keyOf(c) {
			return true, "figures changed since the snapshot of " + prev.date, nil
		}
	}
	return false, "figures identical to the snapshot of " + prev.date, nil
}

// decideWithoutHistory implements rule 3.
func decideWithoutHistory(snap *datasource.MarketSnapshot, now time.Time) (bool, string, error) {
	if wd := now.Weekday(); wd == time.Thursday || wd == time.Friday {
		return false, fmt.Sprintf("%s is a weekend day and there is no earlier snapshot to compare", wd), nil
	}
	for _, c := range snap.Contracts {
		if c.Volume > 0 {
			return true, "first snapshot, contracts have volume", nil
		}
	}
	return false, "first snapshot and no contract has volume", nil
}
