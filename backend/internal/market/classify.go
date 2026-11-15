package market

import (
	"strings"

	"Finova/internal/datasource"
)

// Kind classifies what an option's underlying is. It matches the
// underlying_kind enum in the database.
type Kind string

const (
	KindStock Kind = "stock" // option on a single company's shares (اختیار سهام)
	KindFund  Kind = "fund"  // option on a tradable fund/ETF unit
	KindIndex Kind = "index" // cash-settled option on a market index
	KindOther Kind = "other" // unknown / unclassified
)

// Valid reports whether k is a recognised kind.
func (k Kind) Valid() bool {
	switch k {
	case KindStock, KindFund, KindIndex, KindOther:
		return true
	default:
		return false
	}
}

// fundSectorCode is TSETMC's cSecVal for "صندوق سرمايه گذاري قابل معامله"
// (tradable investment fund). Every fund/ETF underlying observed in the option
// market carries this code.
const fundSectorCode = "68"

// classifyUnderlying maps an underlying's instrument info to a Kind.
//
//   - A fund sector code (or a sector name containing "صندوق") => fund.
//   - Any other non-empty industry sector => stock (it's a listed company).
//   - No sector information (e.g. the identity lookup failed, or a future index
//     product) => other, so a good classification is never overwritten with a
//     guess downstream.
//
// Index products are not currently listed as option underlyings on TSETMC and
// carry no distinguishing sector here, so they fall through to other until a
// reliable signal exists.
func classifyUnderlying(info datasource.InstrumentInfo) Kind {
	code := strings.TrimSpace(info.SectorCode)
	name := info.SectorName

	if code == fundSectorCode || strings.Contains(name, "صندوق") {
		return KindFund
	}
	if code != "" {
		return KindStock
	}
	return KindOther
}
