// Package optionbaaz is a placeholder datasource.Source for https://optionbaaz.ir.
//
// OptionBaaz is an options-analytics site that mostly re-derives Greeks, implied
// volatility and breakeven — which this project computes itself in its own
// options-math engine ("موتور محاسبات آپشن"). It is therefore the lowest-priority
// source and is stubbed out for now; the adapter exists so it can be added later
// (e.g. to cross-check computed analytics) without touching the collector.
package optionbaaz

import (
	"context"

	"Finova/internal/datasource"
)

const defaultBaseURL = "https://optionbaaz.ir"

// Client is a (currently unimplemented) optionbaaz.ir data source.
type Client struct {
	baseURL string
}

// New builds an optionbaaz.ir client. baseURL may be empty to use the default.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{baseURL: baseURL}
}

func (c *Client) Name() string { return "optionbaaz" }

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
