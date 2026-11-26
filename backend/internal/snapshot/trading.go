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
//  1. If any contract carries a LastTradeTime (the TSETMC adapter fills it
//     from the market watch), we use the direct rule: trading happened iff some
//     LastTradeTime falls on today's Tehran date. As a guard against a source
//     that serves a stale date, a "yes" is overruled when every figure is
//     identical to the latest earlier snapshot.
//  2. If the source gave no trade times at all (its extra requests failed, or
//     it never provides them), compare with the latest earlier snapshot. On a
//     closed day the source keeps serving the last trading day's final
//     numbers, so if no contract's volume, trade count or prices differ, nothing
//     traded.
//  3. With no earlier snapshot (first ever run) we cannot compare. We then
//     skip Thursday and Friday (the Iranian weekend) and any day on which no
//     contract has volume.
func hasTradedToday(ctx context.Context, db querier, snap *datasource.MarketSnapshot, now time.Time) (bool, string, error) {
	today := now.Format(dateLayout)
	direct, hasTime := directRule(snap, today)
	if hasTime && !direct {
		return false, "no contract has a last trade time on " + today, nil
	}

	prev, err := loadPrevious(ctx, db, today)
	if err != nil {
		return false, "", err
	}
	switch {
	case hasTime: // direct rule says yes
		if prev != nil {
			if ok, reason, _ := decideAgainst(snap, prev); !ok {
				return false, "last trade time is today but " + reason + " (stale source?)", nil
			}
		}
		return true, "contract has a last trade time today", nil
	case prev == nil:
		return decideWithoutHistory(snap, now)
	default:
		return decideAgainst(snap, prev)
	}
}

// directRule reports whether any contract has a LastTradeTime on today
// (Tehran date, YYYY-MM-DD), and whether any contract has one at all.
func directRule(snap *datasource.MarketSnapshot, today string) (onToday, hasTime bool) {
	for _, c := range snap.Contracts {
		if c.LastTradeTime == nil {
			continue
		}
		hasTime = true
		if c.LastTradeTime.In(datasource.Tehran).Format(dateLayout) == today {
			return true, true
		}
	}
	return false, hasTime
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
