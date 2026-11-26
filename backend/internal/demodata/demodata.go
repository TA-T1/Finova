// Package demodata saves the market tables to files and loads them back, so
// the app can run offline with real data (a demo, or a fresh machine).
//
// A dataset is a directory with one gzip CSV file per table (<table>.csv.gz,
// with a header row). Export reads the tables; Import loads them into an empty
// database whose migrations are applied.
package demodata

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Table is one table of the dataset.
type Table struct {
	Name string
	// Export is the query that reads the rows to save. Only the newest quote of
	// each contract is exported for the quote tables, because the demo needs the
	// current market, not every tick.
	Export string
	// HasSerialID marks tables whose id sequence must be moved past the loaded ids.
	HasSerialID bool
	// Optional marks a file that older datasets do not have: Import skips the
	// table when the file is missing.
	Optional bool
}

// Tables lists the dataset in load order (parents before children).
var Tables = []Table{
	{Name: "underlyings", Export: "SELECT * FROM underlyings ORDER BY id", HasSerialID: true},
	{Name: "underlying_daily_prices", Export: "SELECT * FROM underlying_daily_prices ORDER BY underlying_id, trade_date"},
	{Name: "option_contracts", Export: "SELECT * FROM option_contracts ORDER BY id", HasSerialID: true},
	{Name: "option_quotes", Export: "SELECT * FROM option_quotes_latest ORDER BY id", HasSerialID: true},
	// After the contracts it references. Without these rows every calculated field of the API is null.
	{Name: "option_metrics", Export: "SELECT * FROM option_metrics_latest ORDER BY contract_id", Optional: true},
	{Name: "underlying_quotes", Export: "SELECT DISTINCT ON (underlying_id) * FROM underlying_quotes ORDER BY underlying_id, ts DESC", HasSerialID: true},
	{Name: "quote_snapshots", Export: "SELECT * FROM quote_snapshots ORDER BY trade_date, contract_symbol"},
}

func path(dir string, t Table) string { return filepath.Join(dir, t.Name+".csv.gz") }

// Export writes every table to dir and returns the number of rows per table.
func Export(ctx context.Context, db *pgxpool.Pool, dir string) (map[string]int64, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	counts := map[string]int64{}
	for _, t := range Tables {
		f, err := os.Create(path(dir, t))
		if err != nil {
			return counts, err
		}
		zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
		tag, err := conn.Conn().PgConn().CopyTo(ctx, zw,
			fmt.Sprintf("COPY (%s) TO STDOUT WITH (FORMAT csv, HEADER true)", t.Export))
		if cerr := zw.Close(); err == nil {
			err = cerr
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return counts, fmt.Errorf("demodata: export %s: %w", t.Name, err)
		}
		counts[t.Name] = tag.RowsAffected()
	}
	return counts, nil
}

// Import loads the dataset in dir. A missing file of an Optional table is
// skipped with a log line. The target tables must be empty, unless
// replace is true, which first empties all of them (TRUNCATE ... CASCADE).
// Everything happens in one transaction.
func Import(ctx context.Context, db *pgxpool.Pool, dir string, replace bool) (map[string]int64, error) {
	var load []Table // the tables that have a file
	for _, t := range Tables {
		_, err := os.Stat(path(dir, t))
		switch {
		case err == nil:
			load = append(load, t)
		case t.Optional && errors.Is(err, fs.ErrNotExist):
			slog.Info("demodata: no file for an optional table, skipping", "table", t.Name, "file", path(dir, t))
		default:
			return nil, fmt.Errorf("demodata: %w", err)
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if replace {
		names := make([]string, len(Tables))
		for i, t := range Tables {
			names[i] = t.Name
		}
		if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
			return nil, fmt.Errorf("demodata: truncate: %w", err)
		}
	} else {
		for _, t := range Tables {
			var any bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+t.Name+")").Scan(&any); err != nil {
				return nil, fmt.Errorf("demodata: check %s: %w", t.Name, err)
			}
			if any {
				return nil, fmt.Errorf("demodata: table %s is not empty; pass replace to wipe the market tables first", t.Name)
			}
		}
	}

	counts := map[string]int64{}
	for _, t := range load {
		n, err := importTable(ctx, tx.Conn().PgConn(), dir, t)
		if err != nil {
			return counts, fmt.Errorf("demodata: import %s: %w", t.Name, err)
		}
		counts[t.Name] = n
		if t.HasSerialID {
			q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', 'id'), GREATEST((SELECT COALESCE(max(id), 0) FROM %s), 1))", t.Name, t.Name)
			if _, err := tx.Exec(ctx, q); err != nil {
				return counts, fmt.Errorf("demodata: reset %s sequence: %w", t.Name, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return counts, err
	}
	return counts, nil
}

// importTable loads one table. The column list comes from the file's header, so
// the files stay valid if a table later gets a new nullable column.
func importTable(ctx context.Context, pg *pgconn.PgConn, dir string, t Table) (int64, error) {
	cols, err := header(path(dir, t))
	if err != nil {
		return 0, err
	}
	f, err := os.Open(path(dir, t))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	sql := fmt.Sprintf("COPY %s (%s) FROM STDIN WITH (FORMAT csv, HEADER true)", t.Name, strings.Join(cols, ", "))
	tag, err := pg.CopyFrom(ctx, zr, sql)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func header(file string) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	line, err := bufio.NewReader(zr).ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read header of %s: %w", file, err)
	}
	return csv.NewReader(strings.NewReader(line)).Read()
}
