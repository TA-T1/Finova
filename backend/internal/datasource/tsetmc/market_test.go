package tsetmc

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"Finova/internal/datasource"
)

const realSample = "../../../testdata/tsetmc_option_watch.json"

// serve returns a test server that answers the option-chain endpoint with
// body and every other path with 404 (so enrichment is skipped).
func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return serveRoutes(t, map[string][]byte{"/Instrument/GetInstrumentOptionMarketWatch/0": body})
}

// serveRoutes answers each path in routes with its body and everything else
// with 404. It records the requested paths in the returned *[]string.
func serveRoutes(t *testing.T, routes map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// rowJSON builds a chain response from rows (maps), starting from a valid base
// row that each test may mutate.
func chain(t *testing.T, rows ...map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"instrumentOptMarketWatch": rows})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func baseRow() map[string]any {
	return map[string]any{
		"insCode_C": "C1", "insCode_P": "P1", "uaInsCode": "U1",
		"lVal18AFC_C": "ضخود1001", "lVal18AFC_P": "طخود1001",
		"lVal30_C": "اختيارخ خودرو-2500-1405/08/27", "lVal30_P": "اختيارف خودرو-2500-1405/08/27",
		"lval30_UA": "خودرو", "contractSize": 1000, "strikePrice": 2500,
		"beginDate": "20260822", "endDate": "20261118",
		"pDrCotVal_C": 120, "pClosing_C": 118, "priceYesterday_C": 110,
		"pMeDem_C": 119, "qTitMeDem_C": 10, "pMeOf_C": 121, "qTitMeOf_C": 20,
		"qTotTran5J_C": 500, "qTotCap_C": 60000.5, "zTotTran_C": 12, "oP_C": 900,
		"pDrCotVal_P": 30, "pClosing_P": 31, "priceYesterday_P": 29,
		"pMeDem_P": 29, "qTitMeDem_P": 1, "pMeOf_P": 31, "qTitMeOf_P": 2,
		"qTotTran5J_P": 50, "qTotCap_P": 1500, "zTotTran_P": 3, "oP_P": 400,
		"pDrCotVal_UA": 2600, "pClosing_UA": 2590, "priceYesterday_UA": 2550,
	}
}

func fetch(t *testing.T, body []byte, opts ...Option) (*datasource.MarketSnapshot, error) {
	t.Helper()
	srv := serve(t, body)
	return newTestClient(srv, opts...).FetchMarket(context.Background())
}

func find(s *datasource.MarketSnapshot, symbol string) *datasource.OptionContract {
	for i := range s.Contracts {
		if s.Contracts[i].Symbol == symbol {
			return &s.Contracts[i]
		}
	}
	return nil
}

func TestFetchMarketRealSample(t *testing.T) {
	body, err := os.ReadFile(realSample)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := fetch(t, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Contracts) != 1800 || len(snap.Underlyings) != 28 {
		t.Fatalf("got %d contracts / %d underlyings, want 1800 / 28", len(snap.Contracts), len(snap.Underlyings))
	}

	call := find(snap, "ضهرم8030")
	if call == nil {
		t.Fatal("call ضهرم8030 not found")
	}
	wantExpiry := time.Date(2026, 11, 18, 0, 0, 0, 0, datasource.Tehran)
	want := datasource.OptionContract{
		SourceID: "61500711433740234", Symbol: "ضهرم8030", UnderlyingSymbol: "اهرم", Type: "call",
		Strike: 56000, Expiry: wantExpiry, ContractSize: 1000,
		LastPrice: 37851, ClosePrice: 36183, PrevClose: 33400,
		BidPrice: 37800, BidVolume: 16, AskPrice: 38000, AskVolume: 5,
		Volume: 24832, Value: 898503596000, TradeCount: 999, OpenInterest: 118111,
	}
	got := *call
	if !got.Expiry.Equal(wantExpiry) {
		t.Errorf("expiry = %v, want %v", got.Expiry, wantExpiry)
	}
	got.Expiry, want.Expiry = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("call mismatch:\n got %+v\nwant %+v", got, want)
	}
	if call.LastTradeTime != nil {
		t.Error("LastTradeTime should be nil: the source does not provide it")
	}

	put := find(snap, "طهرم8030")
	if put == nil || put.Type != "put" || put.SourceID != "58980125072230074" || put.LastPrice != 15 || put.OpenInterest != 132909 {
		t.Fatalf("bad put: %+v", put)
	}

	var under *datasource.UnderlyingQuote
	for i := range snap.Underlyings {
		if snap.Underlyings[i].Symbol == "اهرم" {
			under = &snap.Underlyings[i]
		}
	}
	if under == nil || under.SourceID != "17914401175772326" || under.LastPrice != 94568 ||
		under.ClosePrice != 93013 || under.PrevClose != 89216 {
		t.Fatalf("bad underlying: %+v", under)
	}

	// Invariants over the whole snapshot.
	ids := map[string]bool{}
	for _, c := range snap.Contracts {
		if ids[c.SourceID] {
			t.Fatalf("duplicate id %s", c.SourceID)
		}
		ids[c.SourceID] = true
		if strings.ContainsAny(c.Symbol+c.UnderlyingSymbol, "يك\u200c\u200b\u200d") || c.Symbol != strings.TrimSpace(c.Symbol) {
			t.Fatalf("un-normalized text: %q / %q", c.Symbol, c.UnderlyingSymbol)
		}
		wantPrefix := map[string]string{"call": "ض", "put": "ط"}[c.Type]
		if wantPrefix == "" || !strings.HasPrefix(c.Symbol, wantPrefix) {
			t.Fatalf("type/prefix mismatch: %+v", c)
		}
		if c.Strike <= 0 || c.Expiry.IsZero() || c.Expiry.Location().String() != "Asia/Tehran" {
			t.Fatalf("bad strike/expiry: %+v", c)
		}
	}
	for _, u := range snap.Underlyings {
		if strings.ContainsAny(u.Symbol, "يك") {
			t.Fatalf("un-normalized underlying %q", u.Symbol)
		}
	}
}

func TestPersianNormalization(t *testing.T) {
	r := baseRow()
	r["lVal18AFC_C"] = " ضفملي\u200c۱۰۰۱ " // Arabic ي, ZWNJ, Persian digits, spaces
	r["lVal18AFC_P"] = "طفملي1001"
	r["lval30_UA"] = "فملي\u200f"
	r["pDrCotVal_C"] = "۱٬۲۳۴" // number sent as a Persian string with separator
	snap, err := fetch(t, chain(t, r))
	if err != nil {
		t.Fatal(err)
	}
	c := find(snap, "ضفملی1001")
	if c == nil {
		t.Fatalf("symbol not normalized: %+v", snap.Contracts)
	}
	if c.UnderlyingSymbol != "فملی" || snap.Underlyings[0].Symbol != "فملی" {
		t.Errorf("underlying not normalized: %q / %q", c.UnderlyingSymbol, snap.Underlyings[0].Symbol)
	}
	if c.LastPrice != 1234 {
		t.Errorf("LastPrice = %v, want 1234", c.LastPrice)
	}
}

func TestCallPutDetection(t *testing.T) {
	snap, err := fetch(t, chain(t, baseRow()))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Contracts) != 2 || len(snap.Underlyings) != 1 {
		t.Fatalf("got %d contracts, %d underlyings", len(snap.Contracts), len(snap.Underlyings))
	}
	if c := find(snap, "ضخود1001"); c == nil || c.Type != "call" {
		t.Errorf("call wrong: %+v", c)
	}
	if p := find(snap, "طخود1001"); p == nil || p.Type != "put" {
		t.Errorf("put wrong: %+v", p)
	}
}

func TestOnlyOneSideListed(t *testing.T) {
	r := baseRow()
	r["insCode_P"] = ""
	snap, err := fetch(t, chain(t, r))
	if err != nil || len(snap.Contracts) != 1 || snap.Contracts[0].Type != "call" {
		t.Fatalf("err=%v contracts=%+v", err, snap)
	}
}

func TestJalaliNameFallback(t *testing.T) {
	r := baseRow()
	delete(r, "endDate")
	delete(r, "strikePrice")
	delete(r, "lval30_UA")
	snap, err := fetch(t, chain(t, r))
	if err != nil {
		t.Fatal(err)
	}
	c := find(snap, "ضخود1001")
	if c == nil {
		t.Fatal("contract skipped despite parsable name")
	}
	wantExpiry := time.Date(2026, 11, 18, 0, 0, 0, 0, datasource.Tehran) // 1405/08/27
	if c.UnderlyingSymbol != "خودرو" || c.Strike != 2500 || !c.Expiry.Equal(wantExpiry) {
		t.Errorf("fallback wrong: %+v", c)
	}
}

func TestStructuredFieldsBeatName(t *testing.T) {
	r := baseRow()
	r["lVal30_C"] = "اختيارخ دیگر-9999-1400/01/01"
	snap, err := fetch(t, chain(t, r))
	if err != nil {
		t.Fatal(err)
	}
	if c := find(snap, "ضخود1001"); c.Strike != 2500 || c.UnderlyingSymbol != "خودرو" || c.Expiry.Year() != 2026 {
		t.Errorf("name overrode structured fields: %+v", c)
	}
}

func TestEmptyFieldsBecomeZero(t *testing.T) {
	r := baseRow()
	r["pDrCotVal_C"] = ""     // empty string
	r["pMeDem_C"] = nil       // null
	delete(r, "qTotTran5J_C") // missing
	r["qTotCap_C"] = ""
	r["oP_C"] = "  "
	snap, err := fetch(t, chain(t, r))
	if err != nil {
		t.Fatal(err)
	}
	c := find(snap, "ضخود1001")
	if c == nil {
		t.Fatal("row wrongly skipped")
	}
	if c.LastPrice != 0 || c.BidPrice != 0 || c.Volume != 0 || c.Value != 0 || c.OpenInterest != 0 {
		t.Errorf("empty fields should be 0: %+v", c)
	}
	if c.LastTradeTime != nil {
		t.Error("LastTradeTime should be nil")
	}
	if c.AskPrice != 121 {
		t.Errorf("neighbouring fields damaged: %+v", c)
	}
}

func TestMalformedRowsAreSkippedWithWarning(t *testing.T) {
	good := baseRow()

	badType := baseRow() // strike is not a number -> decode error
	badType["insCode_C"], badType["insCode_P"] = "C2", "P2"
	badType["strikePrice"] = "abc"

	noUnder := baseRow() // no underlying id
	noUnder["insCode_C"], noUnder["insCode_P"] = "C3", "P3"
	noUnder["uaInsCode"] = ""

	noDate := baseRow() // no usable expiry anywhere
	noDate["insCode_C"], noDate["insCode_P"] = "C4", "P4"
	delete(noDate, "endDate")
	noDate["lVal30_C"], noDate["lVal30_P"] = "???", "???"

	noLegs := baseRow()
	noLegs["insCode_C"], noLegs["insCode_P"] = "", ""

	body := chain(t, badType, good, noUnder, noDate, noLegs)
	body = bytes.Replace(body, []byte(`]}`), []byte(`,"not an object",42]}`), 1) // rows that are not objects at all

	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	snap, err := fetch(t, body, WithLogger(log))
	if err != nil {
		t.Fatalf("one bad row must not fail the snapshot: %v", err)
	}
	if len(snap.Contracts) != 2 || snap.Contracts[0].SourceID != "C1" && snap.Contracts[1].SourceID != "C1" {
		t.Fatalf("want only the good row's 2 contracts, got %+v", snap.Contracts)
	}
	if n := strings.Count(logs.String(), "skipping malformed option row"); n != 6 {
		t.Errorf("logged %d skip warnings, want 6:\n%s", n, logs.String())
	}
}

func TestDuplicateUnderlyingsAreMerged(t *testing.T) {
	a, b := baseRow(), baseRow()
	b["insCode_C"], b["insCode_P"] = "C2", "P2"
	b["lVal18AFC_C"], b["lVal18AFC_P"] = "ضخود1002", "طخود1002"
	snap, err := fetch(t, chain(t, a, b))
	if err != nil || len(snap.Underlyings) != 1 || len(snap.Contracts) != 4 {
		t.Fatalf("err=%v %d underlyings, %d contracts", err, len(snap.Underlyings), len(snap.Contracts))
	}
}

func TestUnrecognisedResponsesAreErrors(t *testing.T) {
	for name, body := range map[string]string{
		"empty rows": `{"instrumentOptMarketWatch":[]}`,
		"wrong key":  `{"something":[]}`,
		"html":       `<html>blocked</html>`,
	} {
		if _, err := fetch(t, []byte(body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestFetchMarketRetriesThenSucceeds(t *testing.T) {
	body := chain(t, baseRow())
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Instrument/GetInstrumentOptionMarketWatch/0" {
			http.NotFound(w, r) // enrichment endpoints are not under test here
			return
		}
		hits++
		if hits == 1 {
			http.Error(w, "boom", 500)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	snap, err := newTestClient(srv).FetchMarket(context.Background())
	if err != nil || hits != 2 || len(snap.Contracts) != 2 {
		t.Fatalf("err=%v hits=%d snap=%+v", err, hits, snap)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const (
	realWatch = "../../../testdata/tsetmc_market_watch.json"
	realInfo  = "../../../testdata/tsetmc_closing_price_info.json"
	// وتجارت: the most traded underlying in the real capture, so the one the
	// adapter picks to read the trade date from.
	refID = "63917421733088077"
)

func realRoutes(t *testing.T) map[string][]byte {
	return map[string][]byte{
		"/Instrument/GetInstrumentOptionMarketWatch/0": readFile(t, realSample),
		"/ClosingPrice/GetMarketWatch":                 readFile(t, realWatch),
		"/ClosingPrice/GetClosingPriceInfo/" + refID:   readFile(t, realInfo),
	}
}

func TestEnrichmentFillsNamesVolumeAndTradeTimes(t *testing.T) {
	srv := serveRoutes(t, realRoutes(t))
	snap, err := newTestClient(srv).FetchMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	under := map[string]datasource.UnderlyingQuote{}
	for _, u := range snap.Underlyings {
		under[u.Symbol] = u
	}

	// Real values from the captures: full name normalized, volume from the watch.
	if u := under["اهرم"]; u.Name != "صندوق س سهامی کاریزما- اهرمی" || u.Volume != 356541417 {
		t.Errorf("اهرم: %+v", u)
	}
	// وبصادر is absent from the market watch (halted): fallback values.
	if u := under["وبصادر"]; u.Name != "وبصادر" || u.Volume != 0 {
		t.Errorf("وبصادر should keep Name=Symbol and Volume=0, got %+v", u)
	}

	// Traded contract: trade date 2026-11-15 (from the closing info) + hEven 123001.
	call := find(snap, "ضهرم8030")
	want := time.Date(2026, 11, 15, 12, 30, 1, 0, datasource.Tehran)
	if call.LastTradeTime == nil || !call.LastTradeTime.Equal(want) {
		t.Fatalf("LastTradeTime = %v, want %v", call.LastTradeTime, want)
	}

	// Every traded contract has a time on that date; no untraded one has any.
	withTime := 0
	for _, c := range snap.Contracts {
		if c.Volume > 0 {
			if c.LastTradeTime == nil {
				t.Fatalf("traded contract %s has no LastTradeTime", c.Symbol)
			}
			if d := c.LastTradeTime.In(datasource.Tehran).Format("2006-01-02"); d != "2026-11-15" {
				t.Fatalf("%s trade date %s", c.Symbol, d)
			}
			// One real contract (ضموج844) reports 14:19:20, after the close
			// (probably an exercise/settlement update), so only sanity-check.
			if h := c.LastTradeTime.In(datasource.Tehran).Hour(); h < 9 || h > 23 {
				t.Fatalf("%s trade hour %d is implausible", c.Symbol, h)
			}
			withTime++
		} else if c.LastTradeTime != nil {
			t.Fatalf("untraded contract %s has LastTradeTime %v", c.Symbol, c.LastTradeTime)
		}
	}
	if withTime < 500 {
		t.Errorf("only %d contracts got a trade time, expected most of the traded ones", withTime)
	}
}

func TestEnrichmentRequestsTheMostTradedUnderlyingForTheDate(t *testing.T) {
	var paths []string
	routes := realRoutes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if b, ok := routes[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	if _, err := newTestClient(srv).FetchMarket(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/Instrument/GetInstrumentOptionMarketWatch/0",
		"/ClosingPrice/GetMarketWatch",
		"/ClosingPrice/GetClosingPriceInfo/" + refID,
	}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %v, want %v", paths, want)
	}
}

func TestEnrichmentFailureStillReturnsTheChain(t *testing.T) {
	for name, drop := range map[string]string{
		"market watch down": "/ClosingPrice/GetMarketWatch",
		"closing info down": "/ClosingPrice/GetClosingPriceInfo/" + refID,
	} {
		routes := realRoutes(t)
		delete(routes, drop)
		var logs bytes.Buffer
		srv := serveRoutes(t, routes)
		snap, err := newTestClient(srv, WithLogger(slog.New(slog.NewTextHandler(&logs, nil)))).FetchMarket(context.Background())
		if err != nil || len(snap.Contracts) != 1800 || len(snap.Underlyings) != 28 {
			t.Fatalf("%s: err=%v snapshot=%v", name, err, snap)
		}
		if !strings.Contains(logs.String(), "enrichment failed") {
			t.Errorf("%s: no warning logged:\n%s", name, logs.String())
		}
		for _, c := range snap.Contracts {
			if c.LastTradeTime != nil {
				t.Fatalf("%s: unexpected LastTradeTime on %s", name, c.Symbol)
			}
		}
	}
	// With only the closing info down, names and volume still come from the watch.
	routes := realRoutes(t)
	delete(routes, "/ClosingPrice/GetClosingPriceInfo/"+refID)
	snap, _ := newTestClient(serveRoutes(t, routes), WithLogger(quietLogger())).FetchMarket(context.Background())
	for _, u := range snap.Underlyings {
		if u.Symbol == "اهرم" && u.Volume != 356541417 {
			t.Errorf("volume lost when only the closing info failed: %+v", u)
		}
	}
}

func TestTimeOnDate(t *testing.T) {
	d := time.Date(2026, 11, 15, 0, 0, 0, 0, datasource.Tehran)
	ok := map[int]time.Time{
		123001: time.Date(2026, 11, 15, 12, 30, 1, 0, datasource.Tehran),
		90000:  time.Date(2026, 11, 15, 9, 0, 0, 0, datasource.Tehran),
		122957: time.Date(2026, 11, 15, 12, 29, 57, 0, datasource.Tehran),
	}
	for in, want := range ok {
		if got, good := timeOnDate(d, in); !good || !got.Equal(want) {
			t.Errorf("timeOnDate(%d) = %v, %v; want %v", in, got, good, want)
		}
	}
	for _, bad := range []int{0, 61038, 80000, 245959, 126099, 123061, -5} {
		if got, good := timeOnDate(d, bad); good {
			t.Errorf("timeOnDate(%d) = %v, want rejected", bad, got)
		}
	}
}
