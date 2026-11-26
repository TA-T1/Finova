package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSnakeCaseFixtureDetailThroughRepository(t *testing.T) {
	db := newBacktestTestDB(t)
	ctx := context.Background()

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "frontend", "src", "mocks", "backtestRuns.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture []map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture) == 0 {
		t.Fatal("fixture is empty")
	}
	first := fixture[0]

	paramsJSON, _ := json.Marshal(SnakeCaseKeys(first["params"]))
	metricsJSON, _ := json.Marshal(SnakeCaseKeys(first["metrics"]))
	tradesJSON, _ := json.Marshal(SnakeCaseKeys(first["trades"]))
	skippedJSON, _ := json.Marshal(SnakeCaseKeys(first["skipped"]))
	equityJSON, _ := json.Marshal(SnakeCaseKeys(first["equityCurve"]))
	createdAt, err := time.Parse(time.RFC3339, first["createdAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	startDate, _ := time.Parse("2006-01-02", first["startDate"].(string))
	endDate, _ := time.Parse("2006-01-02", first["endDate"].(string))

	var id int
	err = db.QueryRow(ctx, `INSERT INTO backtest_runs
		(id, model_name, params, start_date, end_date, win_rate, max_drawdown, total_return,
		 metrics, trades, skipped, equity_curve, created_at)
		VALUES (1,$1,$2::jsonb,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$11::jsonb,$12)
		ON CONFLICT (id) DO UPDATE SET model_name=EXCLUDED.model_name
		RETURNING id`,
		first["modelName"], paramsJSON, startDate, endDate,
		first["winRate"], first["maxDrawdown"], first["totalReturn"],
		metricsJSON, tradesJSON, skippedJSON, equityJSON, createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)
	detail, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if detail == nil {
		t.Fatal("detail unexpectedly nil")
	}

	gotJSON, _ := json.Marshal(detail)
	var got map[string]any
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatal(err)
	}
	delete(got, "createdAt")
	delete(got, "id")
	delete(got, "modelName")
	delete(got, "template")
	delete(got, "underlying")
	delete(got, "startDate")
	delete(got, "endDate")
	delete(got, "winRate")
	delete(got, "maxDrawdown")
	delete(got, "totalReturn")
	delete(got, "tradeCount")
	delete(got, "avgPnl")
	delete(got, "modelPricedShare")

	expectedKeys := []string{"params", "metrics", "trades", "skipped", "equityCurve"}
	for _, key := range expectedKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("detail missing %s", key)
		}
	}
	for _, key := range expectedKeys {
		fixtureValue := first[key]
		gotValue := got[key]
		if !reflect.DeepEqual(gotValue, fixtureValue) {
			t.Errorf("detail %s differs from fixture\n got: %#v\nwant: %#v", key, gotValue, fixtureValue)
		}
	}
	if detail.Template == nil || *detail.Template != "long-call" {
		t.Errorf("template=%v, want long-call", detail.Template)
	}
	if detail.Underlying == nil || *detail.Underlying != "خودرو" {
		t.Errorf("underlying=%v, want خودرو", detail.Underlying)
	}
	if detail.TradeCount != 20 || detail.AvgPnl == nil || *detail.AvgPnl != 16418 ||
		detail.ModelPricedShare == nil || *detail.ModelPricedShare != 1 {
		t.Errorf("summary metric fields: %+v", detail.Summary)
	}
}

func TestListFiltersAndNullMetrics(t *testing.T) {
	db := newBacktestTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(ctx, `INSERT INTO backtest_runs
		(model_name, params, start_date, end_date, win_rate, max_drawdown, total_return, metrics, created_at)
		VALUES
		('strategy:long-call','{"underlying":"خودرو"}','2026-01-01','2026-02-01',0.5,0.1,0.2,
		 '{"trades":5,"avg_pnl":1000,"model_priced_share":0.8}','2026-11-24T20:00:00Z'),
		('strategy:long-put','{"underlying":"خودرو"}','2026-01-01','2026-02-01',0.4,0.2,-0.1,
		 null,'2026-11-24T19:00:00Z'),
		('rsi-meanrev-v1','{}','2026-01-01','2026-02-01',0.3,0.3,0.1,
		 null,'2026-11-24T18:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)
	items, err := repo.List(ctx, ListQuery{Underlying: "خودرو", Template: "long-call", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Template == nil || *items[0].Template != "long-call" ||
		items[0].Underlying == nil || *items[0].Underlying != "خودرو" {
		t.Fatalf("combined filter items=%+v", items)
	}
	if items[0].TradeCount != 5 || items[0].AvgPnl == nil || *items[0].AvgPnl != 1000 ||
		items[0].ModelPricedShare == nil || *items[0].ModelPricedShare != 0.8 {
		t.Fatalf("metric fields=%+v", items[0])
	}

	items, err = repo.List(ctx, ListQuery{Underlying: "خودرو", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("underlying filter count=%d, want 2", len(items))
	}
	if items[1].TradeCount != 0 || items[1].AvgPnl != nil || items[1].ModelPricedShare != nil {
		t.Fatalf("null metrics item=%+v", items[1])
	}
}

func TestDetail404(t *testing.T) {
	db := newBacktestTestDB(t)
	repo := NewRepository(db)
	detail, err := repo.Get(context.Background(), 999999)
	if err != nil {
		t.Fatal(err)
	}
	if detail != nil {
		t.Fatalf("unknown detail=%+v, want nil", detail)
	}
}

func SnakeCaseKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[snakeCase(key)] = SnakeCaseKeys(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = SnakeCaseKeys(item)
		}
		return out
	default:
		return value
	}
}

func snakeCase(value string) string {
	var out []rune
	for i, r := range value {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out = append(out, '_')
			}
			out = append(out, r-'A'+'a')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

func newBacktestTestDB(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("backtest_test_%d", time.Now().UnixNano())
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
	paths, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(path), err)
		}
	}
	return db
}
