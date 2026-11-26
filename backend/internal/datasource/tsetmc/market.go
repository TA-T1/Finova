package tsetmc

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Finova/internal/datasource"
)

const (
	optionWatchPath = "/Instrument/GetInstrumentOptionMarketWatch/0"
	// Market-wide watch (every instrument, ~3.7 MB): full names, underlying
	// volume and per-instrument last-trade time (hEven).
	marketWatchPath = "/ClosingPrice/GetMarketWatch?market=0&industrialGroup=" +
		"&paperTypes[0]=1&paperTypes[1]=2&paperTypes[2]=3&paperTypes[3]=4&paperTypes[4]=5" +
		"&paperTypes[5]=6&paperTypes[6]=7&paperTypes[7]=8&paperTypes[8]=9" +
		"&showTraded=false&withBestLimits=false&hEven=0&RefID=0"
	// Per-instrument closing info: the only place that carries the trade DATE.
	closingInfoPath = "/ClosingPrice/GetClosingPriceInfo/"
)

// contractNameRe parses names like "اختیارخ اهرم-56000-1405/08/27" (kind +
// underlying, strike, Jalali expiry). Used only as a fallback when the
// structured fields are missing.
var contractNameRe = regexp.MustCompile(`^\S+\s+(.+)-(\d+(?:\.\d+)?)-(\d{4}/\d{1,2}/\d{1,2})$`)

// FetchMarket implements datasource.MarketSource. The whole chain comes from a
// single TSETMC call (GetInstrumentOptionMarketWatch), which already carries
// each underlying's quote, so one call is one consistent snapshot.
//
// A malformed row is skipped with a warning; only a failure to fetch or to
// recognise the response at all returns an error.
func (c *Client) FetchMarket(ctx context.Context) (*datasource.MarketSnapshot, error) {
	b, err := c.getBytes(ctx, optionWatchPath)
	if err != nil {
		return nil, fmt.Errorf("fetch market: %w", err)
	}
	var resp struct {
		Rows []json.RawMessage `json:"instrumentOptMarketWatch"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("fetch market: decode %s: %w", optionWatchPath, err)
	}
	if len(resp.Rows) == 0 {
		// An empty chain is indistinguishable from a changed/blocked response;
		// failing is safer than silently reporting "no contracts".
		return nil, fmt.Errorf("fetch market: %s returned no rows", optionWatchPath)
	}

	snap := &datasource.MarketSnapshot{
		FetchedAt:   time.Now().In(datasource.Tehran),
		Contracts:   make([]datasource.OptionContract, 0, 2*len(resp.Rows)),
		Underlyings: []datasource.UnderlyingQuote{},
	}
	seenContract := map[string]bool{}
	seenUnder := map[string]bool{}
	skipped := 0

	for i, raw := range resp.Rows {
		contracts, under, err := c.parseRow(raw)
		if err != nil {
			skipped++
			c.log.Warn("tsetmc: skipping malformed option row", "row", i, "err", err)
			continue
		}
		if !seenUnder[under.SourceID] {
			seenUnder[under.SourceID] = true
			snap.Underlyings = append(snap.Underlyings, under)
		}
		for _, oc := range contracts {
			if seenContract[oc.SourceID] {
				c.log.Warn("tsetmc: duplicate contract skipped", "row", i, "symbol", oc.Symbol)
				continue
			}
			seenContract[oc.SourceID] = true
			snap.Contracts = append(snap.Contracts, oc)
		}
	}
	if len(snap.Contracts) == 0 {
		return nil, fmt.Errorf("fetch market: no valid contracts in %d rows", len(resp.Rows))
	}
	if skipped > 0 {
		c.log.Warn("tsetmc: some rows were skipped", "skipped", skipped, "total", len(resp.Rows))
	}
	// Best effort: the chain alone is a complete, valid snapshot.
	if err := c.enrich(ctx, snap); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("fetch market: %w", ctx.Err())
		}
		c.log.Warn("tsetmc: enrichment failed, returning the snapshot without it "+
			"(underlying names/volume and last trade times stay unset)", "err", err)
	}
	return snap, nil
}

// watchRow is the part of a GetMarketWatch row we use.
type watchRow struct {
	InsCode string    `json:"insCode"`
	Name    string    `json:"lvc"`   // full name
	Volume  flexFloat `json:"qtj"`   // traded quantity today
	HEven   flexInt   `json:"hEven"` // HHMMSS of the last trade/update; date is not included
}

// enrich fills what the option-chain endpoint lacks, using two more requests:
//
//   - GetMarketWatch gives each underlying's full name and volume, and every
//     instrument's last-trade time of day (hEven, HHMMSS).
//   - hEven has no date, so one GetClosingPriceInfo call for the most traded
//     underlying gives the date of the latest trading session (dEven).
//     A contract with volume > 0 traded in that session, so its
//     LastTradeTime is that date plus its hEven. Contracts that did not trade
//     keep nil (their hEven is just a pre-open marker such as 06:10:38).
//
// Whatever can be filled is filled; the first problem is returned.
func (c *Client) enrich(ctx context.Context, snap *datasource.MarketSnapshot) error {
	b, err := c.getBytes(ctx, marketWatchPath)
	if err != nil {
		return fmt.Errorf("market watch: %w", err)
	}
	var resp struct {
		Rows []json.RawMessage `json:"marketwatch"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return fmt.Errorf("market watch: decode: %w", err)
	}
	watch := make(map[string]watchRow, len(resp.Rows))
	for _, raw := range resp.Rows {
		var w watchRow
		if err := json.Unmarshal(raw, &w); err != nil {
			continue // one odd row must not lose the others
		}
		if w.InsCode = datasource.NormalizeText(w.InsCode); w.InsCode != "" {
			watch[w.InsCode] = w
		}
	}
	if len(watch) == 0 {
		return fmt.Errorf("market watch: no rows")
	}

	// Underlying names and volume. An underlying missing from the watch (for
	// example a halted one) keeps Name = Symbol and Volume = 0.
	refID, refVol := "", 0.0
	for i := range snap.Underlyings {
		u := &snap.Underlyings[i]
		w, ok := watch[u.SourceID]
		if !ok {
			continue
		}
		if name := datasource.NormalizeText(w.Name); name != "" {
			u.Name = name
		}
		u.Volume = int64(w.Volume)
		if float64(w.Volume) > refVol {
			refID, refVol = u.SourceID, float64(w.Volume)
		}
	}
	if refID == "" {
		return fmt.Errorf("no underlying with volume to take the trade date from")
	}

	date, err := c.lastSessionDate(ctx, refID)
	if err != nil {
		return err
	}
	for i := range snap.Contracts {
		oc := &snap.Contracts[i]
		w, ok := watch[oc.SourceID]
		if !ok || oc.Volume <= 0 {
			continue
		}
		if t, ok := timeOnDate(date, int(w.HEven)); ok {
			oc.LastTradeTime = &t
		}
	}
	return nil
}

// lastSessionDate returns the Tehran date (00:00) of the latest trading
// session, from the closing info of one liquid instrument.
func (c *Client) lastSessionDate(ctx context.Context, insCode string) (time.Time, error) {
	var resp struct {
		Info struct {
			DEven         flexInt `json:"dEven"`
			FinalLastDate flexInt `json:"finalLastDate"`
		} `json:"closingPriceInfo"`
	}
	if err := c.getJSON(ctx, closingInfoPath+insCode, &resp); err != nil {
		return time.Time{}, fmt.Errorf("closing info %s: %w", insCode, err)
	}
	for _, v := range []flexInt{resp.Info.DEven, resp.Info.FinalLastDate} {
		if v > 0 {
			if t, err := datasource.ParseGregorianYMD(strconv.FormatInt(int64(v), 10)); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("closing info %s: no usable date (dEven=%d, finalLastDate=%d)",
		insCode, resp.Info.DEven, resp.Info.FinalLastDate)
}

// timeOnDate combines a Tehran date with a TSETMC HHMMSS integer (123001 is
// 12:30:01). It rejects invalid values and times before 09:00, which TSETMC
// uses as pre-open markers (e.g. 061038) for instruments that did not trade.
func timeOnDate(date time.Time, hhmmss int) (time.Time, bool) {
	h, m, s := hhmmss/10000, hhmmss/100%100, hhmmss%100
	if h < 9 || h > 23 || m > 59 || s > 59 {
		return time.Time{}, false
	}
	return time.Date(date.Year(), date.Month(), date.Day(), h, m, s, 0, datasource.Tehran), true
}

// parseRow turns one chain row (call + put + underlying) into contracts and the
// underlying quote. It returns an error if the row is unusable.
func (c *Client) parseRow(raw json.RawMessage) ([]datasource.OptionContract, datasource.UnderlyingQuote, error) {
	var r optionWatchRow
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, datasource.UnderlyingQuote{}, fmt.Errorf("decode row: %w", err)
	}

	callSym := datasource.NormalizeText(r.CSymbol)
	putSym := datasource.NormalizeText(r.PSymbol)
	callName := datasource.NormalizeText(r.CName)
	putName := datasource.NormalizeText(r.PName)
	callID := datasource.NormalizeText(r.CInsCode)
	putID := datasource.NormalizeText(r.PInsCode)

	// Structured fields first; the contract name is only a fallback.
	underSym := datasource.NormalizeText(r.UASymbol)
	strike := float64(r.StrikePrice)
	var expiry time.Time
	if r.EndDate != "" {
		if t, err := datasource.ParseGregorianYMD(r.EndDate); err == nil {
			expiry = t
		}
	}
	if underSym == "" || strike <= 0 || expiry.IsZero() {
		for _, name := range []string{callName, putName} {
			nu, ns, ne, ok := parseContractName(name)
			if !ok {
				continue
			}
			if underSym == "" {
				underSym = nu
			}
			if strike <= 0 {
				strike = ns
			}
			if expiry.IsZero() {
				expiry = ne
			}
			break
		}
	}

	underID := datasource.NormalizeText(r.UAInsCode)
	switch {
	case underID == "":
		return nil, datasource.UnderlyingQuote{}, fmt.Errorf("missing underlying id (symbol %q)", underSym)
	case underSym == "":
		return nil, datasource.UnderlyingQuote{}, fmt.Errorf("missing underlying symbol (id %s)", underID)
	case strike <= 0:
		return nil, datasource.UnderlyingQuote{}, fmt.Errorf("missing strike (%s)", firstNonEmpty(callSym, putSym))
	case expiry.IsZero():
		return nil, datasource.UnderlyingQuote{}, fmt.Errorf("missing expiry (%s)", firstNonEmpty(callSym, putSym))
	}

	base := datasource.OptionContract{
		UnderlyingSymbol: underSym,
		Strike:           strike,
		Expiry:           expiry,
		ContractSize:     int(r.ContractSize),
	}
	var out []datasource.OptionContract
	if callID != "" && callSym != "" {
		oc := base
		oc.SourceID, oc.Symbol, oc.Type = callID, callSym, string(datasource.Call)
		oc.LastPrice, oc.ClosePrice, oc.PrevClose = float64(r.CLast), float64(r.CClosing), float64(r.CYesterday)
		oc.BidPrice, oc.BidVolume = float64(r.CBidPrice), int64(r.CBidVol)
		oc.AskPrice, oc.AskVolume = float64(r.CAskPrice), int64(r.CAskVol)
		oc.Volume, oc.Value, oc.TradeCount = int64(r.CVolume), float64(r.CValue), int64(r.CTrades)
		oc.OpenInterest = int64(r.COP)
		out = append(out, oc)
		c.checkPrefix(oc.Symbol, "ض")
	}
	if putID != "" && putSym != "" {
		oc := base
		oc.SourceID, oc.Symbol, oc.Type = putID, putSym, string(datasource.Put)
		oc.LastPrice, oc.ClosePrice, oc.PrevClose = float64(r.PLast), float64(r.PClosing), float64(r.PYesterday)
		oc.BidPrice, oc.BidVolume = float64(r.PBidPrice), int64(r.PBidVol)
		oc.AskPrice, oc.AskVolume = float64(r.PAskPrice), int64(r.PAskVol)
		oc.Volume, oc.Value, oc.TradeCount = int64(r.PVolume), float64(r.PValue), int64(r.PTrades)
		oc.OpenInterest = int64(r.POP)
		out = append(out, oc)
		c.checkPrefix(oc.Symbol, "ط")
	}
	if len(out) == 0 {
		return nil, datasource.UnderlyingQuote{}, fmt.Errorf("row has neither a call nor a put (underlying %s)", underSym)
	}

	// This endpoint gives the underlying's short symbol but no full name and no
	// volume, so Name falls back to the symbol and Volume stays 0.
	under := datasource.UnderlyingQuote{
		SourceID:   underID,
		Symbol:     underSym,
		Name:       underSym,
		LastPrice:  float64(r.UALast),
		ClosePrice: float64(r.UAClosing),
		PrevClose:  float64(r.UAYesterday),
	}
	return out, under, nil
}

// checkPrefix warns when a symbol disagrees with Iran's naming rule (calls
// start with ض, puts with ط). The side of the response stays authoritative.
func (c *Client) checkPrefix(symbol, want string) {
	if !strings.HasPrefix(symbol, want) {
		c.log.Warn("tsetmc: contract symbol prefix does not match its side", "symbol", symbol, "want_prefix", want)
	}
}

// parseContractName extracts underlying, strike and expiry from a contract
// name such as "اختیارخ اهرم-56000-1405/08/27".
func parseContractName(name string) (underlying string, strike float64, expiry time.Time, ok bool) {
	m := contractNameRe.FindStringSubmatch(datasource.NormalizeText(name))
	if m == nil {
		return "", 0, time.Time{}, false
	}
	k, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return "", 0, time.Time{}, false
	}
	e, err := datasource.ParseJalali(m[3])
	if err != nil {
		return "", 0, time.Time{}, false
	}
	return strings.TrimSpace(m[1]), k, e, true
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

var _ datasource.MarketSource = (*Client)(nil)
