package snapshot

import (
	"testing"
	"time"

	"Finova/internal/datasource"
)

const fixture = "../../testdata/market_sample.json"

// loadFixture returns the saved sample snapshot (synthetic contracts, real
// underlyings) as an editable value.
func loadFixture(t *testing.T) *datasource.MarketSnapshot {
	t.Helper()
	f, err := datasource.NewFake(fixture)
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.FetchMarket(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The sample's contracts have LastTradeTime 2026-11-07 12:25 Tehran (a Saturday).
var tradingDay = time.Date(2026, 11, 7, 13, 0, 0, 0, datasource.Tehran)

func TestDirectRuleUsesLastTradeTime(t *testing.T) {
	snap := loadFixture(t)
	ok, reason, err := hasTradedToday(t.Context(), nil, snap, tradingDay)
	if err != nil || !ok {
		t.Fatalf("trades today: ok=%v reason=%q err=%v", ok, reason, err)
	}
	// Next day: the same data has no trade time on that date.
	ok, _, err = hasTradedToday(t.Context(), nil, snap, tradingDay.AddDate(0, 0, 1))
	if err != nil || ok {
		t.Fatalf("no trades on the next day: ok=%v err=%v", ok, err)
	}
}

func TestDirectRuleUsesTehranDateNotUTC(t *testing.T) {
	snap := loadFixture(t)
	late := time.Date(2026, 11, 7, 23, 50, 0, 0, datasource.Tehran) // 20:20 UTC, still Nov 7 both places
	for i := range snap.Contracts {
		tt := time.Date(2026, 11, 7, 23, 40, 0, 0, datasource.Tehran)
		snap.Contracts[i].LastTradeTime = &tt
	}
	if ok, _, _ := hasTradedToday(t.Context(), nil, snap, late); !ok {
		t.Error("23:40 Tehran trade should count for Nov 7 Tehran")
	}
	// 00:10 Tehran on Nov 8 is Nov 7 UTC: it must count for Nov 8, not Nov 7.
	for i := range snap.Contracts {
		tt := time.Date(2026, 11, 8, 0, 10, 0, 0, datasource.Tehran)
		snap.Contracts[i].LastTradeTime = &tt
	}
	if ok, _, _ := hasTradedToday(t.Context(), nil, snap, tradingDay); ok {
		t.Error("a trade at 00:10 Tehran on Nov 8 must not count for Nov 7")
	}
}

func withoutTimes(s *datasource.MarketSnapshot) {
	for i := range s.Contracts {
		s.Contracts[i].LastTradeTime = nil
	}
}

func TestDecideAgainstPreviousSnapshot(t *testing.T) {
	snap := loadFixture(t)
	withoutTimes(snap)
	prev := &previousDay{date: "2026-11-04", keys: map[string]tradedKey{}}
	for _, c := range snap.Contracts {
		prev.keys[c.Symbol] = keyOf(c)
	}

	if ok, reason, _ := decideAgainst(snap, prev); ok {
		t.Errorf("identical to previous day must mean no trading, got ok (%s)", reason)
	}

	changed := loadFixture(t)
	withoutTimes(changed)
	changed.Contracts[5].Volume++
	if ok, _, _ := decideAgainst(changed, prev); !ok {
		t.Error("a changed volume must mean trading")
	}

	moved := loadFixture(t)
	withoutTimes(moved)
	moved.Contracts[7].LastPrice += 10
	if ok, _, _ := decideAgainst(moved, prev); !ok {
		t.Error("a changed last price must mean trading")
	}

	listed := loadFixture(t)
	withoutTimes(listed)
	extra := listed.Contracts[0]
	extra.Symbol = "ضنو9999" // a contract the previous day did not have
	listed.Contracts = append(listed.Contracts, extra)
	if ok, _, _ := decideAgainst(listed, prev); !ok {
		t.Error("a newly seen traded contract must mean trading")
	}
}

func TestDecideWithoutHistory(t *testing.T) {
	snap := loadFixture(t)
	withoutTimes(snap)
	cases := []struct {
		day  time.Weekday
		date time.Time
		want bool
	}{
		{time.Saturday, tehran(2026, 11, 14, 13, 0, 0), true},
		{time.Wednesday, tehran(2026, 11, 11, 13, 0, 0), true},
		{time.Thursday, tehran(2026, 11, 12, 13, 0, 0), false},
		{time.Friday, tehran(2026, 11, 13, 13, 0, 0), false},
	}
	for _, c := range cases {
		if c.date.Weekday() != c.day {
			t.Fatalf("test date %v is not a %v", c.date, c.day)
		}
		if ok, _, _ := decideWithoutHistory(snap, c.date); ok != c.want {
			t.Errorf("%v: ok=%v, want %v", c.day, ok, c.want)
		}
	}
	for i := range snap.Contracts {
		snap.Contracts[i].Volume = 0
	}
	if ok, _, _ := decideWithoutHistory(snap, tehran(2026, 11, 14, 13, 0, 0)); ok {
		t.Error("a day without any volume is not a trading day")
	}
}

func TestBuildRowsJoinsUnderlyingPrice(t *testing.T) {
	snap := loadFixture(t)
	rows, st := buildRows(snap, "2026-11-07", snap.FetchedAt)
	if len(rows) != len(snap.Contracts) || st.noUnderlying != 0 || st.duplicates != 0 {
		t.Fatalf("rows=%d stats=%+v", len(rows), st)
	}
	price := map[string]float64{}
	for _, u := range snap.Underlyings {
		price[u.Symbol] = u.LastPrice
	}
	for _, r := range rows {
		want, ok := price[r.UnderlyingSymbol]
		if !ok || r.UnderlyingPrice == nil || float64(*r.UnderlyingPrice) != want {
			t.Fatalf("%s: underlying_price=%v, want %v", r.ContractSymbol, r.UnderlyingPrice, want)
		}
	}
	r := rows[0]
	c := snap.Contracts[0]
	if r.ContractSymbol != c.Symbol || r.OptionType != c.Type || r.Strike != int64(c.Strike) ||
		r.Expiry != "2026-12-20" || r.TradeDate != "2026-11-07" || r.Volume != c.Volume {
		t.Errorf("row not mapped: %+v from %+v", r, c)
	}
}

func TestBuildRowsMissingOrZeroUnderlyingIsNull(t *testing.T) {
	snap := loadFixture(t)
	snap.Contracts[0].UnderlyingSymbol = "ناشناس"
	for i := range snap.Underlyings {
		if snap.Underlyings[i].Symbol == snap.Contracts[1].UnderlyingSymbol {
			snap.Underlyings[i].LastPrice = 0
		}
	}
	rows, st := buildRows(snap, "2026-11-07", snap.FetchedAt)
	if rows[0].UnderlyingPrice != nil || rows[1].UnderlyingPrice != nil {
		t.Error("unknown or zero underlying price must be NULL")
	}
	if st.noUnderlying < 2 || len(st.examples) == 0 {
		t.Errorf("stats = %+v", st)
	}
}

func TestBuildRowsCollapsesDuplicateSymbols(t *testing.T) {
	snap := loadFixture(t)
	dup := snap.Contracts[0]
	dup.LastPrice = 12345
	snap.Contracts = append(snap.Contracts, dup)
	rows, st := buildRows(snap, "2026-11-07", snap.FetchedAt)
	if len(rows) != len(snap.Contracts)-1 || st.duplicates != 1 || rows[0].LastPrice != 12345 {
		t.Errorf("rows=%d stats=%+v first.last=%d", len(rows), st, rows[0].LastPrice)
	}
}
