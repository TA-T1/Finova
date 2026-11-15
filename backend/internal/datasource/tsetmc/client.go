// Package tsetmc is a datasource.Source backed by the undocumented JSON API of
// the Tehran Securities Exchange Technology Management Co. (TSETMC), served from
// https://cdn.tsetmc.com/api. The endpoints require a browser-like User-Agent
// and a tsetmc.com Referer/Origin, but no cookies or authentication.
package tsetmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL   = "https://cdn.tsetmc.com/api"
	defaultReferer   = "https://www.tsetmc.com/"
	defaultOrigin    = "https://www.tsetmc.com"
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

	defaultTimeout     = 10 * time.Second
	defaultMaxAttempts = 3
	defaultBackoff     = 500 * time.Millisecond
	maxBackoff         = 10 * time.Second
	// TSETMC documents no rate limit; 2 requests/second is a polite default.
	defaultRateLimit = 2.0
	maxBodyBytes     = 64 << 20
)

// Client is an HTTP wrapper around the TSETMC CDN API. It applies a
// per-request timeout, retries transient failures with exponential backoff and
// throttles requests to a configurable rate.
type Client struct {
	baseURL     string
	userAgent   string
	referer     string
	origin      string
	http        *http.Client
	timeout     time.Duration
	timeoutSet  bool // WithTimeout was used
	maxAttempts int
	backoff     time.Duration
	limiter     *rate.Limiter // nil = unlimited
	log         *slog.Logger
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (no trailing slash).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithUserAgent overrides the User-Agent header sent to TSETMC.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithHTTPClient supplies a custom *http.Client (e.g. with a proxy). If that
// client has its own Timeout and WithTimeout is not used, the per-attempt
// timeout follows it, as before the retry layer existed.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeout sets the timeout of each single HTTP attempt (default 10s).
// Zero or negative keeps the default.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout, c.timeoutSet = d, d > 0 }
}

// WithMaxAttempts sets the total number of attempts per request, including the
// first one (default 3). Values below 1 keep the default.
func WithMaxAttempts(n int) Option { return func(c *Client) { c.maxAttempts = n } }

// WithBackoff sets the first retry delay; it doubles on each further retry
// (default 500ms, capped at 10s). Zero or negative keeps the default.
func WithBackoff(d time.Duration) Option { return func(c *Client) { c.backoff = d } }

// WithRateLimit caps requests per second (default 2). Zero or negative
// disables limiting.
func WithRateLimit(rps float64) Option {
	return func(c *Client) {
		if rps > 0 {
			c.limiter = rate.NewLimiter(rate.Limit(rps), 1)
		} else {
			c.limiter = nil
		}
	}
}

// WithLogger sets the logger used for warnings (default slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.log = l } }

// New builds a TSETMC client. Empty or zero option values fall back to defaults.
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:     defaultBaseURL,
		userAgent:   defaultUserAgent,
		referer:     defaultReferer,
		origin:      defaultOrigin,
		http:        &http.Client{},
		timeout:     defaultTimeout,
		maxAttempts: defaultMaxAttempts,
		backoff:     defaultBackoff,
		limiter:     rate.NewLimiter(rate.Limit(defaultRateLimit), 1),
		log:         slog.Default(),
	}
	for _, o := range opts {
		o(c)
	}
	if c.baseURL == "" {
		c.baseURL = defaultBaseURL
	}
	if c.userAgent == "" {
		c.userAgent = defaultUserAgent
	}
	if c.http == nil {
		c.http = &http.Client{}
	}
	if c.timeout <= 0 {
		c.timeout = defaultTimeout
	}
	if !c.timeoutSet && c.http.Timeout > 0 {
		c.timeout = c.http.Timeout // a custom client's own timeout still applies
	}
	if c.maxAttempts < 1 {
		c.maxAttempts = defaultMaxAttempts
	}
	if c.backoff <= 0 {
		c.backoff = defaultBackoff
	}
	if c.log == nil {
		c.log = slog.Default()
	}
	return c
}

// Name implements datasource.Source.
func (c *Client) Name() string { return "tsetmc" }

// statusError is a non-200 response.
type statusError struct {
	status     int
	body       string
	retryAfter time.Duration
}

func (e *statusError) Error() string { return fmt.Sprintf("status %d: %s", e.status, e.body) }

// retryable reports whether a failed attempt should be retried: network errors,
// timeouts, 5xx and 429. Other 4xx and decode errors are final.
func retryable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		return se.status >= 500 || se.status == http.StatusTooManyRequests
	}
	var de *decodeError
	return !errors.As(err, &de)
}

type decodeError struct{ err error }

func (e *decodeError) Error() string { return e.err.Error() }
func (e *decodeError) Unwrap() error { return e.err }

// getBytes performs a GET against path (joined to baseURL) with retries and
// returns the response body.
func (c *Client) getBytes(ctx context.Context, path string) ([]byte, error) {
	var lastErr error
	delay := c.backoff
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("tsetmc: GET %s: %w", path, err)
		}
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return nil, fmt.Errorf("tsetmc: GET %s: rate limiter: %w", path, err)
			}
		}
		b, err := c.attempt(ctx, path)
		if err == nil {
			return b, nil
		}
		lastErr = err
		// The caller's context ended: stop, do not retry.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tsetmc: GET %s: %w", path, ctx.Err())
		}
		if !retryable(err) || attempt == c.maxAttempts {
			break
		}
		wait := delay
		var se *statusError
		if errors.As(err, &se) && se.retryAfter > 0 {
			wait = se.retryAfter
		}
		if wait > maxBackoff {
			wait = maxBackoff
		}
		c.log.Warn("tsetmc request failed, retrying", "path", path, "attempt", attempt, "wait", wait, "err", err)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, fmt.Errorf("tsetmc: GET %s: %w", path, ctx.Err())
		}
		delay *= 2
	}
	return nil, fmt.Errorf("tsetmc: GET %s: %w", path, lastErr)
}

// attempt performs one HTTP request bounded by the per-request timeout.
func (c *Client) attempt(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, &decodeError{fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", c.referer)
	req.Header.Set("Origin", c.origin)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		se := &statusError{status: resp.StatusCode, body: string(b)}
		if s := resp.Header.Get("Retry-After"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				se.retryAfter = time.Duration(n) * time.Second
			}
		}
		return nil, se
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err // truncated body: a network error, retryable
	}
	return b, nil
}

// getJSON performs a GET and decodes the JSON body into out.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	b, err := c.getBytes(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("tsetmc: decode %s: %w", path, err)
	}
	return nil
}
