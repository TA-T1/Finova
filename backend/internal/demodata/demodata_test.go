package demodata

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Needs a real PostgreSQL: set TEST_DATABASE_URL. Each test gets a throw-away
// schema with every migration applied.
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
	schema := fmt.Sprintf("demo_test_%d", rand.Int63())
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
	files, _ := filepath.Glob("../../migrations/*.up.sql")
	sort.Strings(files)
	for _, f := range files {
		ddl, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(ddl)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	return db
}

func seed(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO underlyings (symbol, name, ins_code) VALUES ('خودرو', 'ایران‌خودرو', 'U1')`,
		`INSERT INTO underlying_daily_prices (underlying_id, trade_date, close, final_price, volume)
		 SELECT id, d::date, 2400, 2390, 10 FROM underlyings, generate_series('2026-11-01'::date, '2026-11-05', '1 day') d`,
		`INSERT INTO option_contracts (symbol, underlying_id, option_type, strike_price, expiry_date, ins_code)
		 SELECT 'ضخود1', id, 'call', 2000, '2026-12-23', 'C1' FROM underlyings`,
		// two ticks for one contract: only the newest is exported
		`INSERT INTO option_quotes (contract_id, ts, last_price, volume, open_interest)
		 SELECT id, '2026-11-24 09:30+03:30', 500, 1, 9 FROM option_contracts`,
		`INSERT INTO option_quotes (contract_id, ts, last_price, volume, open_interest)
		 SELECT id, '2026-11-24 10:30+03:30', 520, 2, 9 FROM option_contracts`,
		`INSERT INTO underlying_quotes (underlying_id, ts, last_price) SELECT id, '2026-11-24 10:30+03:30', 2450 FROM underlyings`,
		`INSERT INTO quote_snapshots (trade_date, contract_symbol, underlying_symbol, option_type, strike, expiry, captured_at)
		 VALUES ('2026-11-23', 'ضخود1', 'خودرو', 'call', 2000, '2026-12-23', now())`,
	} {
		if _, err := db.Exec(ctx, q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
}

func TestExportThenImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := testDB(t)
	seed(t, src)
	dir := t.TempDir()

	exp, err := Export(ctx, src, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"underlyings": 1, "underlying_daily_prices": 5, "option_contracts": 1,
		"option_quotes": 1, "underlying_quotes": 1, "quote_snapshots": 1}
	for name, n := range want {
		if exp[name] != n {
			t.Errorf("exported %s = %d, want %d", name, exp[name], n)
		}
	}

	dst := testDB(t)
	imp, err := Import(ctx, dst, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, n := range want {
		if imp[name] != n {
			t.Errorf("imported %s = %d, want %d", name, imp[name], n)
		}
	}
	var last int64
	if err := dst.QueryRow(ctx, `SELECT last_price FROM option_quotes`).Scan(&last); err != nil || last != 520 {
		t.Errorf("newest quote = %d, %v; want 520", last, err)
	}

	// New rows must not collide with the loaded ids.
	if _, err := dst.Exec(ctx, `INSERT INTO underlyings (symbol) VALUES ('شپنا')`); err != nil {
		t.Errorf("insert after import: %v", err)
	}

	// A second import into a filled database is refused, unless replace is set.
	if _, err := Import(ctx, dst, dir, false); err == nil {
		t.Error("import into a non-empty database should fail")
	}
	if _, err := Import(ctx, dst, dir, true); err != nil {
		t.Errorf("import with replace: %v", err)
	}
	var n int
	dst.QueryRow(ctx, `SELECT count(*) FROM underlyings`).Scan(&n)
	if n != 1 {
		t.Errorf("underlyings after replace = %d, want 1", n)
	}
}

func TestImportMissingFileFailsBeforeTouchingTheDatabase(t *testing.T) {
	db := testDB(t)
	if _, err := Import(context.Background(), db, t.TempDir(), true); err == nil {
		t.Fatal("want an error for an empty directory")
	}
}
