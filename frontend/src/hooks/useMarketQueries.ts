// Data hooks. The market ones refresh every 30 seconds and stop while the browser
// tab is hidden (TanStack Query only refetches on an interval while the tab is
// visible, because refetchIntervalInBackground is false). `dataUpdatedAt` of the
// result is the "last updated" time.

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
	analyzeStrategy,
	getBacktest,
	getBacktests,
	getChain,
	getExpiries,
	getMarketWatch,
	getStatus,
	getStrategyTemplates,
	getSymbol,
	getUnderlyings,
} from "@/services/marketService";
import type { BacktestParams } from "@/types/backtestTypes";
import type { MarketWatchParams } from "@/types/marketTypes";
import type { LegInput } from "@/types/strategyTypes";

export const REFRESH_MS = 30_000;

const live = {
	refetchInterval: REFRESH_MS,
	refetchIntervalInBackground: false,
} as const;

export function useMarketStatus() {
	return useQuery({ queryKey: ["marketStatus"], queryFn: getStatus, ...live });
}

export function useMarketWatch(params: MarketWatchParams) {
	return useQuery({
		queryKey: ["marketWatch", params],
		queryFn: () => getMarketWatch(params),
		// Keep the old rows on screen while a new sort/page/filter loads.
		placeholderData: keepPreviousData,
		...live,
	});
}

/** The list for the underlying filter. Changes rarely: no auto-refresh. */
export function useUnderlyings() {
	return useQuery({
		queryKey: ["underlyings"],
		queryFn: () => getUnderlyings(),
		staleTime: 5 * 60_000,
	});
}

/** The expiries of one underlying, or of the whole market. Changes rarely: no auto-refresh. */
export function useExpiries(underlying?: string) {
	return useQuery({
		queryKey: ["expiries", underlying ?? null],
		queryFn: () => getExpiries(underlying),
		staleTime: 5 * 60_000,
	});
}

/** The symbol page data: price, expiries, open interest, max pain. */
export function useSymbol(symbol: string | undefined) {
	return useQuery({
		queryKey: ["symbol", symbol],
		queryFn: () => getSymbol(symbol!),
		enabled: !!symbol,
		...live,
	});
}

export function useChain(symbol: string | undefined, expiry: string | undefined) {
	return useQuery({
		queryKey: ["chain", symbol, expiry],
		queryFn: () => getChain(symbol!, expiry!),
		enabled: !!symbol && !!expiry,
		placeholderData: keepPreviousData,
		...live,
	});
}

export function useStrategyTemplates() {
	return useQuery({ queryKey: ["strategyTemplates"], queryFn: getStrategyTemplates, staleTime: 60 * 60_000 });
}

/** Analysis of the legs being edited. Runs when they change, not on a timer. */
export function useStrategyAnalysis(legs: LegInput[]) {
	return useQuery({
		queryKey: ["strategyAnalysis", legs],
		queryFn: () => analyzeStrategy(legs),
		enabled: legs.length > 0,
		placeholderData: keepPreviousData,
		staleTime: 15_000,
		retry: false,
	});
}

export function useBacktests(params: BacktestParams) {
	return useQuery({
		queryKey: ["backtests", params],
		queryFn: () => getBacktests(params),
		placeholderData: keepPreviousData,
		staleTime: 60_000,
	});
}

export function useBacktest(id: number | undefined) {
	return useQuery({
		queryKey: ["backtest", id],
		queryFn: () => getBacktest(id!),
		enabled: id !== undefined,
		staleTime: 5 * 60_000,
	});
}
