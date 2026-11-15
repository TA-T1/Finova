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

const optionWatchPath = "/Instrument/GetInstrumentOptionMarketWatch/0"

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
	return snap, nil
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
