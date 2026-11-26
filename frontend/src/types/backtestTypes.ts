// Strategy backtests (docs/strategy-backtest.md, proposed by the backtest engine).
// Extends GET /backtests and GET /backtests/:id of docs/api-contract.md section 3.

export type ExitReason = "expiry" | "target" | "stop";
export type TradePriceSource = "snapshot" | "model" | "mixed";

/** An item of GET /backtests. `template` and `underlying` are null for a signal backtest. */
export interface BacktestSummary {
	id: number;
	modelName: string; // "strategy:bull-call-spread"
	template: string | null;
	underlying: string | null;
	startDate: string | null;
	endDate: string | null;
	/** Fraction 0..1. */
	winRate: number | null;
	/** Positive fraction: 0.18 = a peak-to-trough fall of 18%. */
	maxDrawdown: number | null;
	/** Fraction: 0.42 = +42%. */
	totalReturn: number | null;
	tradeCount: number;
	/** Average P&L of a trade, rial, after fees. */
	avgPnl: number | null;
	/** Share of trades priced with a model instead of real quotes (0..1). */
	modelPricedShare: number | null;
	createdAt: string;
}

export interface BacktestLeg {
	type: "call" | "put" | "underlying";
	side: "buy" | "sell";
	qty: number;
	strike: number | null;
	entryPrice: number;
	exitPrice: number;
	priceSource: "snapshot" | "model";
}

/** One expiry of a run. Money is rial per position. */
export interface BacktestTrade {
	expiry: string;
	entryDate: string;
	exitDate: string;
	exitReason: ExitReason;
	spotEntry: number;
	spotExit: number;
	netPremium: number;
	capitalAtRisk: number;
	maxProfit: number | null;
	maxLoss: number | null;
	grossPnl: number;
	fees: number;
	pnl: number;
	/** pnl / capitalAtRisk, after fees. */
	returnOnRisk: number;
	priceSource: TradePriceSource;
	legs: BacktestLeg[];
}

export interface SkippedExpiry {
	expiry: string;
	reason: string;
}

export interface EquityPoint {
	date: string;
	/** Starts at 1.0. */
	equity: number;
}

export interface BacktestMetrics {
	trades: number;
	skipped: number;
	winRate: number | null;
	avgPnl: number | null;
	avgReturnOnRisk: number | null;
	totalPnl: number;
	totalFees: number;
	bestPnl: number | null;
	worstPnl: number | null;
	/** Gross wins / gross losses; null when there is no losing trade. */
	profitFactor: number | null;
	maxDrawdown: number;
	totalReturn: number;
	modelPricedShare: number | null;
}

/** GET /backtests/:id */
export interface BacktestDetail extends BacktestSummary {
	params: Record<string, unknown>;
	metrics: BacktestMetrics;
	equityCurve: EquityPoint[];
	trades: BacktestTrade[];
	skipped: SkippedExpiry[];
}

export interface BacktestParams {
	underlying?: string;
	template?: string;
	limit?: number;
}
