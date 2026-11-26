package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type ListQuery struct {
	Underlying string
	Template   string
	Model      string
	Limit      int
}

type Summary struct {
	ID               int       `json:"id"`
	ModelName        string    `json:"modelName"`
	Template         *string   `json:"template"`
	Underlying       *string   `json:"underlying"`
	StartDate        *string   `json:"startDate"`
	EndDate          *string   `json:"endDate"`
	WinRate          *float64  `json:"winRate"`
	MaxDrawdown      *float64  `json:"maxDrawdown"`
	TotalReturn      *float64  `json:"totalReturn"`
	TradeCount       int       `json:"tradeCount"`
	AvgPnl           *float64  `json:"avgPnl"`
	ModelPricedShare *float64  `json:"modelPricedShare"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Detail struct {
	Summary
	Params      map[string]any `json:"params"`
	Metrics     any            `json:"metrics"`
	Trades      []any          `json:"trades"`
	Skipped     []any          `json:"skipped"`
	EquityCurve []any          `json:"equityCurve"`
}

type ListResponse struct {
	Count int       `json:"count"`
	Items []Summary `json:"items"`
}

type runRow struct {
	ID          int
	ModelName   string
	Params      []byte
	StartDate   *time.Time
	EndDate     *time.Time
	WinRate     *float64
	MaxDrawdown *float64
	TotalReturn *float64
	Metrics     []byte
	Trades      []byte
	Skipped     []byte
	EquityCurve []byte
	CreatedAt   time.Time
}

func (r *Repository) List(ctx context.Context, query ListQuery) ([]Summary, error) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if query.Underlying != "" {
		args = append(args, query.Underlying)
		conditions = append(conditions, fmt.Sprintf("params->>'underlying' = $%d", len(args)))
	}
	if query.Template != "" {
		args = append(args, query.Template)
		conditions = append(conditions, fmt.Sprintf("model_name = 'strategy:' || $%d", len(args)))
	}
	if query.Model != "" {
		args = append(args, query.Model)
		conditions = append(conditions, fmt.Sprintf("model_name = $%d", len(args)))
	}
	args = append(args, query.Limit)

	sql := `SELECT id, model_name, params, start_date, end_date, win_rate,
	           max_drawdown, total_return, metrics, trades, skipped, equity_curve, created_at
	        FROM backtest_runs`
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	items := make([]Summary, 0)
	for rows.Next() {
		var row runRow
		if err := rows.Scan(
			&row.ID, &row.ModelName, &row.Params, &row.StartDate, &row.EndDate,
			&row.WinRate, &row.MaxDrawdown, &row.TotalReturn, &row.Metrics,
			&row.Trades, &row.Skipped, &row.EquityCurve, &row.CreatedAt,
		); err != nil {
			return nil, err
		}
		params, _ := decodeObject(row.Params)
		underlying := stringValue(params["underlying"])
		template := templateFromModel(row.ModelName)
		metrics, _ := decodeObject(row.Metrics)
		tradeCount := 0
		if value, ok := numberValue(metrics["trades"]); ok {
			tradeCount = int(value)
		}

		items = append(items, Summary{
			ID: row.ID, ModelName: row.ModelName, Template: template, Underlying: underlying,
			StartDate: dateString(row.StartDate), EndDate: dateString(row.EndDate),
			WinRate: row.WinRate, MaxDrawdown: row.MaxDrawdown, TotalReturn: row.TotalReturn,
			TradeCount:       tradeCount,
			AvgPnl:           floatValue(metrics["avg_pnl"]),
			ModelPricedShare: floatValue(metrics["model_priced_share"]),
			CreatedAt:        row.CreatedAt,
		})
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int) (*Detail, error) {
	const sql = `SELECT id, model_name, params, start_date, end_date, win_rate,
	           max_drawdown, total_return, metrics, trades, skipped, equity_curve, created_at
	        FROM backtest_runs WHERE id = $1`
	var row runRow
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&row.ID, &row.ModelName, &row.Params, &row.StartDate, &row.EndDate,
		&row.WinRate, &row.MaxDrawdown, &row.TotalReturn, &row.Metrics,
		&row.Trades, &row.Skipped, &row.EquityCurve, &row.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	params, _ := decodeObject(row.Params)
	metrics, _ := decodeObject(row.Metrics)
	trades, _ := decodeArray(row.Trades)
	skipped, _ := decodeArray(row.Skipped)
	equityCurve, _ := decodeArray(row.EquityCurve)
	tradeCount := 0
	if value, ok := numberValue(metrics["trades"]); ok {
		tradeCount = int(value)
	}

	detail := &Detail{
		Summary: Summary{
			ID: row.ID, ModelName: row.ModelName,
			Template:   templateFromModel(row.ModelName),
			Underlying: stringValue(params["underlying"]),
			StartDate:  dateString(row.StartDate), EndDate: dateString(row.EndDate),
			WinRate: row.WinRate, MaxDrawdown: row.MaxDrawdown, TotalReturn: row.TotalReturn,
			TradeCount:       tradeCount,
			AvgPnl:           floatValue(metrics["avg_pnl"]),
			ModelPricedShare: floatValue(metrics["model_priced_share"]),
			CreatedAt:        row.CreatedAt,
		},
		Params:      CamelCaseKeys(params).(map[string]any),
		Trades:      CamelCaseKeys(trades).([]any),
		Skipped:     CamelCaseKeys(skipped).([]any),
		EquityCurve: CamelCaseKeys(equityCurve).([]any),
	}
	if len(row.Metrics) > 0 && string(row.Metrics) != "null" {
		detail.Metrics = CamelCaseKeys(metrics)
	} else {
		detail.Metrics = nil
	}
	return detail, nil
}

func templateFromModel(model string) *string {
	const prefix = "strategy:"
	if !strings.HasPrefix(model, prefix) {
		return nil
	}
	template := strings.TrimPrefix(model, prefix)
	return &template
}

func dateString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.Format("2006-01-02")
	return &text
}

func stringValue(value any) *string {
	text, ok := value.(string)
	if !ok || text == "" {
		return nil
	}
	return &text
}

func floatValue(value any) *float64 {
	number, ok := numberValue(value)
	if !ok {
		return nil
	}
	return &number
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func decodeObject(raw []byte) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

func decodeArray(raw []byte) ([]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []any{}, nil
	}
	var value []any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return []any{}, nil
	}
	return value, nil
}
