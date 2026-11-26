package snapshot

import (
	"context"
	"testing"
	"time"
)

func TestInsertMissingNeverOverwritesExistingRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)
	live := Row{TradeDate: "2026-11-15", ContractSymbol: "ضخود1", UnderlyingSymbol: "خودرو", OptionType: "call",
		Strike: 2000, Expiry: "2026-12-23", ClosePrice: 495, LastPrice: 500, BidPrice: 490, AskPrice: 510,
		OpenInterest: 777, CapturedAt: at}
	if _, err := upsert(ctx, db, []Row{live}); err != nil {
		t.Fatal(err)
	}

	hist := live
	hist.BidPrice, hist.AskPrice, hist.OpenInterest, hist.LastPrice = 0, 0, 0, 1
	other := hist
	other.TradeDate = "2026-11-14"
	n, err := InsertMissing(ctx, db, []Row{hist, other})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("inserted %d rows, want 1 (the one that did not exist)", n)
	}

	var bid, oi, last int64
	if err := db.QueryRow(ctx, `SELECT bid_price, open_interest, last_price FROM quote_snapshots
		WHERE trade_date = '2026-11-15' AND contract_symbol = $1`, live.ContractSymbol).Scan(&bid, &oi, &last); err != nil {
		t.Fatal(err)
	}
	if bid != 490 || oi != 777 || last != 500 {
		t.Errorf("existing row was overwritten: bid=%d oi=%d last=%d", bid, oi, last)
	}
	var total int
	db.QueryRow(ctx, `SELECT count(*) FROM quote_snapshots`).Scan(&total)
	if total != 2 {
		t.Errorf("rows = %d, want 2", total)
	}
}
