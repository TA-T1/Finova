// Typed API functions for the market pages.
//
// Mock or real is decided HERE, once, by VITE_USE_MOCK (default: mock). The
// components only call these functions, so connecting the real backend is a
// config change: set VITE_USE_MOCK=false (and VITE_API_BASE_URL if needed).
//
// The real-API branch follows docs/api.md plus the proposals in
// docs/frontend-api-requests.md. IT HAS NOT BEEN TESTED AGAINST A SERVER:
// the endpoints for sort, paging, chain and underlying detail do not exist yet.

import axios from "axios";
import type {
	ExpiryInfo,
	ListResponse,
	MarketWatchParams,
	OptionQuote,
	PagedResponse,
	Underlying,
	UnderlyingDetail,
	UnderlyingKind,
} from "@/types/marketTypes";
import { getData } from "./services";

export const USE_MOCK: boolean = import.meta.env.VITE_USE_MOCK !== "false";

const MARKET = "/api/v1/market";

const mocks = () => import("@/mocks/mockApi"); // its own chunk; unused when real

/** Market watch: contracts with quotes and calculated columns. [contract + proposed params] */
export async function getMarketWatch(
	params: MarketWatchParams = {},
): Promise<PagedResponse<OptionQuote>> {
	if (USE_MOCK) return (await mocks()).getMarketWatch(params);
	return getData({ endPoint: `${MARKET}/options`, params });
}

/** Both sides of one underlying's chain for one expiry ("YYYY-MM-DD"). [proposed] */
export async function getChain(
	symbol: string,
	expiry: string,
): Promise<ListResponse<OptionQuote>> {
	if (USE_MOCK) return (await mocks()).getChain(symbol, expiry);
	return getData({ endPoint: `${MARKET}/chain`, params: { underlying: symbol, expiry } });
}

/** One underlying: price, previous close and the list of expiries. [proposed] */
export async function getUnderlying(symbol: string): Promise<UnderlyingDetail> {
	if (USE_MOCK) return (await mocks()).getUnderlying(symbol);
	return getData({ endPoint: `${MARKET}/underlyings/${encodeURIComponent(symbol)}` });
}

/** All underlyings, for the filter lists. [contract] */
export async function getUnderlyings(
	kind?: UnderlyingKind,
): Promise<ListResponse<Underlying>> {
	if (USE_MOCK) return (await mocks()).getUnderlyings(kind);
	return getData({ endPoint: `${MARKET}/underlyings`, params: { kind } });
}

/** Distinct expiries of all contracts, for the expiry filter. [proposed] */
export async function getExpiries(): Promise<ListResponse<ExpiryInfo>> {
	if (USE_MOCK) return (await mocks()).getExpiries();
	return getData({ endPoint: `${MARKET}/expiries` });
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
