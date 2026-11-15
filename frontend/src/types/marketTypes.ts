// Every shape the market pages (market watch, option chain) get from the API.
// This is the ONE place to update when the real backend is wired in.
//
// Legend used in the comments below:
//   [contract]  exists today: docs/api.md + the JSON tags in
//               backend/internal/market/repository.go (camelCase).
//   [proposed]  NOT in the contract yet, but these pages need it. Each one is
//               listed, with an example, in docs/frontend-api-requests.md.
//               The names follow backend/internal/options (Evaluation, Metrics).
//
// Conventions that come from the backend:
//   - prices are integers in RIAL (the DB stores BIGINT); contract size is 1000
//   - a missing value is `null` (the Go side uses pointers); never 0
//   - option type is "call" | "put"
//   - dates are ISO 8601 strings; expiryDate is a DATE, so it arrives as
//     "2026-12-20T00:00:00Z" and only its YYYY-MM-DD part is meaningful

export type OptionType = "call" | "put";
export type UnderlyingKind = "stock" | "fund" | "index" | "other";
export type Moneyness = "ITM" | "ATM" | "OTM";

export type SortDir = "asc" | "desc";

/** An option contract with its latest quote and calculated columns. */
export interface OptionQuote {
	// ---- [contract] ----
	contractId: number;
	symbol: string; // e.g. ضخود9001
	underlyingId: number;
	underlyingSymbol: string; // e.g. خودرو
	underlyingKind: UnderlyingKind;
	optionType: OptionType;
	strikePrice: number; // rial
	expiryDate: string;
	contractSize: number;
	lastPrice: number | null;
	closePrice: number | null;
	bestBid: number | null;
	bestAsk: number | null;
	volume: number | null;
	openInterest: number | null;
	quoteTs: string | null;

	// ---- [proposed] quote extras ----
	/** Previous day's price; the % change is computed from it. */
	prevClose: number | null;
	/** Traded value today, rial. */
	value: number | null;

	// ---- [proposed] calculated columns (backend/internal/options) ----
	/** Decimal fraction per year: 0.35 means 35%. null when not computable. */
	impliedVolatility: number | null;
	/** Call: 0..1, put: -1..0. null when not computable. */
	delta: number | null;
	/** Black-Scholes price at the historical volatility, per share, rial. */
	theoreticalPrice: number | null;
	/** (market - theoretical) / theoretical * 100. Already a percentage. */
	priceGapPercent: number | null;
	/** Underlying price at which the buyer breaks even at expiry, rial. */
	breakEven: number | null;
	/** spot / premium. null when the premium is 0. */
	leverage: number | null;
	moneyness: Moneyness | null;
}

/** [contract] GET /api/v1/market/underlyings -> items[] */
export interface Underlying {
	id: number;
	symbol: string;
	name: string | null;
	isin: string | null;
	insCode: string | null;
	kind: UnderlyingKind;
}

/** [proposed] GET /api/v1/market/underlyings/:symbol */
export interface UnderlyingDetail extends Underlying {
	lastPrice: number | null;
	closePrice: number | null;
	prevClose: number | null;
	/** Distinct expiries that have contracts, ascending. "YYYY-MM-DD". */
	expiryDates: string[];
}

/** [proposed] GET /api/v1/market/expiries -> items[]: feeds the expiry filter. */
export interface ExpiryInfo {
	expiryDate: string; // "YYYY-MM-DD" (a date; no time part is needed)
	contractCount: number;
}

/** [contract] list envelope used by /market/options and /market/underlyings. */
export interface ListResponse<T> {
	count: number; // items in this response
	items: T[];
}

/** [proposed] the same envelope plus paging, for the market watch. */
export interface PagedResponse<T> extends ListResponse<T> {
	total: number; // matching rows over all pages
	page: number; // 1-based
	pageSize: number;
}

/** Columns the market watch can be sorted by (values of the `sortBy` param). */
export type MarketSortKey =
	| "symbol"
	| "underlyingSymbol"
	| "optionType"
	| "strikePrice"
	| "expiryDate"
	| "lastPrice"
	| "bestBid"
	| "volume"
	| "value"
	| "openInterest"
	| "impliedVolatility"
	| "delta"
	| "priceGapPercent"
	| "breakEven"
	| "leverage"
	| "moneyness";

/**
 * Query of the market watch: GET /api/v1/market/options
 * [contract] underlying, type, underlyingKind   [proposed] the rest
 * (the contract has `limit`, which `pageSize` replaces).
 */
export interface MarketWatchParams {
	underlying?: string; // symbol, exact
	type?: OptionType;
	underlyingKind?: UnderlyingKind;
	expiry?: string; // "YYYY-MM-DD"
	page?: number;
	pageSize?: number;
	sortBy?: MarketSortKey;
	sortDir?: SortDir;
}

/** The error body the backend sends on failure: { "error": "message" }. */
export interface ApiErrorBody {
	error: string;
}
