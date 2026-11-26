package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"Finova/internal/datasource"
)

func TestParseOptionListQuery(t *testing.T) {
	tests := []struct {
		name    string
		values  url.Values
		wantErr bool
		check   func(t *testing.T, got OptionListQuery)
	}{
		{
			name:   "defaults",
			values: url.Values{},
			check: func(t *testing.T, got OptionListQuery) {
				if got.Page != 1 || got.PageSize != 50 || got.Order != "asc" {
					t.Fatalf("defaults = page %d, pageSize %d, order %q", got.Page, got.PageSize, got.Order)
				}
			},
		},
		{
			name:   "limit alias",
			values: url.Values{"limit": {"25"}},
			check: func(t *testing.T, got OptionListQuery) {
				if got.PageSize != 25 {
					t.Fatalf("pageSize = %d, want 25", got.PageSize)
				}
			},
		},
		{
			name:   "pageSize takes precedence over alias",
			values: url.Values{"limit": {"25"}, "pageSize": {"8"}},
			check: func(t *testing.T, got OptionListQuery) {
				if got.PageSize != 8 {
					t.Fatalf("pageSize = %d, want 8", got.PageSize)
				}
			},
		},
		{
			name:   "maximum page size",
			values: url.Values{"pageSize": {"500"}},
			check: func(t *testing.T, got OptionListQuery) {
				if got.PageSize != 500 {
					t.Fatalf("pageSize = %d, want 500", got.PageSize)
				}
			},
		},
		{name: "unknown sort", values: url.Values{"sort": {"c.symbol; DROP TABLE option_contracts"}}, wantErr: true},
		{name: "invalid order", values: url.Values{"order": {"sideways"}}, wantErr: true},
		{name: "zero page", values: url.Values{"page": {"0"}}, wantErr: true},
		{name: "zero pageSize", values: url.Values{"pageSize": {"0"}}, wantErr: true},
		{name: "pageSize too large", values: url.Values{"pageSize": {"501"}}, wantErr: true},
		{name: "bad expiry", values: url.Values{"expiry": {"2026-02-30"}}, wantErr: true},
		{name: "noncanonical expiry", values: url.Values{"expiry": {"2026-1-02"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOptionListQuery(tt.values)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseOptionListQuery() err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestParseOptionListQueryAcceptsSortableFields(t *testing.T) {
	keys := []string{
		"symbol", "underlyingSymbol", "optionType", "strikePrice", "expiryDate",
		"daysToExpiry", "lastPrice", "volume", "openInterest", "impliedVolatility",
		"delta", "leverage", "effectiveLeverage", "timeValue", "intrinsicValue",
		"priceGapPercent", "breakEvenPercent", "bestBid", "value", "moneyness", "breakEven",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			if _, err := ParseOptionListQuery(url.Values{"sort": {key}}); err != nil {
				t.Fatalf("sort %q rejected: %v", key, err)
			}
		})
	}
}

func TestMarketHandlersRejectInvalidQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(nil, nil).RegisterRoutes(router.Group("/api/v1"))
	tests := []struct {
		path string
	}{
		{path: "/api/v1/market/options?sort=notAColumn"},
		{path: "/api/v1/market/options?order=up"},
		{path: "/api/v1/market/options?expiry=2026-13-40"},
		{path: "/api/v1/market/options?page=0"},
		{path: "/api/v1/market/options?pageSize=501"},
		{path: "/api/v1/market/chain?underlying=AAA"},
		{path: "/api/v1/market/chain?underlying=AAA&expiry=bad"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; response: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestMarketOptionsDatabaseQueries(t *testing.T) {
	db := newMarketTestDB(t, true)
	ids, today, tomorrow := seedMarketTestData(t, db)
	repo := NewRepository(db)
	ctx := context.Background()
	for _, row := range []struct {
		id        int
		yesterday int64
		wantPrev  *int64
	}{
		{id: ids.expiredCall, yesterday: 2, wantPrev: int64Ptr(2)},
		{id: ids.noMetricCall, yesterday: 0, wantPrev: nil},
	} {
		ts := time.Now().Add(time.Hour)
		if err := repo.InsertOptionQuote(ctx, row.id, ts, datasource.OptionLeg{
			Last: 12, Value: 77, Yesterday: row.yesterday,
		}); err != nil {
			t.Fatal(err)
		}
		var got *int64
		if err := db.QueryRow(ctx, `SELECT prev_close FROM option_quotes_latest WHERE contract_id=$1`, row.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (row.wantPrev == nil) || (got != nil && *got != *row.wantPrev) {
			t.Fatalf("contract %d prev_close = %v, want %v", row.id, got, row.wantPrev)
		}
	}

	filtered, err := repo.ListMarketOptions(ctx, OptionListQuery{
		UnderlyingSymbol: "AAA",
		UnderlyingKind:   "stock",
		OptionType:       "call",
		Expiry:           &today,
		Page:             1,
		PageSize:         50,
		Order:            "asc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || filtered.Count != 1 || filtered.Items[0].ContractID != ids.aaaCallToday {
		t.Fatalf("filtered page = %+v, want only today's AAA call", filtered)
	}
	if filtered.Items[0].Value == nil || *filtered.Items[0].Value != 1100 ||
		filtered.Items[0].PrevClose == nil || *filtered.Items[0].PrevClose != 9 {
		t.Fatalf("quote additions are missing: value=%v prevClose=%v", filtered.Items[0].Value, filtered.Items[0].PrevClose)
	}
	if filtered.Items[0].ExpiryDate != today.Format("2006-01-02") {
		t.Fatalf("expiryDate = %q", filtered.Items[0].ExpiryDate)
	}

	pastEnd, err := repo.ListMarketOptions(ctx, OptionListQuery{Page: 99, PageSize: 1, Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if pastEnd.Count != 0 || len(pastEnd.Items) != 0 || pastEnd.Total != 5 || pastEnd.TotalPages != 5 {
		t.Fatalf("past-end page = %+v, want empty page with total 5 and 5 pages", pastEnd)
	}

	for _, order := range []string{"asc", "desc"} {
		t.Run("nulls last "+order, func(t *testing.T) {
			seen := map[int]bool{}
			var lastBid *int64
			sawNull := false
			for page := 1; page <= 5; page++ {
				got, err := repo.ListMarketOptions(ctx, OptionListQuery{
					Sort: "bestBid", Order: order, Page: page, PageSize: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				if got.Count != 1 {
					t.Fatalf("page %d count = %d", page, got.Count)
				}
				item := got.Items[0]
				if seen[item.ContractID] {
					t.Fatalf("contract %d repeated on page %d", item.ContractID, page)
				}
				seen[item.ContractID] = true
				if item.BestBid == nil {
					sawNull = true
					continue
				}
				if sawNull {
					t.Fatalf("non-null bid appeared after null on page %d", page)
				}
				if lastBid != nil {
					if order == "asc" && *item.BestBid < *lastBid {
						t.Fatalf("ascending bids out of order: %d after %d", *item.BestBid, *lastBid)
					}
					if order == "desc" && *item.BestBid > *lastBid {
						t.Fatalf("descending bids out of order: %d after %d", *item.BestBid, *lastBid)
					}
				}
				lastBid = item.BestBid
			}
			if len(seen) != 5 {
				t.Fatalf("seen %d contracts, want 5", len(seen))
			}
		})
	}
	tied, err := repo.ListMarketOptions(ctx, OptionListQuery{
		Sort: "bestBid", Order: "asc", Page: 1, PageSize: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	positions := map[int]int{}
	for i, item := range tied.Items {
		positions[item.ContractID] = i
	}
	if positions[ids.aaaCallToday] >= positions[ids.bbbCallTomorrow] {
		t.Fatalf("equal sort values do not use contract id as the tie breaker: %+v", tied.Items)
	}

	t.Run("expiry counts use Tehran today", func(t *testing.T) {
		symbol := "AAA"
		result, err := repo.ListExpiries(ctx, &symbol, today.Add(12*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if result.UnderlyingSymbol == nil || *result.UnderlyingSymbol != symbol || len(result.Items) != 2 {
			t.Fatalf("expiries = %+v, want two active AAA expiries", result)
		}
		if result.Items[0].ExpiryDate != today.Format("2006-01-02") ||
			result.Items[0].DaysToExpiry != 0 || result.Items[0].ContractCount != 2 {
			t.Fatalf("today expiry = %+v", result.Items[0])
		}
		if result.Items[1].ExpiryDate != tomorrow.Format("2006-01-02") ||
			result.Items[1].DaysToExpiry != 1 || result.Items[1].ContractCount != 1 {
			t.Fatalf("tomorrow expiry = %+v", result.Items[1])
		}
	})

	t.Run("chain includes quote and metric fields", func(t *testing.T) {
		strikes, err := repo.ListOptionChainForExpiry(ctx, "AAA", today)
		if err != nil {
			t.Fatal(err)
		}
		if len(strikes) != 1 || strikes[0].Call == nil || strikes[0].Put == nil {
			t.Fatalf("chain = %+v", strikes)
		}
		call := strikes[0].Call
		if call.ContractID != ids.aaaCallToday ||
			call.Value == nil || *call.Value != 1100 ||
			call.PrevClose == nil || *call.PrevClose != 9 ||
			call.Delta == nil || *call.Delta != 0.6 ||
			call.MarketPrice == nil || *call.MarketPrice != 11 {
			t.Fatalf("chain call missing data: %+v", call)
		}
		encoded, err := json.Marshal(call)
		if err != nil {
			t.Fatal(err)
		}
		var side map[string]any
		if err := json.Unmarshal(encoded, &side); err != nil {
			t.Fatal(err)
		}
		if _, ok := side["underlyingPrice"]; ok {
			t.Fatal("underlyingPrice leaked onto chain side")
		}
		if _, ok := side["daysToExpiry"]; ok {
			t.Fatal("daysToExpiry leaked onto chain side")
		}
		price, err := repo.LatestUnderlyingPrice(ctx, "AAA")
		if err != nil || price == nil || *price != 100 {
			t.Fatalf("latest underlying price = %v, err=%v", price, err)
		}
	})

	t.Run("handler empty page and not found", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		NewHandler(nil, repo).RegisterRoutes(router.Group("/api/v1"))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/market/options?page=99", nil))
		var page MarketOptionsPage
		if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || page.Count != 0 || page.Total != 5 || page.Items == nil {
			t.Fatalf("empty page status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/market/options?underlying=BBB", nil))
		var noMetrics struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &noMetrics); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || len(noMetrics.Items) != 1 {
			t.Fatalf("no-metrics option response status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		for _, field := range []string{"underlyingPrice", "daysToExpiry", "moneyness", "marketPrice", "metricsTs"} {
			value, present := noMetrics.Items[0][field]
			if !present || value != nil {
				t.Fatalf("calculated field %q = %v (present %t), want null", field, value, present)
			}
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
			"/api/v1/market/chain?underlying=AAA&expiry="+today.Format("2006-01-02"), nil))
		var chain map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &chain); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || chain["daysToExpiry"] != float64(0) ||
			chain["underlyingPrice"] != float64(100) {
			t.Fatalf("chain top-level fields status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
			"/api/v1/market/chain?underlying=missing&expiry="+today.Format("2006-01-02"), nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("chain status=%d, want 404: %s", recorder.Code, recorder.Body.String())
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/market/expiries?underlying=missing", nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expiries status=%d, want 404: %s", recorder.Code, recorder.Body.String())
		}
		underlyingPage := httptest.NewRecorder()
		router.ServeHTTP(underlyingPage, httptest.NewRequest(http.MethodGet, "/api/v1/market/underlyings/AAA", nil))
		symbolAlias := httptest.NewRecorder()
		router.ServeHTTP(symbolAlias, httptest.NewRequest(http.MethodGet, "/api/v1/market/symbol/AAA", nil))
		if underlyingPage.Code != http.StatusOK || symbolAlias.Code != http.StatusOK ||
			underlyingPage.Body.String() != symbolAlias.Body.String() {
			t.Fatalf("symbol path status/body differ: underlying=%d %s alias=%d %s",
				underlyingPage.Code, underlyingPage.Body.String(), symbolAlias.Code, symbolAlias.Body.String())
		}
		var symbolShape map[string]any
		if err := json.Unmarshal(underlyingPage.Body.Bytes(), &symbolShape); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{
			"symbol", "name", "kind", "price", "priceChange", "priceChangePercent",
			"expiries", "openInterest", "strikeOpenInterest", "maxPain",
		} {
			if _, present := symbolShape[field]; !present {
				t.Fatalf("symbol page field %q missing: %s", field, underlyingPage.Body.String())
			}
		}
	})
}

func TestOptionQuotePrevCloseMigrationCanBeReappliedAndReversed(t *testing.T) {
	db := newMarketTestDB(t, false)
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000011_option_quote_prev_close.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := db.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}
	assertPrevCloseColumn(t, db, true)

	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000011_option_quote_prev_close.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), string(down)); err != nil {
		t.Fatalf("reverse migration: %v", err)
	}
	assertPrevCloseColumn(t, db, false)
}

type marketTestIDs struct {
	aaaCallToday    int
	noMetricCall    int
	expiredCall     int
	bbbCallTomorrow int
}

func seedMarketTestData(t *testing.T, db *pgxpool.Pool) (marketTestIDs, time.Time, time.Time) {
	t.Helper()
	ctx := context.Background()
	today := tehranDate(time.Now())
	tomorrow := today.AddDate(0, 0, 1)
	yesterday := today.AddDate(0, 0, -1)
	var aaaID, bbbID int
	if err := db.QueryRow(ctx, `INSERT INTO underlyings(symbol, kind) VALUES ('AAA','stock') RETURNING id`).Scan(&aaaID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO underlyings(symbol, kind) VALUES ('BBB','fund') RETURNING id`).Scan(&bbbID); err != nil {
		t.Fatal(err)
	}
	type contract struct {
		symbol string
		uid    int
		typ    string
		strike int64
		expiry time.Time
	}
	contracts := []contract{
		{symbol: "AAA-C-TODAY", uid: aaaID, typ: "call", strike: 100, expiry: today},
		{symbol: "AAA-P-TODAY", uid: aaaID, typ: "put", strike: 100, expiry: today},
		{symbol: "AAA-C-TOMORROW", uid: aaaID, typ: "call", strike: 105, expiry: tomorrow},
		{symbol: "BBB-C-TOMORROW", uid: bbbID, typ: "call", strike: 100, expiry: tomorrow},
		{symbol: "AAA-C-PAST", uid: aaaID, typ: "call", strike: 95, expiry: yesterday},
	}
	ids := make([]int, len(contracts))
	for i, contract := range contracts {
		if err := db.QueryRow(ctx, `INSERT INTO option_contracts
			(symbol, underlying_id, option_type, strike_price, expiry_date, contract_size)
			VALUES ($1,$2,$3::option_type,$4,$5,1000) RETURNING id`,
			contract.symbol, contract.uid, contract.typ, contract.strike, contract.expiry).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	idsResult := marketTestIDs{
		aaaCallToday: ids[0], noMetricCall: ids[2], expiredCall: ids[4], bbbCallTomorrow: ids[3],
	}
	quotes := []struct {
		id      int
		bid     *int64
		value   int64
		prev    int64
		last    int64
		underID int
	}{
		{id: ids[0], bid: int64Ptr(10), value: 1100, prev: 9, last: 11, underID: aaaID},
		{id: ids[1], bid: int64Ptr(20), value: 1200, prev: 19, last: 21, underID: aaaID},
		{id: ids[2], bid: nil, value: 0, prev: 0, last: 0, underID: aaaID},
		{id: ids[3], bid: int64Ptr(10), value: 1400, prev: 8, last: 10, underID: bbbID},
		{id: ids[4], bid: nil, value: 0, prev: 0, last: 0, underID: aaaID},
	}
	for _, quote := range quotes {
		if _, err := db.Exec(ctx, `INSERT INTO option_quotes
			(contract_id, ts, last_price, close_price, best_bid, best_ask,
			 volume, value, open_interest, prev_close)
			VALUES ($1, now(), $2, $2, $3, NULL, 100, $4, 50, NULLIF($5, 0))`,
			quote.id, quote.last, quote.bid, quote.value, quote.prev); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO underlying_quotes
		(underlying_id, ts, last_price, close_price, prev_close)
		VALUES ($1, now(), 100, 99, 95)`, aaaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO option_metrics
		(contract_id, ts, underlying_price, days_to_expiry, risk_free_rate,
		 market_price, price_source, moneyness, intrinsic_value, delta)
		VALUES ($1, now(), 100, 0, 0.3, 11, 'last', 'ITM', 0, 0.6)`, ids[0]); err != nil {
		t.Fatal(err)
	}
	return idsResult, today, tomorrow
}

func int64Ptr(value int64) *int64 { return &value }

func newMarketTestDB(t *testing.T, applyLatest bool) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("market_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, path := range files {
		if !applyLatest && filepath.Base(path) == "000011_option_quote_prev_close.up.sql" {
			continue
		}
		if _, err := db.Exec(ctx, readMigration(t, path)); err != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(path), err)
		}
	}
	return db
}

func readMigration(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func assertPrevCloseColumn(t *testing.T, db *pgxpool.Pool, want bool) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'option_quotes'
			  AND column_name = 'prev_close'
		)`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != want {
		t.Fatalf("prev_close exists=%t, want %t", exists, want)
	}
}
