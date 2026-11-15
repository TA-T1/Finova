package datasource

import (
	"context"
	"errors"
	"testing"
)

func TestFakeSource(t *testing.T) {
	f, err := NewFake("../../testdata/market_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.FetchMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Contracts) == 0 || len(s.Underlyings) == 0 {
		t.Fatalf("empty snapshot: %d contracts, %d underlyings", len(s.Contracts), len(s.Underlyings))
	}
	s.Contracts[0].Symbol = "mutated"
	s2, _ := f.FetchMarket(context.Background())
	if s2.Contracts[0].Symbol == "mutated" {
		t.Fatal("snapshots share memory")
	}
	boom := errors.New("boom")
	f.SetError(boom)
	if _, err := f.FetchMarket(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}
