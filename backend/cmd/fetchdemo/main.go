// Command fetchdemo calls the real market source once and prints a summary:
// contract and underlying counts, 5 sample contracts and the call duration.
//
//	go run ./cmd/fetchdemo
//
// It reads the same TSETMC_* settings as the API (see .env.example) but does
// not need a database.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"Finova/internal/config"
	"Finova/internal/datasource"
	"Finova/internal/datasource/tsetmc"
)

func main() {
	cfg, err := config.LoadSource()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	var src datasource.MarketSource = tsetmc.New(
		tsetmc.WithBaseURL(cfg.TSETMCBaseURL),
		tsetmc.WithUserAgent(cfg.TSETMCUserAgent),
		tsetmc.WithTimeout(cfg.TSETMCTimeout),
		tsetmc.WithMaxAttempts(cfg.TSETMCMaxAttempts),
		tsetmc.WithRateLimit(cfg.TSETMCRateLimit),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	start := time.Now()
	snap, err := src.FetchMarket(ctx)
	took := time.Since(start)
	if err != nil {
		slog.Error("fetch market", "err", err, "took", took)
		os.Exit(1)
	}

	fmt.Printf("contracts:   %d\n", len(snap.Contracts))
	fmt.Printf("underlyings: %d\n", len(snap.Underlyings))
	fmt.Printf("fetched at:  %s\n\n", snap.FetchedAt.Format(time.RFC3339))

	fmt.Println("sample contracts:")
	step := len(snap.Contracts) / 5
	if step < 1 {
		step = 1
	}
	shown := 0
	for i := 0; i < len(snap.Contracts) && shown < 5; i += step {
		c := snap.Contracts[i]
		fmt.Printf("  %-10s %-4s under=%-8s strike=%-8.0f expiry=%s last=%-8.0f bid=%-8.0f ask=%-8.0f vol=%-9d OI=%d\n",
			c.Symbol, c.Type, c.UnderlyingSymbol, c.Strike, c.Expiry.Format("2006-01-02"),
			c.LastPrice, c.BidPrice, c.AskPrice, c.Volume, c.OpenInterest)
		shown++
	}
	fmt.Printf("\ncall took: %s\n", took.Round(time.Millisecond))
}
