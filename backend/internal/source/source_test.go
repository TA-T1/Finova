package source_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Finova/internal/datasource"
	"Finova/internal/datasource/tsetmc"
	"Finova/internal/source"
)

// Both implementations satisfy the brief's interface, and it is the very same
// type as the datasource one, so code written against either name compiles
// with the other.
var (
	_ source.Source           = (*tsetmc.Client)(nil)
	_ source.Source           = (*source.FakeSource)(nil)
	_ datasource.MarketSource = source.Source(nil)
)

func TestNewFakeServesTheFixture(t *testing.T) {
	f, err := source.NewFake("../../testdata/market_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var s source.Source = f
	snap, err := s.FetchMarket(t.Context())
	if err != nil || len(snap.Contracts) == 0 || len(snap.Underlyings) == 0 {
		t.Fatalf("snap=%v err=%v", snap, err)
	}
}

// "Nothing else in the codebase may call the external data source directly."
// Only the composition roots (cmd/...) may import the TSETMC client, to build
// it; library packages must depend on the interfaces. No file outside the
// adapter may mention the TSETMC hosts either.
func TestOnlyTheAdapterAndCommandsTouchTSETMC(t *testing.T) {
	root := ".." // backend/internal
	fset := token.NewFileSet()
	check := func(dir string, allowImport bool) {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			rel := filepath.ToSlash(path)
			if strings.Contains(rel, "/datasource/") || strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, "config/config.go") {
				return nil // the adapter itself; tests; config only documents the env var
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(src), "tsetmc.com") {
				t.Errorf("%s mentions a TSETMC host; go through the adapter", rel)
			}
			f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				if strings.Trim(imp.Path.Value, `"`) == "Finova/internal/datasource/tsetmc" && !allowImport {
					t.Errorf("%s imports the TSETMC client; only cmd/... may build it, libraries take a source.Source", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(root, false)
	check("../../cmd", true)
}
