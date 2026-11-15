// Data hooks for the market pages. All of them refresh every 30 seconds and
// stop while the browser tab is hidden (TanStack Query only refetches on an
// interval while the tab is visible, because refetchIntervalInBackground is
// false). `dataUpdatedAt` of the result is the "last updated" time.

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
	getChain,
	getExpiries,
	getMarketWatch,
	getUnderlying,
	getUnderlyings,
} from "@/services/marketService";
import type { MarketWatchParams } from "@/types/marketTypes";

export const REFRESH_MS = 30_000;

const live = {
	refetchInterval: REFRESH_MS,
	refetchIntervalInBackground: false,
} as const;

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

/** The expiry filter list. Changes rarely: no auto-refresh. */
export function useExpiries() {
	return useQuery({
		queryKey: ["expiries"],
		queryFn: () => getExpiries(),
		staleTime: 5 * 60_000,
	});
}

export function useUnderlying(symbol: string | undefined) {
	return useQuery({
		queryKey: ["underlying", symbol],
		queryFn: () => getUnderlying(symbol!),
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
