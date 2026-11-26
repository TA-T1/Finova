package market

import (
	"context"
	"testing"
	"time"
)

func TestBuildMetricInputsUsesCloseFallbackAndSkipsInvalidContracts(t *testing.T) {
	quoteTS := time.Date(2026, time.November, 23, 8, 0, 0, 0, time.UTC)
	zero := int64(0)
	closePrice := int64(120)
	strike := int64(100)
	quotes := []LatestMetricQuote{
		{
			ContractID: 1, UnderlyingID: 10, OptionType: "call", StrikePrice: strike,
			ExpiryDate: quoteTS.AddDate(0, 0, 1), QuoteTS: &quoteTS,
			UnderlyingLastPrice: &zero, UnderlyingClosePrice: &closePrice,
		},
		{
			ContractID: 2, UnderlyingID: 10, OptionType: "call", StrikePrice: 0,
			ExpiryDate: quoteTS.AddDate(0, 0, 1), QuoteTS: &quoteTS,
			UnderlyingLastPrice: &closePrice,
		},
		{ContractID: 3, OptionType: "put", StrikePrice: strike},
	}

	inputs, skipped := BuildMetricInputs(quotes, nil)
	if len(inputs) != 1 {
		t.Fatalf("built %d inputs, want 1", len(inputs))
	}
	if inputs[0].Spot != float64(closePrice) {
		t.Errorf("spot = %g, want underlying close %d", inputs[0].Spot, closePrice)
	}
	if !inputs[0].TS.Equal(quoteTS) {
		t.Errorf("timestamp = %s, want quote timestamp %s", inputs[0].TS, quoteTS)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped %d contracts, want 2", len(skipped))
	}
	if skipped[0].ContractID != 2 || skipped[0].Reason != "strike price is not positive" {
		t.Errorf("zero-strike skip = %+v", skipped[0])
	}
	if skipped[1].ContractID != 3 || skipped[1].Reason != "no option quote" {
		t.Errorf("missing-quote skip = %+v", skipped[1])
	}
}

func TestBuildMetricInputsTimestampOverride(t *testing.T) {
	quoteTS := time.Date(2026, time.November, 23, 8, 0, 0, 0, time.UTC)
	override := quoteTS.Add(2 * time.Hour)
	price := int64(100)
	quote := LatestMetricQuote{
		ContractID: 1, UnderlyingID: 10, OptionType: "put", StrikePrice: 100,
		ExpiryDate: quoteTS.AddDate(0, 0, 1), QuoteTS: &quoteTS,
		UnderlyingLastPrice: &price,
	}

	inputs, skipped := BuildMetricInputs([]LatestMetricQuote{quote}, &override)
	if len(skipped) != 0 || len(inputs) != 1 {
		t.Fatalf("inputs=%d skipped=%v, want one input and no skips", len(inputs), skipped)
	}
	if !inputs[0].TS.Equal(override) {
		t.Errorf("timestamp = %s, want override %s", inputs[0].TS, override)
	}
}

func TestLatestQuotesForMetricsAndComputeAndStore(t *testing.T) {
	db := newMarketTestDB(t, true)
	ids, _, _ := seedMarketTestData(t, db)
	ctx := context.Background()
	repo := NewRepository(db)

	quotes, err := repo.LatestQuotesForMetrics(ctx)
	if err != nil {
		t.Fatalf("LatestQuotesForMetrics: %v", err)
	}
	if len(quotes) != 4 {
		t.Fatalf("loaded %d active contracts, want 4", len(quotes))
	}
	for _, quote := range quotes {
		if quote.ContractID == ids.expiredCall {
			t.Errorf("expired contract %d was returned", ids.expiredCall)
		}
	}

	inputs, skipped := BuildMetricInputs(quotes, nil)
	if len(inputs) != 3 || len(skipped) != 1 {
		t.Fatalf("built %d inputs and skipped %d, want 3 and 1", len(inputs), len(skipped))
	}
	if skipped[0].ContractID != ids.bbbCallTomorrow || skipped[0].Reason != "underlying spot is not positive" {
		t.Errorf("missing-underlying skip = %+v", skipped[0])
	}

	var before int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM option_metrics`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	dry, err := ComputeAndStoreMetrics(ctx, repo, inputs, DefaultRiskFreeRate, true, nil)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Calculated != 3 || dry.Stored != 0 {
		t.Errorf("dry-run result = %+v, want calculated=3 stored=0", dry)
	}
	var afterDryRun int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM option_metrics`).Scan(&afterDryRun); err != nil {
		t.Fatal(err)
	}
	if afterDryRun != before {
		t.Errorf("dry-run changed option_metrics count from %d to %d", before, afterDryRun)
	}

	first, err := ComputeAndStoreMetrics(ctx, repo, inputs, DefaultRiskFreeRate, false, nil)
	if err != nil {
		t.Fatalf("store metrics: %v", err)
	}
	if first.Calculated != 3 || first.Stored != 3 {
		t.Errorf("first result = %+v, want calculated=3 stored=3", first)
	}
	var afterFirst int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM option_metrics`).Scan(&afterFirst); err != nil {
		t.Fatal(err)
	}

	second, err := ComputeAndStoreMetrics(ctx, repo, inputs, DefaultRiskFreeRate, false, nil)
	if err != nil {
		t.Fatalf("repeat store metrics: %v", err)
	}
	var afterSecond int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM option_metrics`).Scan(&afterSecond); err != nil {
		t.Fatal(err)
	}
	if second.Stored != 3 || afterSecond != afterFirst {
		t.Errorf("repeat result=%+v counts=%d then %d; want 3 stored and unchanged row count", second, afterFirst, afterSecond)
	}
}

func TestCollectorStoreMetricsUsesSharedPath(t *testing.T) {
	db := newMarketTestDB(t, true)
	ids, _, _ := seedMarketTestData(t, db)
	ctx := context.Background()
	repo := NewRepository(db)
	quotes, err := repo.LatestQuotesForMetrics(ctx)
	if err != nil {
		t.Fatalf("LatestQuotesForMetrics: %v", err)
	}
	inputs, skipped := BuildMetricInputs(quotes, nil)
	if len(skipped) != 1 || len(inputs) != 3 {
		t.Fatalf("built %d inputs and skipped %d, want 3 and 1", len(inputs), len(skipped))
	}

	var input MetricInput
	for _, candidate := range inputs {
		if candidate.ContractID == ids.aaaCallToday {
			input = candidate
			break
		}
	}
	if input.ContractID == 0 {
		t.Fatalf("no metric input for contract %d", ids.aaaCallToday)
	}
	var noteErr error
	collector := NewCollector(nil, repo, nil)
	stored := collector.storeMetrics(ctx, input.TS, []metricInput{input}, func(err error) {
		if err != nil {
			noteErr = err
		}
	})
	if noteErr != nil || stored != 1 {
		t.Fatalf("collector stored=%d noteErr=%v, want one stored row and no error", stored, noteErr)
	}
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM option_metrics WHERE contract_id=$1 AND ts=$2
	)`, input.ContractID, input.TS).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("collector did not persist metrics for contract %d at %s", input.ContractID, input.TS)
	}
}
