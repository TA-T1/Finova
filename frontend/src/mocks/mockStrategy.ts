// Mock of the strategy endpoints (docs/api-contract.md section 2): templates, build and
// analyze. The server does the same sums; payoff.ts holds them.

import type { ChainResponse } from "@/types/marketTypes";
import type {
	AnalyzeResponse,
	BuildRequest,
	BuildResponse,
	LegInput,
	StrategyLeg,
	StrategyTemplate,
} from "@/types/strategyTypes";
import { summarize, type PayoffLeg } from "@/utils/payoff";
import { chainOf, hasUnderlying, snapshot } from "./mockMarket";

const A = { rule: "atm" } as const;
const O1 = { rule: "otm", steps: 1 } as const;

/** The draft list of the contract (2.3). Names are Persian, as the page shows them. */
export const TEMPLATES: StrategyTemplate[] = [
	{ id: "long-call", name: "Long call", nameFa: "خرید اختیار خرید", outlook: "bullish",
		legs: [{ type: "call", side: "buy", strike: A, ratio: 1 }] },
	{ id: "long-put", name: "Long put", nameFa: "خرید اختیار فروش", outlook: "bearish",
		legs: [{ type: "put", side: "buy", strike: A, ratio: 1 }] },
	{ id: "covered-call", name: "Covered call", nameFa: "کاورد کال", outlook: "neutral",
		legs: [{ type: "underlying", side: "buy", ratio: 1 }, { type: "call", side: "sell", strike: O1, ratio: 1 }] },
	{ id: "protective-put", name: "Protective put", nameFa: "پوت محافظ", outlook: "bullish",
		legs: [{ type: "underlying", side: "buy", ratio: 1 }, { type: "put", side: "buy", strike: A, ratio: 1 }] },
	{ id: "collar", name: "Collar", nameFa: "کولار", outlook: "neutral",
		legs: [{ type: "underlying", side: "buy", ratio: 1 }, { type: "put", side: "buy", strike: O1, ratio: 1 }, { type: "call", side: "sell", strike: O1, ratio: 1 }] },
	{ id: "bull-call-spread", name: "Bull call spread", nameFa: "بول کال اسپرد", outlook: "bullish",
		legs: [{ type: "call", side: "buy", strike: A, ratio: 1 }, { type: "call", side: "sell", strike: O1, ratio: 1 }] },
	{ id: "bear-put-spread", name: "Bear put spread", nameFa: "بیر پوت اسپرد", outlook: "bearish",
		legs: [{ type: "put", side: "buy", strike: A, ratio: 1 }, { type: "put", side: "sell", strike: O1, ratio: 1 }] },
	{ id: "long-straddle", name: "Long straddle", nameFa: "استرادل", outlook: "volatile",
		legs: [{ type: "call", side: "buy", strike: A, ratio: 1 }, { type: "put", side: "buy", strike: A, ratio: 1 }] },
	{ id: "long-strangle", name: "Long strangle", nameFa: "استرانگل", outlook: "volatile",
		legs: [{ type: "call", side: "buy", strike: O1, ratio: 1 }, { type: "put", side: "buy", strike: O1, ratio: 1 }] },
];

export class StrategyMockError extends Error {
	status: number;
	constructor(message: string, status: number) {
		super(message);
		this.status = status;
	}
}

/** The listed strike for a rule: ATM = nearest (the lower on a tie); otm/itm = n listed strikes away. */
function pickStrike(chain: ChainResponse, type: "call" | "put", rule: { rule: string; steps?: number }): number | null {
	const grid = chain.strikes.filter((s) => s[type]).map((s) => s.strikePrice);
	const spot = chain.underlyingPrice;
	if (grid.length === 0 || spot === null) return null;
	let atm = 0;
	grid.forEach((k, i) => {
		if (Math.abs(k - spot) < Math.abs(grid[atm] - spot)) atm = i;
	});
	if (rule.rule === "atm") return grid[atm];
	const up = (type === "call") === (rule.rule === "otm");
	const i = up ? atm + (rule.steps ?? 1) : atm - (rule.steps ?? 1);
	return grid[i] ?? null;
}

function legFromContract(contractId: number, side: "buy" | "sell", quantity: number, price?: number): StrategyLeg {
	const q = snapshot().find((r) => r.contractId === contractId);
	if (!q) throw new StrategyMockError(`قرارداد ${contractId} پیدا نشد`, 404);
	return {
		contractId, side, quantity,
		price: price ?? q.marketPrice ?? q.lastPrice ?? 0,
		type: q.optionType, symbol: q.symbol, strikePrice: q.strikePrice, expiryDate: q.expiryDate, contractSize: q.contractSize,
	};
}

function legFromUnderlying(underlyingId: number, side: "buy" | "sell", quantity: number, price?: number): StrategyLeg {
	const q = snapshot().find((r) => r.underlyingId === underlyingId);
	if (!q) throw new StrategyMockError(`دارایی ${underlyingId} پیدا نشد`, 404);
	return {
		underlyingId, side, quantity, price: price ?? q.underlyingPrice ?? 0,
		type: "underlying", symbol: q.underlyingSymbol, strikePrice: null, expiryDate: null, contractSize: null,
	};
}

export function analyze(inputs: LegInput[]): AnalyzeResponse {
	if (inputs.length === 0) throw new StrategyMockError("حداقل یک پا لازم است", 400);
	const legs = inputs.map((l) => {
		if ((l.contractId === undefined) === (l.underlyingId === undefined)) {
			throw new StrategyMockError("هر پا باید دقیقاً یکی از contractId و underlyingId را داشته باشد", 400);
		}
		if (!(l.quantity >= 1)) throw new StrategyMockError("تعداد باید دست‌کم ۱ باشد", 400);
		return l.contractId !== undefined
			? legFromContract(l.contractId, l.side, l.quantity, l.price)
			: legFromUnderlying(l.underlyingId!, l.side, l.quantity, l.price);
	});
	const expiries = new Set(legs.map((l) => l.expiryDate).filter(Boolean));
	if (expiries.size > 1) throw new StrategyMockError("همهٔ پاها باید سررسید یکسان داشته باشند", 422);

	const all = snapshot();
	const spotOf = (l: StrategyLeg) =>
		all.find((r) => (l.contractId !== undefined ? r.contractId === l.contractId : r.underlyingId === l.underlyingId))?.underlyingPrice ?? 0;
	const spot = spotOf(legs[0]);
	const payoffLegs: PayoffLeg[] = legs.map((l) => ({
		type: l.type, side: l.side, quantity: l.quantity, price: l.price, strike: l.strikePrice, contractSize: l.contractSize,
	}));
	const s = summarize(payoffLegs, spot);

	// Position Greeks: +-quantity x contractSize x greek per option leg; a share leg is delta +-quantity.
	let greeks: AnalyzeResponse["analysis"]["greeks"] = { delta: 0, gamma: 0, vega: 0, theta: 0, rho: 0 };
	for (const l of legs) {
		const sign = l.side === "buy" ? 1 : -1;
		if (l.type === "underlying") {
			greeks.delta += sign * l.quantity;
			continue;
		}
		const q = all.find((r) => r.contractId === l.contractId)!;
		if (q.delta === null || q.gamma === null || q.vega === null || q.theta === null || q.rho === null) {
			greeks = null;
			break;
		}
		const n = sign * l.quantity * (l.contractSize ?? 1);
		greeks.delta += n * q.delta;
		greeks.gamma += n * q.gamma;
		greeks.vega += n * q.vega;
		greeks.theta += n * q.theta;
		greeks.rho += n * q.rho;
	}
	return {
		legs,
		analysis: {
			netPremium: s.netPremium, maxProfit: s.maxProfit, maxLoss: s.maxLoss,
			breakEvens: s.breakEvens, payoff: s.payoff, greeks,
		},
	};
}

export function build(req: BuildRequest): BuildResponse {
	const tpl = TEMPLATES.find((t) => t.id === req.templateId);
	if (!tpl) throw new StrategyMockError(`الگوی «${req.templateId}» پیدا نشد`, 404);
	if (!hasUnderlying(req.underlying)) throw new StrategyMockError(`دارایی پایه «${req.underlying}» پیدا نشد`, 404);
	const chain = chainOf(req.underlying, req.expiry);
	if (!chain) throw new StrategyMockError("برای این سررسید قراردادی وجود ندارد", 404);
	const qty = req.quantity ?? 1;
	const all = snapshot();
	const inputs: LegInput[] = [];
	for (const leg of tpl.legs) {
		if (leg.type === "underlying") {
			const u = all.find((r) => r.underlyingSymbol === req.underlying)!;
			inputs.push({ underlyingId: u.underlyingId, side: leg.side, quantity: leg.ratio * qty * u.contractSize });
			continue;
		}
		const k = pickStrike(chain, leg.type, leg.strike!);
		const side = k === null ? null : chain.strikes.find((s) => s.strikePrice === k)?.[leg.type];
		if (!side) throw new StrategyMockError("برای این الگو قیمت اعمال مناسبی در زنجیره نیست", 422);
		inputs.push({ contractId: side.contractId, side: leg.side, quantity: leg.ratio * qty });
	}
	return { templateId: tpl.id, name: tpl.name, ...analyze(inputs) };
}
