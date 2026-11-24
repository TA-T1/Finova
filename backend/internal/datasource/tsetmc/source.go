package tsetmc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"Finova/internal/datasource"
)

// numString turns a raw JSON scalar into a clean numeric string: quotes are
// removed, Persian/Arabic digits become ASCII and thousands separators go. An
// empty result means "missing" (empty string or null).
func numString(b []byte) string {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "null" {
		return ""
	}
	return strings.ReplaceAll(datasource.NormalizeText(s), ",", "")
}

// flexInt decodes a JSON number (integer or float) or a numeric string into an
// int64. TSETMC is mostly consistent but occasionally wraps numbers in strings
// or returns settlement values with a trailing ".00"; this absorbs both. A
// missing, null or empty value is 0.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := numString(b)
	if s == "" {
		*f = 0
		return nil
	}
	if strings.ContainsAny(s, ".eE") {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*f = flexInt(int64(v))
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(v)
	return nil
}

// flexFloat is flexInt for values that may be fractional (e.g. traded value).
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := numString(b)
	if s == "" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

// parseYMD parses a Gregorian YYYYMMDD value (TSETMC's dEven / beginDate /
// endDate) into a UTC date. Returns the zero time for an unparseable value.
func parseYMD(v string) time.Time {
	v = strings.TrimSpace(v)
	if len(v) != 8 {
		return time.Time{}
	}
	y, err1 := strconv.Atoi(v[0:4])
	m, err2 := strconv.Atoi(v[4:6])
	d, err3 := strconv.Atoi(v[6:8])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// --- Option market watch -------------------------------------------------

type optionWatchResponse struct {
	Rows []optionWatchRow `json:"instrumentOptMarketWatch"`
}

// optionWatchRow mirrors one element of GetInstrumentOptionMarketWatch. Each
// row carries both the call (_C) and put (_P) sides for a single strike/expiry
// plus a snapshot of the underlying (_UA).
type optionWatchRow struct {
	ContractSize flexInt `json:"contractSize"`
	UAInsCode    string  `json:"uaInsCode"`
	StrikePrice  flexInt `json:"strikePrice"`
	BeginDate    string  `json:"beginDate"`
	EndDate      string  `json:"endDate"`
	RemainedDay  flexInt `json:"remainedDay"`

	// Underlying snapshot.
	UASymbol    string  `json:"lval30_UA"`
	UAClosing   flexInt `json:"pClosing_UA"`
	UALast      flexInt `json:"pDrCotVal_UA"`
	UAYesterday flexInt `json:"priceYesterday_UA"`

	// Call side.
	CInsCode   string    `json:"insCode_C"`
	CSymbol    string    `json:"lVal18AFC_C"`
	CName      string    `json:"lVal30_C"`
	CLast      flexInt   `json:"pDrCotVal_C"`
	CClosing   flexInt   `json:"pClosing_C"`
	CYesterday flexInt   `json:"priceYesterday_C"`
	COP        flexInt   `json:"oP_C"`
	CVolume    flexInt   `json:"qTotTran5J_C"`
	CValue     flexFloat `json:"qTotCap_C"`
	CBidPrice  flexInt   `json:"pMeDem_C"`
	CBidVol    flexInt   `json:"qTitMeDem_C"`
	CAskPrice  flexInt   `json:"pMeOf_C"`
	CAskVol    flexInt   `json:"qTitMeOf_C"`
	CTrades    flexInt   `json:"zTotTran_C"`

	// Put side.
	PInsCode   string    `json:"insCode_P"`
	PSymbol    string    `json:"lVal18AFC_P"`
	PName      string    `json:"lVal30_P"`
	PLast      flexInt   `json:"pDrCotVal_P"`
	PClosing   flexInt   `json:"pClosing_P"`
	PYesterday flexInt   `json:"priceYesterday_P"`
	POP        flexInt   `json:"oP_P"`
	PVolume    flexInt   `json:"qTotTran5J_P"`
	PValue     flexFloat `json:"qTotCap_P"`
	PBidPrice  flexInt   `json:"pMeDem_P"`
	PBidVol    flexInt   `json:"qTitMeDem_P"`
	PAskPrice  flexInt   `json:"pMeOf_P"`
	PAskVol    flexInt   `json:"qTitMeOf_P"`
	PTrades    flexInt   `json:"zTotTran_P"`
}

// OptionMarketWatch implements datasource.Source.
func (c *Client) OptionMarketWatch(ctx context.Context) ([]datasource.OptionRow, error) {
	var resp optionWatchResponse
	if err := c.getJSON(ctx, "/Instrument/GetInstrumentOptionMarketWatch/0", &resp); err != nil {
		return nil, err
	}

	rows := make([]datasource.OptionRow, 0, len(resp.Rows))
	for _, r := range resp.Rows {
		row := datasource.OptionRow{
			UnderlyingInsCode:   strings.TrimSpace(r.UAInsCode),
			UnderlyingSymbol:    strings.TrimSpace(r.UASymbol),
			UnderlyingLast:      int64(r.UALast),
			UnderlyingClose:     int64(r.UAClosing),
			UnderlyingYesterday: int64(r.UAYesterday),
			StrikePrice:         int64(r.StrikePrice),
			BeginDate:           parseYMD(r.BeginDate),
			ExpiryDate:          parseYMD(r.EndDate),
			RemainedDays:        int(r.RemainedDay),
			ContractSize:        int(r.ContractSize),
		}
		if code := strings.TrimSpace(r.CInsCode); code != "" {
			row.Call = &datasource.OptionLeg{
				InsCode:      code,
				Symbol:       strings.TrimSpace(r.CSymbol),
				Name:         strings.TrimSpace(r.CName),
				Type:         datasource.Call,
				Last:         int64(r.CLast),
				Close:        int64(r.CClosing),
				Yesterday:    int64(r.CYesterday),
				BestBid:      int64(r.CBidPrice),
				BestAsk:      int64(r.CAskPrice),
				BidVolume:    int64(r.CBidVol),
				AskVolume:    int64(r.CAskVol),
				Volume:       int64(r.CVolume),
				Value:        int64(r.CValue),
				OpenInterest: int64(r.COP),
			}
		}
		if code := strings.TrimSpace(r.PInsCode); code != "" {
			row.Put = &datasource.OptionLeg{
				InsCode:      code,
				Symbol:       strings.TrimSpace(r.PSymbol),
				Name:         strings.TrimSpace(r.PName),
				Type:         datasource.Put,
				Last:         int64(r.PLast),
				Close:        int64(r.PClosing),
				Yesterday:    int64(r.PYesterday),
				BestBid:      int64(r.PBidPrice),
				BestAsk:      int64(r.PAskPrice),
				BidVolume:    int64(r.PBidVol),
				AskVolume:    int64(r.PAskVol),
				Volume:       int64(r.PVolume),
				Value:        int64(r.PValue),
				OpenInterest: int64(r.POP),
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// --- Daily price history -------------------------------------------------

type dailyListResponse struct {
	Daily []dailyPriceRow `json:"closingPriceDaily"`
}

type dailyPriceRow struct {
	DEven      flexInt `json:"dEven"`
	PriceFirst flexInt `json:"priceFirst"`
	PriceMax   flexInt `json:"priceMax"`
	PriceMin   flexInt `json:"priceMin"`
	PClosing   flexInt `json:"pClosing"`
	PDrCotVal  flexInt `json:"pDrCotVal"`
	Volume     flexInt `json:"qTotTran5J"`
	Value      flexInt `json:"qTotCap"`
	TradeCount flexInt `json:"zTotTran"`
}

// DailyPrices implements datasource.Source.
func (c *Client) DailyPrices(ctx context.Context, insCode string, adjusted bool) ([]datasource.DailyPrice, error) {
	insCode = strings.TrimSpace(insCode)
	if insCode == "" {
		return nil, fmt.Errorf("tsetmc: empty insCode")
	}
	adj := "0"
	if adjusted {
		adj = "1"
	}
	var resp dailyListResponse
	path := fmt.Sprintf("/ClosingPrice/GetClosingPriceDailyList/%s/%s", insCode, adj)
	if err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}

	out := make([]datasource.DailyPrice, 0, len(resp.Daily))
	for _, d := range resp.Daily {
		date := parseYMD(strconv.FormatInt(int64(d.DEven), 10))
		if date.IsZero() {
			continue
		}
		out = append(out, datasource.DailyPrice{
			Date:       date,
			Open:       int64(d.PriceFirst),
			High:       int64(d.PriceMax),
			Low:        int64(d.PriceMin),
			Close:      int64(d.PDrCotVal),
			Final:      int64(d.PClosing),
			Volume:     int64(d.Volume),
			Value:      int64(d.Value),
			TradeCount: int64(d.TradeCount),
		})
	}
	return out, nil
}

// --- Instrument identity -------------------------------------------------

type instrumentInfoResponse struct {
	InstrumentInfo struct {
		InsCode      string  `json:"insCode"`
		Symbol       string  `json:"lVal18AFC"`
		Name         string  `json:"lVal30"`
		ISIN         string  `json:"cIsin"`
		ContractSize flexInt `json:"contractSize"`
		Sector       struct {
			CSecVal string `json:"cSecVal"`
			LSecVal string `json:"lSecVal"`
		} `json:"sector"`
	} `json:"instrumentInfo"`
}

// InstrumentInfo implements datasource.Source.
func (c *Client) InstrumentInfo(ctx context.Context, insCode string) (*datasource.InstrumentInfo, error) {
	insCode = strings.TrimSpace(insCode)
	if insCode == "" {
		return nil, fmt.Errorf("tsetmc: empty insCode")
	}
	var resp instrumentInfoResponse
	if err := c.getJSON(ctx, "/Instrument/GetInstrumentInfo/"+insCode, &resp); err != nil {
		return nil, err
	}
	ii := resp.InstrumentInfo
	code := strings.TrimSpace(ii.InsCode)
	if code == "" {
		code = insCode
	}
	return &datasource.InstrumentInfo{
		InsCode:      code,
		Symbol:       strings.TrimSpace(ii.Symbol),
		Name:         strings.TrimSpace(ii.Name),
		ISIN:         strings.TrimSpace(ii.ISIN),
		ContractSize: int(ii.ContractSize),
		SectorCode:   strings.TrimSpace(ii.Sector.CSecVal),
		SectorName:   strings.TrimSpace(ii.Sector.LSecVal),
	}, nil
}

// Ensure *Client satisfies the interface at compile time.
var _ datasource.Source = (*Client)(nil)
