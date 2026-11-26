package market

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"Finova/internal/options"
)

// LatestMetricQuote contains the latest persisted quote and its contract and
// underlying data needed to build a metric input.
type LatestMetricQuote struct {
	ContractID           int
	UnderlyingID         int
	OptionType           string
	StrikePrice          int64
	ExpiryDate           time.Time
	QuoteTS              *time.Time
	LastPrice            *int64
	BestBid              *int64
	BestAsk              *int64
	ClosePrice           *int64
	UnderlyingLastPrice  *int64
	UnderlyingClosePrice *int64
}

// MetricSkip identifies an input or calculation that could not be processed.
type MetricSkip struct {
	ContractID int
	Reason     string
}

// MetricComputeResult summarizes calculations and writes for one metrics pass.
type MetricComputeResult struct {
	Calculated int
	Skipped    []MetricSkip
	Stored     int
}

// LatestQuotesForMetrics returns every unexpired contract with its latest
// option quote, if present, and the latest quote for its underlying, if present.
func (r *Repository) LatestQuotesForMetrics(ctx context.Context) ([]LatestMetricQuote, error) {
	const q = `
		SELECT c.id, c.underlying_id, c.option_type::text, c.strike_price, c.expiry_date,
		       oq.ts, oq.last_price, oq.best_bid, oq.best_ask, oq.close_price,
		       uq.last_price, uq.close_price
		FROM option_contracts c
		LEFT JOIN option_quotes_latest oq ON oq.contract_id = c.id
		LEFT JOIN LATERAL (
			SELECT last_price, close_price
			FROM underlying_quotes
			WHERE underlying_id = c.underlying_id
			ORDER BY ts DESC
			LIMIT 1
		) uq ON TRUE
		WHERE c.expiry_date >= $1
		ORDER BY c.id`

	rows, err := r.db.Query(ctx, q, tehranDate(time.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotes []LatestMetricQuote
	for rows.Next() {
		var quote LatestMetricQuote
		if err := rows.Scan(
			&quote.ContractID, &quote.UnderlyingID, &quote.OptionType, &quote.StrikePrice, &quote.ExpiryDate,
			&quote.QuoteTS, &quote.LastPrice, &quote.BestBid, &quote.BestAsk, &quote.ClosePrice,
			&quote.UnderlyingLastPrice, &quote.UnderlyingClosePrice,
		); err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	return quotes, rows.Err()
}

// BuildMetricInputs converts stored quotes to calculation inputs. at, when
// provided, replaces the quote timestamp for every input.
func BuildMetricInputs(quotes []LatestMetricQuote, at *time.Time) ([]MetricInput, []MetricSkip) {
	inputs := make([]MetricInput, 0, len(quotes))
	var skipped []MetricSkip
	for _, quote := range quotes {
		skip := func(reason string) {
			skipped = append(skipped, MetricSkip{ContractID: quote.ContractID, Reason: reason})
		}
		if quote.QuoteTS == nil {
			skip("no option quote")
			continue
		}
		kind, err := options.ParseKind(quote.OptionType)
		if err != nil {
			skip(fmt.Sprintf("unsupported option type %q", quote.OptionType))
			continue
		}
		if quote.StrikePrice <= 0 {
			skip("strike price is not positive")
			continue
		}
		spot := pickSpot(float64(valueOrZero(quote.UnderlyingLastPrice)), float64(valueOrZero(quote.UnderlyingClosePrice)))
		if spot <= 0 {
			skip("underlying spot is not positive")
			continue
		}

		ts := *quote.QuoteTS
		if at != nil {
			ts = *at
		}
		inputs = append(inputs, MetricInput{
			ContractID:   quote.ContractID,
			UnderlyingID: quote.UnderlyingID,
			Kind:         kind,
			Strike:       float64(quote.StrikePrice),
			Expiry:       quote.ExpiryDate,
			Spot:         spot,
			TS:           ts,
			Last:         float64(valueOrZero(quote.LastPrice)),
			Bid:          float64(valueOrZero(quote.BestBid)),
			Ask:          float64(valueOrZero(quote.BestAsk)),
			Close:        float64(valueOrZero(quote.ClosePrice)),
		})
	}
	return inputs, skipped
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

// HistoricalVolatilities returns HV20 for underlyings with a complete close
// window. Missing history and database errors are logged; other metrics do not
// depend on historical volatility.
func HistoricalVolatilities(ctx context.Context, repo *Repository, log *slog.Logger) map[int]float64 {
	if log == nil {
		log = slog.Default()
	}
	closes, err := repo.RecentCloses(ctx, options.HVWindow+1)
	if err != nil {
		log.Warn("historical volatility unavailable: loading daily prices failed", "err", err)
		return nil
	}
	out := make(map[int]float64, len(closes))
	for id, series := range closes {
		if len(series) != options.HVWindow+1 {
			continue
		}
		v, err := options.HV20(series)
		if err != nil {
			log.Warn("historical volatility skipped", "underlyingId", id, "err", err)
			continue
		}
		out[id] = v
	}
	return out
}

// ComputeAndStoreMetrics calculates metrics using the collector's shared
// calculation path. In dry-run mode it calculates rows but does not write them.
func ComputeAndStoreMetrics(
	ctx context.Context,
	repo *Repository,
	inputs []MetricInput,
	rate float64,
	dryRun bool,
	log *slog.Logger,
) (MetricComputeResult, error) {
	if log == nil {
		log = slog.Default()
	}
	if len(inputs) == 0 {
		return MetricComputeResult{}, nil
	}
	hv := HistoricalVolatilities(ctx, repo, log)
	result := MetricComputeResult{}
	rows := make([]OptionMetrics, 0, len(inputs))
	for _, input := range inputs {
		if input.TS.IsZero() {
			result.Skipped = append(result.Skipped, MetricSkip{ContractID: input.ContractID, Reason: "quote timestamp is missing"})
			continue
		}
		input.HV = nil
		if value, ok := hv[input.UnderlyingID]; ok {
			input.HV = &value
		}
		metric, err := computeMetrics(input, input.TS, rate)
		if err != nil {
			result.Skipped = append(result.Skipped, MetricSkip{
				ContractID: input.ContractID,
				Reason:     err.Error(),
			})
			continue
		}
		rows = append(rows, metric)
	}
	result.Calculated = len(rows)
	for _, skipped := range result.Skipped {
		log.Warn("metrics contract skipped", "contractId", skipped.ContractID, "reason", skipped.Reason)
	}
	if dryRun || len(rows) == 0 {
		return result, nil
	}
	if err := repo.UpsertOptionMetrics(ctx, rows); err != nil {
		return result, err
	}
	result.Stored = len(rows)
	return result, nil
}
