// Command backfill fills quote_snapshots with option price history and moves
// the market data to and from files for an offline demo.
//
//	backfill history [-limit N]        fetch the daily history of every listed contract
//	                                   from TSETMC into quote_snapshots (about 15 min)
//	backfill export  <dir>             save the market tables to <dir>/*.csv.gz
//	backfill import  [-replace] <dir>  load them into a database (migrations applied)
//	backfill save-market <file.json>   save one live MarketSnapshot, loadable by source.NewFake
//
// history and save-market reach TSETMC; export and import only need DATABASE_URL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"Finova/internal/backfill"
	"Finova/internal/config"
	"Finova/internal/database"
	"Finova/internal/datasource/tsetmc"
	"Finova/internal/demodata"
)

const usage = `usage:
  backfill history [-limit N]
  backfill export <dir>
  backfill import [-replace] <dir>
  backfill save-market <file.json>`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		slog.Error("backfill", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	limit := fs.Int("limit", 0, "history: stop after this many contracts (0 = all)")
	replace := fs.Bool("replace", false, "import: empty the market tables first")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()

	switch cmd {
	case "history":
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		db, err := database.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer db.Close()
		res, err := backfill.Run(ctx, newClient(cfg), backfill.PoolWriter(db), backfill.WithLimit(*limit))
		fmt.Printf("contracts=%d fetched=%d failed=%d rows=%d inserted=%d range=%s..%s\n",
			res.Contracts, res.Fetched, res.Failed, res.Rows, res.Inserted, res.FirstDate, res.LastDate)
		if res.FirstErrMsg != "" {
			fmt.Println("first error:", res.FirstErrMsg)
		}
		return err

	case "export", "import":
		if len(rest) != 1 {
			return fmt.Errorf("%s needs one directory\n%s", cmd, usage)
		}
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		db, err := database.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer db.Close()
		var counts map[string]int64
		if cmd == "export" {
			counts, err = demodata.Export(ctx, db, rest[0])
		} else {
			counts, err = demodata.Import(ctx, db, rest[0], *replace)
		}
		for _, t := range demodata.Tables {
			if n, ok := counts[t.Name]; ok {
				fmt.Printf("%-24s %d rows\n", t.Name, n)
			}
		}
		return err

	case "save-market":
		if len(rest) != 1 {
			return fmt.Errorf("save-market needs one file\n%s", usage)
		}
		cfg, err := config.LoadSource()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		snap, err := newClient(cfg).FetchMarket(ctx)
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(snap, "", " ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(rest[0], b, 0o644); err != nil {
			return err
		}
		fmt.Printf("saved %d contracts, %d underlyings to %s\n", len(snap.Contracts), len(snap.Underlyings), rest[0])
		return nil
	}
	return fmt.Errorf("unknown command %q\n%s", cmd, usage)
}

func newClient(cfg *config.Config) *tsetmc.Client {
	return tsetmc.New(
		tsetmc.WithBaseURL(cfg.TSETMCBaseURL),
		tsetmc.WithUserAgent(cfg.TSETMCUserAgent),
		tsetmc.WithTimeout(cfg.TSETMCTimeout),
		tsetmc.WithMaxAttempts(cfg.TSETMCMaxAttempts),
		tsetmc.WithRateLimit(cfg.TSETMCRateLimit),
	)
}
