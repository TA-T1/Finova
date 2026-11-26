package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsConfiguredOriginsAndPreflight(t *testing.T) {
	router := NewRouter(nil, nil, []string{"http://localhost:3000", " http://localhost:5173 "})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/market/options", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", "GET")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("allow-methods header missing")
	}
}

func TestCORSDoesNotAllowUnconfiguredOrigin(t *testing.T) {
	router := NewRouter(nil, nil, []string{"http://localhost:3000"})
	request := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured origin received allow-origin %q", got)
	}
}
