// Typed API functions: market, strategies and backtests.
//
// Mock or real is decided HERE, once, by VITE_USE_MOCK (default: mock). The
// components only call these functions, so connecting the real backend is a
// config change: set VITE_USE_MOCK=false (and VITE_API_BASE_URL if needed).
//
// The paths, parameters and shapes follow docs/api-contract.md. Backtests follow
// docs/strategy-backtest.md (a proposal). Fields the pages need that the contract
// lacks are listed in docs/frontend-api-notes.md.

import axios from "axios";
import type {
	ChainResponse,
	ExpiriesResponse,
	ListResponse,
	MarketStatus,
	MarketWatchParams,
	OptionQuote,
	PagedResponse,
	SymbolDetail,
	Underlying,
	UnderlyingKind,
} from "@/types/marketTypes";
import type { BacktestDetail, BacktestParams, BacktestSummary } from "@/types/backtestTypes";
import type {
	AnalyzeResponse,
	BuildRequest,
	BuildResponse,
	LegInput,
	StrategyTemplate,
} from "@/types/strategyTypes";
import { getData, postData } from "./services";

export const USE_MOCK: boolean = import.meta.env.VITE_USE_MOCK !== "false";

const API = "/api/v1";
const MARKET = `${API}/market`;

const mocks = () => import("@/mocks/mockApi"); // its own chunk; unused when real

// ---- market ----------------------------------------------------------------

/** Market open or closed, and the time of the newest quote. */
export async function getStatus(): Promise<MarketStatus> {
	if (USE_MOCK) return (await mocks()).getStatus();
	return getData({ endPoint: `${MARKET}/status` });
}

/** The market watch: contracts with quotes and calculated fields (contract 1.5). */
export async function getMarketWatch(
	params: MarketWatchParams = {},
): Promise<PagedResponse<OptionQuote>> {
	if (USE_MOCK) return (await mocks()).getMarketWatch(params);
	return getData({ endPoint: `${MARKET}/options`, params });
}

/** One expiry of one underlying, call and put side by side (contract 1.6). */
export async function getChain(symbol: string, expiry: string): Promise<ChainResponse> {
	if (USE_MOCK) return (await mocks()).getChain(symbol, expiry);
	return getData({ endPoint: `${MARKET}/chain`, params: { underlying: symbol, expiry } });
}

/** The symbol page of one underlying (contract 1.3). */
export async function getSymbol(symbol: string): Promise<SymbolDetail> {
	if (USE_MOCK) return (await mocks()).getSymbol(symbol);
	return getData({ endPoint: `${MARKET}/underlyings/${encodeURIComponent(symbol)}` });
}

/** All underlyings, for the filter lists (contract 1.2). */
export async function getUnderlyings(kind?: UnderlyingKind): Promise<ListResponse<Underlying>> {
	if (USE_MOCK) return (await mocks()).getUnderlyings(kind);
	return getData({ endPoint: `${MARKET}/underlyings`, params: { kind } });
}

/** Active expiries of one underlying, or of the whole market (contract 1.4). */
export async function getExpiries(underlying?: string): Promise<ExpiriesResponse> {
	if (USE_MOCK) return (await mocks()).getExpiries(underlying);
	return getData({ endPoint: `${MARKET}/expiries`, params: { underlying } });
}

// ---- strategies --------------------------------------------------------------

/** The strategy templates (contract 2.3). */
export async function getStrategyTemplates(): Promise<ListResponse<StrategyTemplate>> {
	if (USE_MOCK) return (await mocks()).getTemplates();
	return getData({ endPoint: `${API}/strategies/templates` });
}

/** Fills a template with real contracts and analyses it. Nothing is saved (contract 2.4). */
export async function buildStrategy(req: BuildRequest): Promise<BuildResponse> {
	if (USE_MOCK) return (await mocks()).buildStrategy(req);
	return postData({ endPoint: `${API}/strategies/build`, data: req });
}

/** Analyses legs the user assembled. Nothing is saved (contract 2.4). */
export async function analyzeStrategy(legs: LegInput[]): Promise<AnalyzeResponse> {
	if (USE_MOCK) return (await mocks()).analyzeStrategy(legs);
	return postData({ endPoint: `${API}/strategies/analyze`, data: { legs } });
}

// ---- backtests ----------------------------------------------------------------

/** Saved backtest runs, newest first (docs/strategy-backtest.md). */
export async function getBacktests(params: BacktestParams = {}): Promise<ListResponse<BacktestSummary>> {
	if (USE_MOCK) return (await mocks()).getBacktests(params);
	return getData({ endPoint: `${API}/backtests`, params });
}

/** One run with its equity curve and per-expiry trades. */
export async function getBacktest(id: number): Promise<BacktestDetail> {
	if (USE_MOCK) return (await mocks()).getBacktestDetail(id);
	return getData({ endPoint: `${API}/backtests/${id}` });
}

/** A message safe to show for any failure (backend sends { "error": "..." }). */
export function getErrorMessage(err: unknown): string {
	if (axios.isAxiosError(err)) {
		const body = err.response?.data as { error?: string } | undefined;
		if (body?.error) return body.error;
		if (err.code === "ECONNABORTED") return "پاسخی از سرور دریافت نشد (زمان انتظار تمام شد)";
		if (!err.response) return "ارتباط با سرور برقرار نشد";
		return `خطای سرور (${err.response.status})`;
	}
	if (err instanceof Error && err.message) return err.message;
	return "خطای نامشخص";
}
