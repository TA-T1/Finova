// Package tsetmc is a datasource.Source backed by the undocumented JSON API of
// the Tehran Securities Exchange Technology Management Co. (TSETMC), served from
// https://cdn.tsetmc.com/api. The endpoints require a browser-like User-Agent
// and a tsetmc.com Referer/Origin, but no cookies or authentication.
package tsetmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL   = "https://cdn.tsetmc.com/api"
	defaultReferer   = "https://www.tsetmc.com/"
	defaultOrigin    = "https://www.tsetmc.com"
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

// Client is a thin HTTP wrapper around the TSETMC CDN API.
type Client struct {
	baseURL   string
	userAgent string
	referer   string
	origin    string
	http      *http.Client
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (no trailing slash).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithUserAgent overrides the User-Agent header sent to TSETMC.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithHTTPClient supplies a custom *http.Client (e.g. with a proxy or timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New builds a TSETMC client. Empty option values fall back to defaults.
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:   defaultBaseURL,
		userAgent: defaultUserAgent,
		referer:   defaultReferer,
		origin:    defaultOrigin,
		http:      &http.Client{Timeout: 30 * time.Second},
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
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	return c
}

// Name implements datasource.Source.
func (c *Client) Name() string { return "tsetmc" }

// getJSON performs a GET against path (joined to baseURL) and decodes the JSON
// body into out.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tsetmc: build request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", c.referer)
	req.Header.Set("Origin", c.origin)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tsetmc: GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tsetmc: GET %s: status %d: %s", path, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("tsetmc: decode %s: %w", path, err)
	}
	return nil
}
