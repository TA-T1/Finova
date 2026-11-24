package market

import (
	"testing"
	"time"
)

func TestBuildSymbolPageAggregatesOpenInterestAndCalculatesMaxPain(t *testing.T) {
	expiry := time.Date(2027, time.January, 20, 0, 0, 0, 0, time.UTC)
	nextExpiry := expiry.AddDate(0, 1, 0)
	price, previousClose := int64(120), int64(100)
	name := "Example"
	contracts := []symbolContractRow{
		{
			Underlying: Underlying{Symbol: "TEST", Name: &name, Kind: "stock"},
			Price:      &price, PrevClose: &previousClose,
			OptionType: "put", Strike: 110, Expiry: expiry, Size: 1000, OpenInterest: 20,
		},
		{
			Underlying: Underlying{Symbol: "TEST", Name: &name, Kind: "stock"},
			Price:      &price, PrevClose: &previousClose,
			OptionType: "call", Strike: 100, Expiry: expiry, Size: 1000, OpenInterest: 10,
		},
		{
			Underlying: Underlying{Symbol: "TEST", Name: &name, Kind: "stock"},
			Price:      &price, PrevClose: &previousClose,
			OptionType: "call", Strike: 105, Expiry: nextExpiry, Size: 1000, OpenInterest: 5,
		},
	}

	page := buildSymbolPage(contracts)
	if page.Price == nil || *page.Price != 120 || page.PriceChange == nil || *page.PriceChange != 20 {
		t.Fatalf("unexpected underlying price fields: %+v", page)
	}
	if page.PriceChangePercent == nil || *page.PriceChangePercent != 20 {
		t.Fatalf("price change percent = %v, want 20", page.PriceChangePercent)
	}
	if len(page.Expiries) != 2 || page.Expiries[0].ContractCount != 2 || page.Expiries[1].ContractCount != 1 {
		t.Fatalf("unexpected expiry summaries: %+v", page.Expiries)
	}
	if page.OpenInterest.Calls != 15 || page.OpenInterest.Puts != 20 || page.OpenInterest.PCR == nil || *page.OpenInterest.PCR != 20.0/15.0 {
		t.Fatalf("unexpected open-interest totals: %+v", page.OpenInterest)
	}
	if len(page.StrikeOpenInterest) != 3 || page.StrikeOpenInterest[0].StrikePrice != 100 || page.StrikeOpenInterest[1].StrikePrice != 105 || page.StrikeOpenInterest[2].StrikePrice != 110 {
		t.Fatalf("strike chart is not sorted: %+v", page.StrikeOpenInterest)
	}
	if page.MaxPain == nil || page.MaxPain.ExpiryDate != expiry.Format("2006-01-02") || page.MaxPain.StrikePrice != 110 || page.MaxPain.TotalPayout != 100000 {
		t.Fatalf("unexpected max pain: %+v", page.MaxPain)
	}
}

func TestBuildSymbolPageLeavesPCRNullWithoutCallOpenInterest(t *testing.T) {
	page := buildSymbolPage([]symbolContractRow{{
		Underlying:   Underlying{Symbol: "TEST", Kind: "stock"},
		OptionType:   "put",
		Strike:       100,
		Expiry:       time.Date(2027, time.January, 20, 0, 0, 0, 0, time.UTC),
		Size:         1000,
		OpenInterest: 10,
	}})
	if page.OpenInterest.PCR != nil {
		t.Fatalf("PCR = %v, want null when call OI is zero", *page.OpenInterest.PCR)
	}
}
