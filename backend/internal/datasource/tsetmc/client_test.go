package tsetmc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newTestClient points a client at srv with fast backoff and no rate limit.
func newTestClient(srv *httptest.Server, opts ...Option) *Client {
	base := []Option{
		WithBaseURL(srv.URL), WithBackoff(time.Millisecond), WithRateLimit(0),
		WithLogger(quietLogger()),
	}
	return New(append(base, opts...)...)
}

func TestRetryOn500ThenSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	var out struct{ OK bool }
	if err := newTestClient(srv).getJSON(context.Background(), "/x", &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("ok=%v hits=%d, want ok after 2 hits", out.OK, atomic.LoadInt32(&hits))
	}
}

func TestRetryOn429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	if _, err := newTestClient(srv).getBytes(context.Background(), "/x"); err != nil || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("err=%v hits=%d, want success after 2 hits", err, atomic.LoadInt32(&hits))
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := newTestClient(srv).getBytes(context.Background(), "/missing")
	if err == nil || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("err=%v hits=%d, want one failed attempt", err, atomic.LoadInt32(&hits))
	}
	if !strings.Contains(err.Error(), "/missing") || !strings.Contains(err.Error(), "404") {
		t.Errorf("error lacks request context: %v", err)
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := newTestClient(srv, WithMaxAttempts(3)).getBytes(context.Background(), "/x")
	if err == nil || atomic.LoadInt32(&hits) != 3 || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err=%v hits=%d, want 3 attempts ending in a 502 error", err, atomic.LoadInt32(&hits))
	}
}

func TestRequestTimeoutIsRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select { // hang until the client gives up
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	start := time.Now()
	_, err := newTestClient(srv, WithTimeout(40*time.Millisecond), WithMaxAttempts(2)).
		getBytes(context.Background(), "/slow")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v, want deadline exceeded", err)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("hits=%d, want 2 (timeout is retried)", atomic.LoadInt32(&hits))
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("took %v, timeout not enforced", time.Since(start))
	}
}

func TestContextCancellationStopsRetries(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := newTestClient(srv, WithTimeout(5*time.Second), WithMaxAttempts(5)).getBytes(ctx, "/slow")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if atomic.LoadInt32(&hits) != 1 || time.Since(start) > 2*time.Second {
		t.Errorf("hits=%d after %v, want 1 hit and a quick return", atomic.LoadInt32(&hits), time.Since(start))
	}
}

func TestAlreadyCancelledContextSendsNothing(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newTestClient(srv).FetchMarket(ctx); !errors.Is(err, context.Canceled) || atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("err=%v hits=%d, want Canceled and no request", err, atomic.LoadInt32(&hits))
	}
}

func TestRateLimitSpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer srv.Close()
	c := newTestClient(srv, WithRateLimit(20)) // 50ms between requests
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.getBytes(context.Background(), "/x"); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 90*time.Millisecond {
		t.Errorf("3 requests took %v, want >= ~100ms at 20 req/s", d)
	}
}

func TestSendsBrowserHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.tsetmc.com/" || r.Header.Get("User-Agent") == "" {
			http.Error(w, "bad headers", http.StatusForbidden)
			return
		}
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	if _, err := newTestClient(srv).getBytes(context.Background(), "/x"); err != nil {
		t.Fatal(err)
	}
}

func TestCustomHTTPClientTimeoutIsStillHonoured(t *testing.T) {
	custom := &http.Client{Timeout: 45 * time.Second}
	if got := New(WithHTTPClient(custom)).timeout; got != 45*time.Second {
		t.Errorf("timeout = %v, want the custom client's 45s", got)
	}
	if got := New(WithHTTPClient(custom), WithTimeout(2*time.Second)).timeout; got != 2*time.Second {
		t.Errorf("explicit WithTimeout should win, got %v", got)
	}
	if got := New().timeout; got != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", got)
	}
}
