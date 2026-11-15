package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FakeSource is an offline MarketSource that serves a snapshot saved on disk.
// Use it to develop and test the collection service without network access:
//
//	src, err := datasource.NewFake("testdata/market_sample.json")
//	snap, err := src.FetchMarket(ctx)
//
// Every call returns a deep copy, so callers may mutate the result freely.
// SetError makes the next calls fail, to exercise error handling.
type FakeSource struct {
	mu   sync.Mutex
	data []byte // JSON of the snapshot; re-decoded per call to give a deep copy
	err  error
}

var _ MarketSource = (*FakeSource)(nil)

// NewFake loads a MarketSnapshot JSON file (the format of
// testdata/market_sample.json).
func NewFake(path string) (*FakeSource, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fake source: read %s: %w", path, err)
	}
	return NewFakeFromJSON(b)
}

// NewFakeFromJSON builds a FakeSource from raw snapshot JSON.
func NewFakeFromJSON(b []byte) (*FakeSource, error) {
	var probe MarketSnapshot
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("fake source: decode snapshot: %w", err)
	}
	return &FakeSource{data: b}, nil
}

// NewFakeFromSnapshot builds a FakeSource from an in-memory snapshot.
func NewFakeFromSnapshot(s *MarketSnapshot) (*FakeSource, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("fake source: encode snapshot: %w", err)
	}
	return &FakeSource{data: b}, nil
}

// SetError makes every following FetchMarket return err (nil clears it).
func (f *FakeSource) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// FetchMarket implements MarketSource.
func (f *FakeSource) FetchMarket(ctx context.Context) (*MarketSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	err, data := f.err, f.data
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	var s MarketSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("fake source: decode snapshot: %w", err)
	}
	return &s, nil
}
