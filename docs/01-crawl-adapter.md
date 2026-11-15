# Task: Live market data adapter (Go)

## Step 0: Find the project information yourself
Nothing has been filled in for you. Explore the repo before writing any code, and find:
- **The chosen data source** (name, base URL, endpoints, what each returns). Sprint 1 produced a decision or comparison document; look in `docs/`, the README, and any `*.md` mentioning TSETMC, API, data source, crawl or the Persian «منبع داده». Also check config files and `.env.example` for a base URL, and existing code or scripts that already call it.
- **Saved sample responses** from the source: look for JSON/HTML/CSV files under `docs/`, `testdata/`, `samples/` or similar.
- **The field mapping** from the source to our model, if Sprint 1 wrote one.
- **The API contract document**: look for `api.md`, `openapi.yaml`, `swagger.*` or a docs section listing endpoints with JSON examples.
- **The Go module path, folder layout, config and logging setup**: read `go.mod`, `cmd/`, `internal/`.
- **The database schema**: read the migrations folder, so the structs match the column names.

Then write a short summary of what you found, with file paths, before you start coding.

**If something is missing:**
- No data source decision or no endpoints found → stop and ask. Do not pick a data source yourself.
- No sample responses → call the source once, save real responses under `testdata/`, and use those.
- No API contract or schema → use the struct shapes in this brief and say so in your summary.

## Context
We are building a student project: an analytics site for Iranian stock options (TSE / Farabourse), similar to optionbaaz.ir. The backend is Go with PostgreSQL. A periodic collection service (owned by another teammate) will call this adapter every 30–60 seconds during market hours and write the results to the database. Nothing else in the codebase may call the external data source directly; everything goes through this adapter.

**Another teammate is blocked on this task.** Deliver it in this order:
1. The interface, the domain structs and a fake implementation. Commit and push these first, so the backend can start building against them.
2. The real implementation.
3. Tests.

## Before writing code
- Follow the repo's existing conventions for layout, naming, config, logging and error handling.
- If the repo already defines domain types for contracts or quotes, reuse them instead of creating duplicates.
- If another adapter or a source client already exists in the repo, extend it rather than starting a parallel one.

## What to build

### Package
`internal/source`, or wherever the existing layout puts external integrations.

### Interface
Adjust the names to the repo's style, but keep this shape:

```go
type Source interface {
    // All active option contracts with their latest quotes, plus quotes for
    // every underlying that has options. One call = one consistent snapshot.
    FetchMarket(ctx context.Context) (*MarketSnapshot, error)
}

type MarketSnapshot struct {
    FetchedAt   time.Time
    Contracts   []OptionContract
    Underlyings []UnderlyingQuote
}

type OptionContract struct {
    SourceID         string    // the source's own id for the instrument
    Symbol           string    // e.g. ضخود1234
    UnderlyingSymbol string    // e.g. خودرو
    Type             string    // "call" | "put"
    Strike           float64
    Expiry           time.Time // Gregorian date, Asia/Tehran
    ContractSize     int

    LastPrice     float64
    ClosePrice    float64
    PrevClose     float64
    BidPrice      float64
    BidVolume     int64
    AskPrice      float64
    AskVolume     int64
    Volume        int64
    Value         float64
    TradeCount    int64
    OpenInterest  int64
    LastTradeTime *time.Time
}

type UnderlyingQuote struct {
    SourceID   string
    Symbol     string
    Name       string
    LastPrice  float64
    ClosePrice float64
    PrevClose  float64
    Volume     int64
}
```

If the source needs several requests to build one snapshot (for example a list call plus a quotes call), do that inside `FetchMarket`, so callers make one call.

### Normalization
All of this happens inside the adapter. Callers only get clean data.
- **Persian text:** convert Arabic `ي`/`ك` to Persian `ی`/`ک`, trim whitespace, and remove zero-width characters from symbols and names.
- **Digits:** if the source returns Persian or Arabic digits, convert them to ASCII.
- **Dates:** convert Jalali (Persian calendar) dates to Gregorian `time.Time` in the `Asia/Tehran` time zone. Use a maintained library, for example `github.com/yaa110/go-persian-calendar`.
- **Contract details:** derive call/put, strike, expiry and underlying from the response fields. Parse them from the contract name only as a fallback. In Iran, call symbols start with `ض` and put symbols with `ط`.
- **Missing values:** a missing or empty numeric field becomes 0. A missing last trade time becomes `nil`.
- **Bad rows:** skip a malformed row and log a warning. Never fail the whole snapshot because of one row.

### Reliability
- **Timeout:** configurable per request, default 10s.
- **Retries:** retry on network errors and 5xx/429 responses, with exponential backoff (default 3 attempts). Do not retry on 4xx other than 429.
- **Rate limit:** configurable as requests per second. Respect the source's documented limit.
- **Context:** respect `ctx` cancellation everywhere.
- **Errors:** wrap errors with context about which request failed. Never panic.
- **Config:** base URL, timeout, retries and rate limit come from the existing config system, with sensible defaults.

### Fake implementation
`FakeSource` implements `Source` by loading a saved snapshot from `testdata/`. The backend teammate uses it to develop the collection service without hitting the network. Make it easy to construct, for example `source.NewFake("testdata/market_sample.json")`.

### Demo command
A small `cmd/fetchdemo` that calls the real source once and prints:
- the number of contracts and underlyings;
- 5 sample contracts;
- the time the call took.

## Tests
- Save real responses under `testdata/` and serve them with `httptest.Server`. Tests must pass with no internet.
- Cover parsing and normalization: Persian characters, Jalali to Gregorian conversion, call/put detection, empty fields, a malformed row being skipped.
- Cover retries: the first request fails with 500, the second succeeds.
- Cover a timeout and context cancellation.

## Done when
- `FetchMarket` returns real data from the live source, and `cmd/fetchdemo` shows a plausible number of contracts. There are roughly 1,500 to 2,500 active contracts.
- `go test ./...` passes with no network access.
- `FakeSource` is pushed and documented in a short README section or doc comment.
- The structs match the field names in the API contract document, or differences are noted in the README.

## Out of scope
- Writing to the database. That is the collection service's job.
- Scheduling or market-hours logic.
- Any calculations: IV, Greeks, Black-Scholes. A separate layer does those.
- Historical data. Note in the README whether the source offers past option prices; it decides a Sprint 3 task.
