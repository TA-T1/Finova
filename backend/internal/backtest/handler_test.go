package backtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeRepository struct {
	items     []Summary
	detail    *Detail
	listErr   error
	getErr    error
	lastQuery ListQuery
}

func (f *fakeRepository) List(_ context.Context, query ListQuery) ([]Summary, error) {
	f.lastQuery = query
	return f.items, f.listErr
}

func (f *fakeRepository) Get(_ context.Context, id int) (*Detail, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.detail == nil {
		return nil, nil
	}
	return f.detail, nil
}

func TestParseListQuery(t *testing.T) {
	query, err := ParseListQuery(url.Values{
		"underlying": {"خودرو"},
		"template":   {"long-call"},
		"model":      {"strategy:long-call"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if query.Limit != 100 || query.Underlying != "خودرو" ||
		query.Template != "long-call" || query.Model != "strategy:long-call" {
		t.Fatalf("query = %+v", query)
	}
	for _, raw := range []string{"0", "-1", "501", "abc"} {
		if _, err := ParseListQuery(url.Values{"limit": {raw}}); err == nil {
			t.Errorf("limit=%q should fail", raw)
		}
	}
	query, err = ParseListQuery(url.Values{"limit": {"500"}})
	if err != nil || query.Limit != 500 {
		t.Fatalf("limit 500: query=%+v err=%v", query, err)
	}
}

func TestCamelCaseKeys(t *testing.T) {
	input := map[string]any{
		"entry_date": "2026-11-23",
		"legs":       []any{map[string]any{"entry_price": 150, "price_source": "model"}},
	}
	got := CamelCaseKeys(input).(map[string]any)
	if got["entryDate"] != "2026-11-23" {
		t.Fatalf("nested key not converted: %+v", got)
	}
	legs := got["legs"].([]any)
	leg := legs[0].(map[string]any)
	if leg["entryPrice"] != 150 || leg["priceSource"] != "model" {
		t.Fatalf("leg keys not converted: %+v", leg)
	}
}

func TestHandlerListAndDetailStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepository{
		items:  []Summary{{ID: 1, ModelName: "strategy:long-call", TradeCount: 0}},
		detail: &Detail{Summary: Summary{ID: 7, ModelName: "strategy:long-call"}, Trades: []any{}, Skipped: []any{}, EquityCurve: []any{}},
	}
	handler := NewHandler(repo)
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1"))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/backtests?underlying=x&template=long-call&limit=20", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	if repo.lastQuery.Underlying != "x" || repo.lastQuery.Template != "long-call" || repo.lastQuery.Limit != 20 {
		t.Fatalf("list query not parsed: %+v", repo.lastQuery)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/backtests?limit=501", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status=%d", response.Code)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/backtests/not-an-int", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("bad id status=%d", response.Code)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/backtests/7", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
	}

	repo.detail = nil
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/backtests/999", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown detail status=%d", response.Code)
	}
}

func TestListMetricsNullDefaults(t *testing.T) {
	metrics, _ := decodeObject(nil)
	tradeCount := 0
	var avgPnl *float64
	var modelShare *float64
	if count, ok := numberValue(metrics["trades"]); ok {
		tradeCount = int(count)
	}
	avgPnl = floatValue(metrics["avg_pnl"])
	modelShare = floatValue(metrics["model_priced_share"])
	if tradeCount != 0 || avgPnl != nil || modelShare != nil {
		t.Fatalf("null metrics defaults: trades=%d avg=%v share=%v", tradeCount, avgPnl, modelShare)
	}
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
