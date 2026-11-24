package snapshot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"Finova/internal/datasource"
)

// These tests need a real PostgreSQL. Set TEST_DATABASE_URL to run them, e.g.
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost:5432/finova_test go test ./internal/snapshot/
//
// Each test gets its own throw-away schema, so nothing real is touched.

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("snap_test_%d", rand.Int63())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	ddl, err := os.ReadFile("../../migrations/000008_quote_snapshots.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return db
}

func quiet() Option          { return WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))) }
func at(tm time.Time) Option { return WithClock(func() time.Time { return tm }) }

func fakeOf(t *testing.T, s *datasource.MarketSnapshot) *datasource.FakeSource {
	t.Helper()
	f, err := datasource.NewFakeFromSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func count(t *testing.T, db *pgxpool.Pool, where string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM quote_snapshots "+where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunWritesOneRowPerContractOnTradingDay(t *testing.T) {
	db := testDB(t)
	snap := loadFixture(t)
	n, err := Run(t.Context(), fakeOf(t, snap), db, quiet(), at(tradingDay))
	if err != nil || n != len(snap.Contracts) {
		t.Fatalf("n=%d err=%v, want %d rows", n, err, len(snap.Contracts))
	}
	if got := count(t, db, "WHERE trade_date = '2026-11-07'"); got != len(snap.Contracts) {
		t.Fatalf("stored %d rows, want %d", got, len(snap.Contracts))
	}

	// Spot-check one row end to end.
	c := snap.Contracts[0]
	var typ, under string
	var strike, last, bid, ask, vol, oi int64
	var expiry string
	var uprice *int64
	err = db.QueryRow(t.Context(), `SELECT option_type, underlying_symbol, strike, expiry::text, last_price,
		bid_price, ask_price, volume, open_interest, underlying_price
		FROM quote_snapshots WHERE trade_date='2026-11-07' AND contract_symbol=$1`, c.Symbol).
		Scan(&typ, &under, &strike, &expiry, &last, &bid, &ask, &vol, &oi, &uprice)
	if err != nil {
		t.Fatal(err)
	}
	if typ != c.Type || under != c.UnderlyingSymbol || strike != int64(c.Strike) || expiry != "2026-12-20" ||
		last != int64(c.LastPrice) || bid != int64(c.BidPrice) || ask != int64(c.AskPrice) ||
		vol != c.Volume || oi != c.OpenInterest || uprice == nil {
		t.Errorf("stored row differs: %s %s %d %s %d %d %d %d %d %v", typ, under, strike, expiry, last, bid, ask, vol, oi, uprice)
	}
}

func TestRunStoresCorrectUnderlyingPrice(t *testing.T) {
	db := testDB(t)
	snap := loadFixture(t)
	if _, err := Run(t.Context(), fakeOf(t, snap), db, quiet(), at(tradingDay)); err != nil {
		t.Fatal(err)
	}
	for _, u := range snap.Underlyings {
		var bad int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM quote_snapshots
			WHERE underlying_symbol=$1 AND underlying_price IS DISTINCT FROM $2`, u.Symbol, int64(u.LastPrice)).Scan(&bad)
		if err != nil || bad != 0 {
			t.Fatalf("%s: %d rows with a wrong underlying_price (err %v)", u.Symbol, bad, err)
		}
	}
}

func TestRunWritesNothingOnNonTradingDay(t *testing.T) {
	db := testDB(t)
	snap := loadFixture(t) // trades happened on Nov 7
	nextDay := tradingDay.AddDate(0, 0, 1)
	n, err := Run(t.Context(), fakeOf(t, snap), db, quiet(), at(nextDay))
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v, want 0 rows and no error", n, err)
	}
	if got := count(t, db, ""); got != 0 {
		t.Fatalf("%d rows were written on a non-trading day", got)
	}
}

func TestRunIsIdempotentAndSecondRunWins(t *testing.T) {
	db := testDB(t)
	snap := loadFixture(t)
	if _, err := Run(t.Context(), fakeOf(t, snap), db, quiet(), at(tradingDay)); err != nil {
		t.Fatal(err)
	}
	first := count(t, db, "")

	snap2 := loadFixture(t)
	sym := snap2.Contracts[3].Symbol
	snap2.Contracts[3].LastPrice = 7777
	snap2.Contracts[3].Volume = 424242
	n, err := Run(t.Context(), fakeOf(t, snap2), db, quiet(), at(tradingDay.Add(30*time.Minute)))
	if err != nil || n != len(snap2.Contracts) {
		t.Fatalf("second run n=%d err=%v", n, err)
	}
	if got := count(t, db, ""); got != first {
		t.Fatalf("rows after second run = %d, want %d (no duplicates)", got, first)
	}
	var last, vol int64
	if err := db.QueryRow(t.Context(), `SELECT last_price, volume FROM quote_snapshots
		WHERE trade_date='2026-11-07' AND contract_symbol=$1`, sym).Scan(&last, &vol); err != nil {
		t.Fatal(err)
	}
	if last != 7777 || vol != 424242 {
		t.Errorf("row keeps first run's values: last=%d volume=%d", last, vol)
	}
}

// TSETMC gives no last trade time, so closed days are detected by comparing
// with the previous snapshot.
func TestRunDetectsClosedDayByComparingWithPreviousSnapshot(t *testing.T) {
	db := testDB(t)
	withNoTimes := func() *datasource.MarketSnapshot { s := loadFixture(t); withoutTimes(s); return s }

	sat := tehran(2026, 11, 14, 13, 0, 0) // Saturday
	if n, err := Run(t.Context(), fakeOf(t, withNoTimes()), db, quiet(), at(sat)); err != nil || n == 0 {
		t.Fatalf("first snapshot: n=%d err=%v", n, err)
	}
	// Thursday: the source repeats the last trading day's numbers.
	thu := sat.AddDate(0, 0, 5)
	if n, err := Run(t.Context(), fakeOf(t, withNoTimes()), db, quiet(), at(thu)); err != nil || n != 0 {
		t.Fatalf("holiday: n=%d err=%v, want 0", n, err)
	}
	// Saturday again, with new trades.
	next := withNoTimes()
	next.Contracts[2].Volume += 1000
	next.Contracts[2].TradeCount += 10
	sat2 := sat.AddDate(0, 0, 7)
	if n, err := Run(t.Context(), fakeOf(t, next), db, quiet(), at(sat2)); err != nil || n == 0 {
		t.Fatalf("next trading day: n=%d err=%v", n, err)
	}
	if got := count(t, db, "WHERE trade_date = '2026-11-19'"); got != 0 {
		t.Errorf("rows were stored for the closed Thursday: %d", got)
	}
	if got := count(t, db, "WHERE trade_date = '2026-11-21'"); got != len(next.Contracts) {
		t.Errorf("rows for the new trading day = %d, want %d", got, len(next.Contracts))
	}
}

func TestRunFirstSnapshotOnFridayIsSkipped(t *testing.T) {
	db := testDB(t)
	s := loadFixture(t)
	withoutTimes(s)
	if n, err := Run(t.Context(), fakeOf(t, s), db, quiet(), at(tehran(2026, 11, 13, 13, 0, 0))); err != nil || n != 0 {
		t.Fatalf("Friday with no history: n=%d err=%v", n, err)
	}
}

func TestRunReturnsFetchErrorAndWritesNothing(t *testing.T) {
	db := testDB(t)
	f := fakeOf(t, loadFixture(t))
	f.SetError(fmt.Errorf("source down"))
	if n, err := Run(t.Context(), f, db, quiet(), at(tradingDay)); err == nil || n != 0 {
		t.Fatalf("n=%d err=%v, want an error", n, err)
	}
	if got := count(t, db, ""); got != 0 {
		t.Fatalf("%d rows written after a failed fetch", got)
	}
}

// A source that serves a stale trade date (e.g. Friday data stamped "today")
// must not create a duplicate row: identical figures overrule the date.
func TestRunStaleTradeTimeWithIdenticalFiguresIsSkipped(t *testing.T) {
	db := testDB(t)
	sat := tehran(2026, 11, 14, 13, 0, 0)
	stamp := func(day time.Time) *datasource.MarketSnapshot {
		s := loadFixture(t)
		tt := day.Add(-30 * time.Minute)
		for i := range s.Contracts {
			t2 := tt
			s.Contracts[i].LastTradeTime = &t2
		}
		return s
	}
	if n, err := Run(t.Context(), fakeOf(t, stamp(sat)), db, quiet(), at(sat)); err != nil || n == 0 {
		t.Fatalf("saturday: n=%d err=%v", n, err)
	}
	fri := sat.AddDate(0, 0, 6) // same figures, but the source claims they are from today
	if n, err := Run(t.Context(), fakeOf(t, stamp(fri)), db, quiet(), at(fri)); err != nil || n != 0 {
		t.Fatalf("stale source: n=%d err=%v, want 0", n, err)
	}
	// Genuinely new figures with today's trade time are stored.
	next := stamp(fri)
	next.Contracts[0].Volume += 5
	if n, err := Run(t.Context(), fakeOf(t, next), db, quiet(), at(fri)); err != nil || n == 0 {
		t.Fatalf("real trading: n=%d err=%v", n, err)
	}
}
