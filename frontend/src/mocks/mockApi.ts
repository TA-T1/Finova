// Mock implementation of the API. It does what the real server is expected to do
// (filter, sort, paginate, answer 404 for an unknown symbol) so the pages cannot
// tell the difference. Same signatures as the real functions in services/*.ts, and
// the same shapes as docs/api-contract.md.
//
// Test switches, read from the page URL on every request:
//   ?mockError=1   every request fails (shows the error state and the retry button)
//   ?mockEmpty=1   every list comes back empty (shows the empty state)

import type {
	ChainResponse,
	ExpiriesResponse,
	ListResponse,
	MarketSortKey,
	MarketStatus,
	MarketWatchParams,
	OptionQuote,
	PagedResponse,
	SymbolDetail,
	Underlying,
	UnderlyingKind,
} from "@/types/marketTypes";
import type { BacktestDetail, BacktestParams, BacktestSummary } from "@/types/backtestTypes";
import type { AnalyzeResponse, BuildRequest, BuildResponse, LegInput, StrategyTemplate } from "@/types/strategyTypes";
import { getBacktest as mockGetBacktest, listBacktests } from "./mockBacktest";
import { chainOf, hasUnderlying, listExpiryRows, listUnderlyingRows, snapshot, symbolDetail } from "./mockMarket";
import { analyze, build, StrategyMockError, TEMPLATES } from "./mockStrategy";

export class MockApiError extends Error {
	status: number;
	constructor(message: string, status: number) {
		super(message);
		this.name = "MockApiError";
		this.status = status;
	}
}

const DEFAULT_PAGE_SIZE = 50;
const MAX_PAGE_SIZE = 500;

function flags() {
	const p = new URLSearchParams(window.location.search);
	return { error: p.get("mockError") === "1", empty: p.get("mockEmpty") === "1" };
}

/** Waits 200-400 ms like a network call, then fails or answers. */
async function respond<T>(build: () => T | Promise<T>): Promise<T> {
	await new Promise((r) => setTimeout(r, 200 + Math.random() * 200));
	if (flags().error) {
		throw new MockApiError("خطای نمونه: سرور در دسترس نیست (mockError=1)", 503);
	}
	try {
		return await build();
	} catch (e) {
		if (e instanceof StrategyMockError) throw new MockApiError(e.message, e.status);
		throw e;
	}
}

// ---- sorting ---------------------------------------------------------------

const MONEYNESS_ORDER = { ITM: 1, ATM: 2, OTM: 3 } as const;

/** Moneyness sorts ITM, ATM, OTM, like the server. */
function sortValue(q: OptionQuote, key: MarketSortKey): string | number | null {
	if (key === "moneyness") return q.moneyness ? MONEYNESS_ORDER[q.moneyness] : null;
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
		typeof x === "string" && typeof y === "string" ? x.localeCompare(y, "fa") : Number(x) - Number(y);
	return c !== 0 ? c * dir : a.contractId - b.contractId;
}

const SORT_KEYS: MarketSortKey[] = [
	"symbol", "underlyingSymbol", "optionType", "strikePrice", "expiryDate", "daysToExpiry", "lastPrice", "volume",
	"openInterest", "impliedVolatility", "delta", "leverage", "effectiveLeverage", "timeValue", "intrinsicValue",
	"priceGapPercent", "breakEvenPercent", "bestBid", "value", "breakEven", "moneyness",
];

// ---- market ----------------------------------------------------------------

/** GET /market/status */
export function getStatus(): Promise<MarketStatus> {
	return respond(() => ({ marketOpen: true, lastUpdate: new Date().toISOString(), timezone: "Asia/Tehran" }));
}

/** GET /market/options */
export function getMarketWatch(params: MarketWatchParams = {}): Promise<PagedResponse<OptionQuote>> {
	return respond(() => {
		if (params.sort && !SORT_KEYS.includes(params.sort)) {
			throw new MockApiError(`sort نامعتبر: ${params.sort}`, 400);
		}
		const pageSize = Math.min(Math.max(params.pageSize ?? DEFAULT_PAGE_SIZE, 1), MAX_PAGE_SIZE);
		const page = Math.max(params.page ?? 1, 1);

		let rows = flags().empty ? [] : snapshot();
		if (params.underlying) rows = rows.filter((r) => r.underlyingSymbol === params.underlying);
		if (params.type) rows = rows.filter((r) => r.optionType === params.type);
		if (params.underlyingKind) rows = rows.filter((r) => r.underlyingKind === params.underlyingKind);
		if (params.expiry) rows = rows.filter((r) => r.expiryDate === params.expiry);

		if (params.sort) {
			const dir = params.order === "desc" ? -1 : 1;
			const key = params.sort;
			rows.sort((a, b) => compare(a, b, key, dir));
		} else {
			// Default order of the contract: underlying, expiry, strike, type.
			rows.sort(
				(a, b) =>
					a.underlyingSymbol.localeCompare(b.underlyingSymbol, "fa") ||
					a.expiryDate.localeCompare(b.expiryDate) ||
					a.strikePrice - b.strikePrice ||
					a.optionType.localeCompare(b.optionType),
			);
		}

		const items = rows.slice((page - 1) * pageSize, page * pageSize);
		return { items, count: items.length, page, pageSize, total: rows.length, totalPages: Math.ceil(rows.length / pageSize) };
	});
}

/** GET /market/chain?underlying=&expiry= */
export function getChain(symbol: string, expiry: string): Promise<ChainResponse> {
	return respond(() => {
		const chain = flags().empty ? null : chainOf(symbol, expiry);
		if (!chain) throw new MockApiError("برای این دارایی و سررسید قرارداد فعالی وجود ندارد", 404);
		return chain;
	});
}

/** GET /market/underlyings/:symbol */
export function getSymbol(symbol: string): Promise<SymbolDetail> {
	return respond(() => {
		const d = flags().empty ? null : symbolDetail(symbol);
		if (!d) throw new MockApiError(`دارایی پایه «${symbol}» پیدا نشد`, 404);
		return d;
	});
}

/** GET /market/underlyings?kind= */
export function getUnderlyings(kind?: UnderlyingKind): Promise<ListResponse<Underlying>> {
	return respond(() => {
		const items = flags().empty ? [] : listUnderlyingRows(kind);
		return { count: items.length, items };
	});
}

/** GET /market/expiries?underlying= */
export function getExpiries(underlying?: string): Promise<ExpiriesResponse> {
	return respond(() => {
		if (underlying && !hasUnderlying(underlying)) {
			throw new MockApiError(`دارایی پایه «${underlying}» پیدا نشد`, 404);
		}
		return { underlyingSymbol: underlying ?? null, items: flags().empty ? [] : listExpiryRows(underlying) };
	});
}

// ---- strategies ------------------------------------------------------------

export function getTemplates(): Promise<ListResponse<StrategyTemplate>> {
	return respond(() => ({ count: TEMPLATES.length, items: TEMPLATES }));
}

export function buildStrategy(req: BuildRequest): Promise<BuildResponse> {
	return respond(() => build(req));
}

export function analyzeStrategy(legs: LegInput[]): Promise<AnalyzeResponse> {
	return respond(() => analyze(legs));
}

// ---- backtests -------------------------------------------------------------

export function getBacktests(p: BacktestParams = {}): Promise<ListResponse<BacktestSummary>> {
	return respond(async () => (flags().empty ? { count: 0, items: [] } : listBacktests(p)));
}

export function getBacktestDetail(id: number): Promise<BacktestDetail> {
	return respond(async () => {
		const r = await mockGetBacktest(id);
		if (!r) throw new MockApiError(`اجرای بک‌تست ${id} پیدا نشد`, 404);
		return r;
	});
}
