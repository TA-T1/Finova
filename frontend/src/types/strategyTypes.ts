// Strategies (docs/api-contract.md section 2). Money is in rials; a leg's `price` is per share.

export type LegType = "call" | "put" | "underlying";
export type Side = "buy" | "sell";
export type Outlook = "bullish" | "bearish" | "neutral" | "volatile";

export interface StrikeRule {
	rule: "atm" | "otm" | "itm";
	steps?: number;
}

export interface TemplateLeg {
	type: LegType;
	side: Side;
	strike?: StrikeRule; // an underlying leg has none
	ratio: number;
}

/** GET /strategies/templates -> items[] */
export interface StrategyTemplate {
	id: string;
	name: string;
	nameFa: string;
	outlook: Outlook;
	legs: TemplateLeg[];
}

/** A leg as the client sends it (contract 2.1): exactly one of contractId / underlyingId. */
export interface LegInput {
	contractId?: number;
	underlyingId?: number;
	side: Side;
	/** Contracts for an option leg, shares for an underlying leg. At least 1. */
	quantity: number;
	/** Per share, rial. Missing: the server uses the current market price. */
	price?: number;
}

/** A leg as the server returns it: `price` is always filled. */
export interface StrategyLeg extends LegInput {
	price: number;
	type: LegType;
	symbol: string;
	strikePrice: number | null;
	expiryDate: string | null;
	contractSize: number | null;
}

export interface PayoffPoint {
	underlyingPrice: number;
	pnl: number;
}

export interface PositionGreeks {
	delta: number;
	gamma: number;
	vega: number;
	theta: number;
	rho: number;
}

/** Contract 2.2. */
export interface StrategyAnalysis {
	/** > 0: you pay (debit); < 0: you receive (credit). */
	netPremium: number;
	/** null when unbounded. */
	maxProfit: number | null;
	/** A positive amount at risk; null when unbounded. */
	maxLoss: number | null;
	breakEvens: number[];
	payoff: PayoffPoint[];
	greeks: PositionGreeks | null;
}

export interface AnalyzeResponse {
	legs: StrategyLeg[];
	analysis: StrategyAnalysis;
}

export interface BuildRequest {
	templateId: string;
	underlying: string;
	expiry: string;
	quantity?: number;
}

export interface BuildResponse extends AnalyzeResponse {
	templateId: string;
	name: string;
}
