// Mock of GET /backtests and GET /backtests/:id for strategy backtests.
//
// backtestRuns.json is REAL output of the backtest engine (backtest/scripts/export_api_runs.py),
// 9 templates on 4 underlyings, the last 26 weekly expiries, with Black-Scholes prices at HV20 on
// adjusted price history (no real option history exists for past expiries yet), so every run has
// modelPricedShare = 1. The page says so.

import type { BacktestDetail, BacktestParams, BacktestSummary } from "@/types/backtestTypes";

type Stored = BacktestSummary & Omit<BacktestDetail, keyof BacktestSummary>;

let cache: Promise<Stored[]> | null = null;
const load = () => (cache ??= import("./backtestRuns.json").then((m) => m.default as unknown as Stored[]));

const summary = (r: Stored): BacktestSummary => ({
	id: r.id, modelName: r.modelName, template: r.template, underlying: r.underlying,
	startDate: r.startDate, endDate: r.endDate, winRate: r.winRate, maxDrawdown: r.maxDrawdown,
	totalReturn: r.totalReturn, tradeCount: r.tradeCount, avgPnl: r.avgPnl, modelPricedShare: r.modelPricedShare,
	createdAt: r.createdAt,
});

export async function listBacktests(p: BacktestParams = {}): Promise<{ count: number; items: BacktestSummary[] }> {
	let rows = await load();
	if (p.underlying) rows = rows.filter((r) => r.underlying === p.underlying);
	if (p.template) rows = rows.filter((r) => r.template === p.template);
	const items = rows.slice(0, p.limit ?? 20).map(summary);
	return { count: items.length, items };
}

export async function getBacktest(id: number): Promise<BacktestDetail | null> {
	return (await load()).find((r) => r.id === id) ?? null;
}
