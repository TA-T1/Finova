// Package tse is a placeholder datasource.Source for the official Tehran Stock
// Exchange site (https://www.tse.ir). TSETMC already covers the live chain,
// daily history and instrument identity the project needs, so this adapter is a
// stub for now; it exists so tse.ir reference/disclosure data can be layered in
// later without changing the collector or DB layer.
package tse

import (
	"context"

	"Finova/internal/datasource"
)

const defaultBaseURL = "https://www.tse.ir"

// Client is a (currently unimplemented) tse.ir data source.
type Client struct {
	baseURL string
}

// New builds a tse.ir client. baseURL may be empty to use the default.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{baseURL: baseURL}
}

func (c *Client) Name() string { return "tse" }

func (c *Client) OptionMarketWatch(ctx context.Context) ([]datasource.OptionRow, error) {
	return nil, datasource.ErrNotSupported
}

func (c *Client) DailyPrices(ctx context.Context, insCode string, adjusted bool) ([]datasource.DailyPrice, error) {
	return nil, datasource.ErrNotSupported
}

func (c *Client) InstrumentInfo(ctx context.Context, insCode string) (*datasource.InstrumentInfo, error) {
	return nil, datasource.ErrNotSupported
}

var _ datasource.Source = (*Client)(nil)
