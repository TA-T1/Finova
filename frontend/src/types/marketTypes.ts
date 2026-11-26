// Every shape the market pages get from the API, as agreed in docs/api-contract.md
// (section 1). This is the ONE place to update when the backend changes.
//
// Conventions from the contract:
//   - camelCase; a field is never omitted, an unknown value is `null`
//   - exchange prices (lastPrice, strikePrice, bestBid, ...) are integers in RIAL
//   - calculated values (marketPrice, timeValue, delta, ...) are per SHARE, not per
//     contract: multiply by contractSize for a contract
//   - volatilities and rates are decimal fractions (0.35 = 35%); names ending in
//     `Percent` are percentages (2.5 = 2.5%)
//   - dates are Gregorian YYYY-MM-DD (Tehran calendar day); timestamps are RFC 3339

export type OptionType = "call" | "put";
export type UnderlyingKind = "stock" | "fund" | "index" | "other";
export type Moneyness = "ITM" | "ATM" | "OTM";
export type PriceSource = "last" | "mid" | "close";
export type SortDir = "asc" | "desc";

/** Calculated fields of one contract (contract 1.5). All `null` before its first metrics run. */
export interface CalculatedFields {
	underlyingPrice: number | null;
	daysToExpiry: number | null;
	moneyness: Moneyness | null;
	marketPrice: number | null;
	priceSource: PriceSource | null;
	intrinsicValue: number | null;
	timeValue: number | null;
	breakEven: number | null;
	/** Move needed from the spot to break even, in percent. */
	breakEvenPercent: number | null;
	leverage: number | null;
	effectiveLeverage: number | null;
	impliedVolatility: number | null;
	historicalVolatility: number | null;
	greeksVolatility: number | null;
	delta: number | null;
	gamma: number | null;
	vega: number | null;
	theta: number | null;
	rho: number | null;
	/** Black-Scholes price at HV20, per share. */
	theoreticalPrice: number | null;
	priceGap: number | null;
	/** (market - theoretical) / theoretical x 100. */
	priceGapPercent: number | null;
	metricsTs: string | null;
}

/** The exchange quote of a contract (contract 1.5). */
export interface QuoteFields {
	lastPrice: number | null;
	closePrice: number | null;
	bestBid: number | null;
	bestAsk: number | null;
	volume: number | null;
	openInterest: number | null;
	quoteTs: string | null;
	/** Previous day's price; `null` for quotes stored before the backend recorded it. */
	prevClose: number | null;
	/** Traded value today, rial. */
	value: number | null;
}

/** One row of GET /market/options. */
export interface OptionQuote extends QuoteFields, CalculatedFields {
	contractId: number;
	symbol: string; // e.g. ضخود1234
	underlyingId: number;
	underlyingSymbol: string;
	underlyingKind: UnderlyingKind;
	optionType: OptionType;
	strikePrice: number;
	expiryDate: string; // YYYY-MM-DD
	contractSize: number;
}

/** GET /market/options: the list plus the paging block. */
export interface PagedResponse<T> {
	items: T[];
	count: number; // items on this page
	page: number; // 1-based
	pageSize: number;
	total: number; // rows over all pages
	totalPages: number;
}

/** Sortable fields of /market/options (contract 1.5). Null values sort last in both directions. */
export type MarketSortKey =
	| "symbol"
	| "underlyingSymbol"
	| "optionType"
	| "strikePrice"
	| "expiryDate"
	| "daysToExpiry"
	| "lastPrice"
	| "volume"
	| "openInterest"
	| "impliedVolatility"
	| "delta"
	| "leverage"
	| "effectiveLeverage"
	| "timeValue"
	| "intrinsicValue"
	| "priceGapPercent"
	| "breakEvenPercent"
	| "bestBid"
	| "value"
	| "breakEven"
	| "moneyness";

/** Query of GET /market/options. `limit` no longer exists: use pageSize. */
export interface MarketWatchParams {
	underlying?: string; // symbol
	underlyingKind?: UnderlyingKind;
	type?: OptionType;
	expiry?: string; // YYYY-MM-DD
	sort?: MarketSortKey;
	order?: SortDir;
	page?: number;
	pageSize?: number; // default 50, maximum 500
}

/** One side of a chain row (contract 1.6). The strike and expiry are on the row / the response. */
export interface ChainSide extends QuoteFields, Omit<CalculatedFields, "underlyingPrice" | "daysToExpiry"> {
	symbol: string;
	/** Needed to put the contract in a strategy. */
	contractId: number;
}

export interface ChainStrike {
	strikePrice: number;
	call: ChainSide | null;
	put: ChainSide | null;
}

/** GET /market/chain?underlying=&expiry= */
export interface ChainResponse {
	underlyingSymbol: string;
	expiryDate: string;
	daysToExpiry: number | null;
	underlyingPrice: number | null;
	strikes: ChainStrike[];
}

/** GET /market/underlyings -> items[] */
export interface Underlying {
	id: number;
	symbol: string;
	name: string | null;
	isin: string | null;
	insCode: string | null;
	kind: UnderlyingKind;
}

export interface ListResponse<T> {
	count: number;
	items: T[];
}

/** An expiry of GET /market/expiries (and of the symbol page). */
export interface ExpiryInfo {
	expiryDate: string;
	daysToExpiry?: number; // only /market/expiries has it
	contractCount: number;
}

/** GET /market/expiries?underlying= */
export interface ExpiriesResponse {
	underlyingSymbol: string | null;
	items: (ExpiryInfo & { daysToExpiry: number })[];
}

export interface StrikeOpenInterest {
	strikePrice: number;
	callOpenInterest: number;
	putOpenInterest: number;
}

/** GET /market/underlyings/:symbol: the symbol page (contract 1.3). */
export interface SymbolDetail {
	symbol: string;
	name: string | null;
	kind: UnderlyingKind;
	price: number | null;
	priceChange: number | null;
	priceChangePercent: number | null;
	expiries: ExpiryInfo[];
	openInterest: { calls: number; puts: number; pcr: number | null };
	/** Totals over all active expiries. */
	strikeOpenInterest: StrikeOpenInterest[];
	/** Of the nearest active expiry. */
	maxPain: { expiryDate: string; strikePrice: number; totalPayout: number } | null;
}

/** GET /market/status */
export interface MarketStatus {
	marketOpen: boolean;
	lastUpdate: string | null;
	timezone: string;
}

/** The error body the backend sends on failure: { "error": "message" }. */
export interface ApiErrorBody {
	error: string;
}
