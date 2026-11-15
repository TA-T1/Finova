// Mock implementation of the market API. It does what the real server is
// expected to do (filter, sort, paginate, answer 404 for an unknown symbol) so
// the pages cannot tell the difference. Same signatures as the real functions in
// services/marketService.ts.
//
// Test switches, read from the page URL on every request:
//   ?mockError=1   every request fails (shows the error state and the retry button)
//   ?mockEmpty=1   every list comes back empty (shows the empty state)

import type {
	ExpiryInfo,
	ListResponse,
	MarketSortKey,
	MarketWatchParams,
	OptionQuote,
	PagedResponse,
	Underlying,
	UnderlyingDetail,
	UnderlyingKind,
} from "@/types/marketTypes";
import { listExpiryRows, listUnderlyingRows, snapshot, underlyingDetail } from "./mockMarket";

export class MockApiError extends Error {
	status: number;
	constructor(message: string, status: number) {
		super(message);
		this.name = "MockApiError";
		this.status = status;
	}
}

const DEFAULT_PAGE_SIZE = 25;
const MAX_PAGE_SIZE = 200;

function flags() {
	const p = new URLSearchParams(window.location.search);
	return { error: p.get("mockError") === "1", empty: p.get("mockEmpty") === "1" };
}

/** Waits 200-400 ms like a network call, then fails or answers. */
async function respond<T>(build: () => T): Promise<T> {
	await new Promise((r) => setTimeout(r, 200 + Math.random() * 200));
	if (flags().error) {
		throw new MockApiError("خطای نمونه: سرور در دسترس نیست (mockError=1)", 503);
	}
	return build();
}

// ---- sorting ---------------------------------------------------------------

const MONEYNESS_ORDER = { ITM: 0, ATM: 1, OTM: 2 } as const;

function sortValue(q: OptionQuote, key: MarketSortKey): string | number | null {
	if (key === "moneyness") return q.moneyness ? MONEYNESS_ORDER[q.moneyness] : null;
	if (key === "expiryDate") return q.expiryDate.slice(0, 10);
	return q[key];
}

/** Nulls always go last, in both directions, like SQL `NULLS LAST`. */
function compare(a: OptionQuote, b: OptionQuote, key: MarketSortKey, dir: 1 | -1): number {
	const x = sortValue(a, key);
	const y = sortValue(b, key);
	if (x === null && y === null) return a.contractId - b.contractId;
	if (x === null) return 1;
	if (y === null) return -1;
	const c =
		typeof x === "string" && typeof y === "string"
			? x.localeCompare(y, "fa")
			: Number(x) - Number(y);
	return c !== 0 ? c * dir : a.contractId - b.contractId;
}

// ---- endpoints -------------------------------------------------------------

/** GET /api/v1/market/options */
export function getMarketWatch(
	params: MarketWatchParams = {},
): Promise<PagedResponse<OptionQuote>> {
	return respond(() => {
		const pageSize = Math.min(Math.max(params.pageSize ?? DEFAULT_PAGE_SIZE, 1), MAX_PAGE_SIZE);
		const page = Math.max(params.page ?? 1, 1);

		let rows = flags().empty ? [] : snapshot();
		if (params.underlying) rows = rows.filter((r) => r.underlyingSymbol === params.underlying);
		if (params.type) rows = rows.filter((r) => r.optionType === params.type);
		if (params.underlyingKind) rows = rows.filter((r) => r.underlyingKind === params.underlyingKind);
		if (params.expiry) rows = rows.filter((r) => r.expiryDate.slice(0, 10) === params.expiry);

		const key = params.sortBy ?? "symbol";
		const dir = params.sortDir === "desc" ? -1 : 1;
		rows.sort((a, b) => compare(a, b, key, dir));

		const items = rows.slice((page - 1) * pageSize, page * pageSize);
		return { count: items.length, total: rows.length, page, pageSize, items };
	});
}

/** GET /api/v1/market/chain?underlying=&expiry= */
export function getChain(symbol: string, expiry: string): Promise<ListResponse<OptionQuote>> {
	return respond(() => {
		if (!underlyingDetail(symbol)) {
			throw new MockApiError(`دارایی پایه «${symbol}» پیدا نشد`, 404);
		}
		const items = flags().empty
			? []
			: snapshot()
					.filter((r) => r.underlyingSymbol === symbol && r.expiryDate.slice(0, 10) === expiry)
					.sort((a, b) => a.strikePrice - b.strikePrice || (a.optionType === "call" ? -1 : 1));
		return { count: items.length, items };
	});
}

/** GET /api/v1/market/underlyings/:symbol */
export function getUnderlying(symbol: string): Promise<UnderlyingDetail> {
	return respond(() => {
		const d = underlyingDetail(symbol);
		if (!d) throw new MockApiError(`دارایی پایه «${symbol}» پیدا نشد`, 404);
		return d;
	});
}

/** GET /api/v1/market/underlyings?kind= */
export function getUnderlyings(kind?: UnderlyingKind): Promise<ListResponse<Underlying>> {
	return respond(() => {
		const items = flags().empty ? [] : listUnderlyingRows(kind);
		return { count: items.length, items };
	});
}

/** GET /api/v1/market/expiries */
export function getExpiries(): Promise<ListResponse<ExpiryInfo>> {
	return respond(() => {
		const items = flags().empty ? [] : listExpiryRows();
		return { count: items.length, items };
	});
}
