package backfill

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"Finova/internal/datasource"
	"Finova/internal/snapshot"
)

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, datasource.Tehran)
	if err != nil {
		panic(err)
	}
	return t
}

type fakeSrc struct {
	chain []datasource.OptionRow
	hist  map[string][]datasource.DailyPrice
	fail  map[string]bool
	calls []string
}

func (f *fakeSrc) OptionMarketWatch(context.Context) ([]datasource.OptionRow, error) {
	return f.chain, nil
}

func (f *fakeSrc) DailyPrices(_ context.Context, ins string, _ bool) ([]datasource.DailyPrice, error) {
	f.calls = append(f.calls, ins)
	if f.fail[ins] {
		return nil, errors.New("boom")
	}
	return f.hist[ins], nil
}

func newFake() *fakeSrc {
	row := datasource.OptionRow{
		UnderlyingInsCode: "U1", UnderlyingSymbol: "خودرو",
		StrikePrice: 2000, ExpiryDate: day("2026-12-23"), ContractSize: 1000,
		Call: &datasource.OptionLeg{InsCode: "C1", Symbol: "ضخود1", Type: datasource.Call},
		Put:  &datasource.OptionLeg{InsCode: "P1", Symbol: "طخود1", Type: datasource.Put},
	}
	return &fakeSrc{
		chain: []datasource.OptionRow{row},
		hist: map[string][]datasource.DailyPrice{
			"U1": {
				{Date: day("2026-11-10"), Close: 2400, Final: 2390},
				{Date: day("2026-11-11"), Close: 0, Final: 2410}, // falls back to Final
				{Date: day("2026-11-12"), Close: 2450, Final: 2440},
			},
			"C1": {
				{Date: day("2026-11-10"), Close: 500, Final: 495, Volume: 100, Value: 5000, TradeCount: 7},
				{Date: day("2026-11-11"), Volume: 0}, // no trade: dropped
				{Date: day("2026-11-12"), Close: 520, Final: 510, Volume: 50},
				{Date: day("2026-11-24"), Close: 530, Final: 525, Volume: 10}, // today: dropped
			},
			"P1": {{Date: day("2026-11-10"), Close: 30, Final: 31, Volume: 5}},
		},
		fail: map[string]bool{},
	}
}

type memWriter struct{ rows []snapshot.Row }

func (m *memWriter) write(_ context.Context, rows []snapshot.Row) (int, error) {
	m.rows = append(m.rows, rows...)
	return len(rows), nil
}

func opts() []Option {
	return []Option{
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		WithClock(func() time.Time { return day("2026-11-24").Add(15 * time.Hour) }),
	}
}

func TestRunBuildsRowsFromHistory(t *testing.T) {
	src, w := newFake(), &memWriter{}
	res, err := Run(context.Background(), src, w.write, opts()...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Contracts != 2 || res.Fetched != 2 || res.Failed != 0 {
		t.Fatalf("result = %+v", res)
	}
	if len(w.rows) != 3 || res.Rows != 3 || res.Inserted != 3 {
		t.Fatalf("rows = %d, result = %+v", len(w.rows), res)
	}
	if res.FirstDate != "2026-11-10" || res.LastDate != "2026-11-12" {
		t.Errorf("range = %s..%s", res.FirstDate, res.LastDate)
	}
	var first snapshot.Row
	for _, r := range w.rows {
		if r.ContractSymbol == "ضخود1" && r.TradeDate == "2026-11-10" {
			first = r
		}
	}
	if first.LastPrice != 500 || first.ClosePrice != 495 || first.Volume != 100 || first.Value != 5000 || first.TradeCount != 7 {
		t.Errorf("row = %+v", first)
	}
	if first.BidPrice != 0 || first.AskPrice != 0 || first.OpenInterest != 0 {
		t.Errorf("bid/ask/open interest must stay 0: %+v", first)
	}
	if first.OptionType != "call" || first.Strike != 2000 || first.Expiry != "2026-12-23" || first.UnderlyingSymbol != "خودرو" {
		t.Errorf("identity = %+v", first)
	}
	if first.UnderlyingPrice == nil || *first.UnderlyingPrice != 2400 {
		t.Errorf("underlying price = %v, want 2400", first.UnderlyingPrice)
	}
}

func TestRunNeverWritesTodayOrAfterExpiry(t *testing.T) {
	src, w := newFake(), &memWriter{}
	src.chain[0].ExpiryDate = day("2026-11-11") // 11-12 is after the expiry
	if _, err := Run(context.Background(), src, w.write, opts()...); err != nil {
		t.Fatal(err)
	}
	for _, r := range w.rows {
		if r.TradeDate >= "2026-11-24" || r.TradeDate > "2026-11-11" {
			t.Errorf("unexpected row on %s", r.TradeDate)
		}
	}
}

func TestRunUnderlyingFallsBackToFinalPrice(t *testing.T) {
	src := newFake()
	src.hist["C1"] = []datasource.DailyPrice{{Date: day("2026-11-11"), Close: 100, Final: 100, Volume: 1}}
	w := &memWriter{}
	if _, err := Run(context.Background(), src, w.write, opts()...); err != nil {
		t.Fatal(err)
	}
	var got *int64
	for _, r := range w.rows {
		if r.ContractSymbol == "ضخود1" {
			got = r.UnderlyingPrice
		}
	}
	if got == nil || *got != 2410 {
		t.Errorf("underlying price = %v, want 2410", got)
	}
}

func TestRunMissingUnderlyingHistoryGivesNullPrice(t *testing.T) {
	src, w := newFake(), &memWriter{}
	src.fail["U1"] = true
	res, err := Run(context.Background(), src, w.write, opts()...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.FirstErrMsg == "" {
		t.Errorf("result = %+v", res)
	}
	if len(w.rows) != 3 {
		t.Fatalf("rows = %d", len(w.rows))
	}
	for _, r := range w.rows {
		if r.UnderlyingPrice != nil {
			t.Errorf("want NULL underlying price, got %d", *r.UnderlyingPrice)
		}
	}
}

func TestRunContractFailureIsCountedAndSkipped(t *testing.T) {
	src, w := newFake(), &memWriter{}
	src.fail["P1"] = true
	res, err := Run(context.Background(), src, w.write, opts()...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.Fetched != 1 || len(w.rows) != 2 {
		t.Errorf("result = %+v rows = %d", res, len(w.rows))
	}
}

func TestRunLimitAndUnderlyingFetchedOnce(t *testing.T) {
	src, w := newFake(), &memWriter{}
	res, err := Run(context.Background(), src, w.write, append(opts(), WithLimit(1))...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Contracts != 2 || res.Fetched != 1 {
		t.Errorf("result = %+v", res)
	}
	n := 0
	for _, c := range src.calls {
		if c == "U1" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("underlying history fetched %d times, want 1", n)
	}
}

func TestRunEmptyChainIsAnError(t *testing.T) {
	src := newFake()
	src.chain = nil
	if _, err := Run(context.Background(), src, (&memWriter{}).write, opts()...); err == nil {
		t.Fatal("want an error")
	}
}
