// A small simulated option market: 4 underlyings, 3 expiries each, 6 strikes
// per expiry, a call and a put at each strike (144 contracts).
//
// The structure (symbols, strikes, expiries) is fixed. The numbers move: every
// call to snapshot() lets the underlying prices random-walk a little and the
// option prices, volumes and open interest follow, so the 30-second refresh on
// the pages is visibly doing something. Prices are integers in rial.

import type {
	Moneyness,
	OptionQuote,
	OptionType,
	Underlying,
	UnderlyingDetail,
	UnderlyingKind,
} from "@/types/marketTypes";
import { bsDelta, bsPrice } from "./blackScholes";

const RATE = 0.25; // annual risk-free rate, decimal
const CONTRACT_SIZE = 1000;
const ATM_BAND = 0.005; // same band as backend/internal/options/metrics.go
const DAY_MS = 86_400_000;
const MAX_DAILY_MOVE = 0.04; // price limit of the exchange, roughly

interface UnderlyingDef {
	id: number;
	symbol: string;
	name: string;
	kind: UnderlyingKind;
	isin: string;
	insCode: string;
	/** Short form used inside option symbols, e.g. خود -> ضخود9001. */
	prefix: string;
	spot0: number;
	strikeStep: number;
	/** Historical volatility (drives the Black-Scholes "theoretical" price). */
	hv: number;
	/** Base implied volatility around the money. */
	iv0: number;
	/** Days from today to each expiry. They differ per underlying on purpose, so
	 *  some underlying + expiry filter combinations really are empty. */
	expiryDays: number[];
}

const DEFS: UnderlyingDef[] = [
	{
		id: 1, symbol: "خودرو", name: "ایران‌خودرو", kind: "stock",
		isin: "IRO1IKCO0001", insCode: "778253364357513", prefix: "خود",
		spot0: 709, strikeStep: 50, hv: 0.42, iv0: 0.46, expiryDays: [12, 40, 68],
	},
	{
		id: 2, symbol: "وبملت", name: "بانک ملت", kind: "stock",
		isin: "IRO1BMLT0001", insCode: "35366681030756042", prefix: "ملت",
		spot0: 1526, strikeStep: 100, hv: 0.38, iv0: 0.41, expiryDays: [12, 40, 96],
	},
	{
		id: 3, symbol: "شپنا", name: "پالایش نفت اصفهان", kind: "stock",
		isin: "IRO1PNES0001", insCode: "46348559193224090", prefix: "شپنا",
		spot0: 9300, strikeStep: 500, hv: 0.36, iv0: 0.39, expiryDays: [26, 40, 68],
	},
	{
		id: 4, symbol: "اهرم", name: "صندوق اهرمی کاریزما", kind: "fund",
		isin: "IRT1AHRM0001", insCode: "17914401175772326", prefix: "هرم",
		spot0: 94568, strikeStep: 5000, hv: 0.3, iv0: 0.34, expiryDays: [12, 54, 96],
	},
];

interface ContractState {
	contractId: number;
	def: UnderlyingDef;
	type: OptionType;
	symbol: string;
	strike: number;
	expiryDate: string; // "YYYY-MM-DD"
	days: number;
	sigma0: number; // this contract's own implied volatility (smile)
	ivNoise: number;
	prevClose: number;
	closePrice: number;
	volume: number;
	openInterest: number;
	volumeRate: number; // how fast volume grows per refresh
}

interface UnderlyingState {
	def: UnderlyingDef;
	spot: number;
	prevClose: number;
	closePrice: number;
}

// ---- deterministic structure ------------------------------------------------

/** Small seeded generator, so the structure is the same on every page load. */
function mulberry32(seed: number): () => number {
	let a = seed;
	return () => {
		a |= 0;
		a = (a + 0x6d2b79f5) | 0;
		let t = Math.imul(a ^ (a >>> 15), 1 | a);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

function gauss(rnd: () => number = Math.random): number {
	const u = Math.max(rnd(), 1e-9);
	return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * rnd());
}

const clamp = (x: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, x));

function dateInDays(days: number): string {
	return new Date(Date.now() + days * DAY_MS).toISOString().slice(0, 10);
}

const underlyings = new Map<string, UnderlyingState>();
const contracts: ContractState[] = [];

(function build() {
	const rnd = mulberry32(20261115);
	let contractId = 1;
	for (const def of DEFS) {
		const spot = Math.round(def.spot0 * (1 + gauss(rnd) * 0.008));
		underlyings.set(def.symbol, {
			def,
			spot,
			prevClose: Math.round(def.spot0 * (1 + gauss(rnd) * 0.01)),
			closePrice: Math.round(spot * (1 + gauss(rnd) * 0.003)),
		});
		const center = Math.round(def.spot0 / def.strikeStep) * def.strikeStep;
		let series = 0;
		for (const days of def.expiryDays) {
			for (let j = 0; j < 6; j++) {
				const strike = center + (j - 2) * def.strikeStep;
				series++;
				const code = 9000 + def.id * 100 + series; // call and put share it
				// Volatility smile: higher away from the money, a little higher for longer expiries.
				const smile = 1 + 1.1 * Math.abs(Math.log(strike / def.spot0));
				const term = 1 + 0.03 * def.expiryDays.indexOf(days);
				const sigma0 = clamp(def.iv0 * smile * term, 0.22, 0.78);
				for (const type of ["call", "put"] as const) {
					const T = days / 365;
					const fair = bsPrice(type, spot, strike, T, RATE, sigma0);
					const prev = Math.max(1, Math.round(fair * (1 + gauss(rnd) * 0.04)));
					const liquid = rnd() > 0.15;
					contracts.push({
						contractId: contractId++,
						def,
						type,
						symbol: `${type === "call" ? "ض" : "ط"}${def.prefix}${code}`,
						strike,
						expiryDate: dateInDays(days),
						days,
						sigma0,
						ivNoise: 0,
						prevClose: prev,
						closePrice: Math.max(1, Math.round(prev * (1 + gauss(rnd) * 0.03))),
						volume: liquid ? Math.floor(rnd() * 60_000 * Math.exp(-3 * Math.abs(Math.log(strike / def.spot0)))) : 0,
						openInterest: Math.floor(5_000 + rnd() * 400_000),
						volumeRate: liquid ? 500 + rnd() * 4_000 : 0,
					});
				}
			}
		}
	}
})();

// ---- movement ---------------------------------------------------------------

let lastTick = 0;

/** Moves the market one step. Calls closer than 1 s apart share one step, so
 *  two requests fired together (chain + underlying) see the same prices. */
function tick(): void {
	const now = Date.now();
	if (now - lastTick < 1000) return;
	lastTick = now;
	for (const u of underlyings.values()) {
		const next = u.spot * (1 + gauss() * 0.0035);
		u.spot = Math.round(
			clamp(next, u.prevClose * (1 - MAX_DAILY_MOVE), u.prevClose * (1 + MAX_DAILY_MOVE)),
		);
	}
	for (const c of contracts) {
		c.ivNoise = clamp(c.ivNoise * 0.85 + gauss() * 0.01, -0.07, 0.07);
		if (c.volumeRate > 0) c.volume += Math.floor(Math.random() * c.volumeRate);
		c.openInterest = Math.max(0, c.openInterest + Math.round(gauss() * 400));
	}
}

// ---- reading ----------------------------------------------------------------

function moneyness(type: OptionType, spot: number, strike: number): Moneyness {
	const diff = (spot - strike) / spot;
	if (Math.abs(diff) <= ATM_BAND) return "ATM";
	return (type === "call" && diff > 0) || (type === "put" && diff < 0) ? "ITM" : "OTM";
}

function quoteOf(c: ContractState, nowIso: string): OptionQuote {
	const u = underlyings.get(c.def.symbol)!;
	const spot = u.spot;
	const days = Math.max(c.days, 1 / 24);
	const T = days / 365;
	const sigma = clamp(c.sigma0 * (1 + c.ivNoise), 0.2, 0.8);

	const fair = bsPrice(c.type, spot, c.strike, T, RATE, sigma);
	const last = Math.max(1, Math.round(fair));
	const bid = last < 3 ? null : Math.max(1, Math.round(last * 0.985));
	const ask = Math.round(last * 1.015) + 1;
	const theo = bsPrice(c.type, spot, c.strike, T, RATE, c.def.hv);
	const avgPrice = (last + c.prevClose) / 2;

	return {
		contractId: c.contractId,
		symbol: c.symbol,
		underlyingId: c.def.id,
		underlyingSymbol: c.def.symbol,
		underlyingKind: c.def.kind,
		optionType: c.type,
		strikePrice: c.strike,
		expiryDate: `${c.expiryDate}T00:00:00Z`,
		contractSize: CONTRACT_SIZE,
		lastPrice: last,
		closePrice: c.closePrice,
		bestBid: bid,
		bestAsk: ask,
		volume: c.volume,
		openInterest: c.openInterest,
		quoteTs: nowIso,

		prevClose: c.prevClose,
		value: c.volume > 0 ? Math.round(c.volume * avgPrice * CONTRACT_SIZE) : 0,

		impliedVolatility: sigma,
		delta: bsDelta(c.type, spot, c.strike, T, RATE, sigma),
		theoreticalPrice: Math.round(theo * 100) / 100,
		priceGapPercent: theo > 0 ? ((last - theo) / theo) * 100 : null,
		breakEven: c.type === "call" ? c.strike + last : c.strike - last,
		leverage: last > 0 ? spot / last : null,
		moneyness: moneyness(c.type, spot, c.strike),
	};
}

/** Every contract, after one market step. */
export function snapshot(): OptionQuote[] {
	tick();
	const nowIso = new Date().toISOString();
	return contracts.map((c) => quoteOf(c, nowIso));
}

export function listUnderlyingRows(kind?: UnderlyingKind): Underlying[] {
	return DEFS.filter((d) => !kind || d.kind === kind).map((d) => ({
		id: d.id,
		symbol: d.symbol,
		name: d.name,
		isin: d.isin,
		insCode: d.insCode,
		kind: d.kind,
	}));
}

/** Distinct expiries over all underlyings, ascending, with their contract counts. */
export function listExpiryRows(): { expiryDate: string; contractCount: number }[] {
	const counts = new Map<string, number>();
	for (const c of contracts) counts.set(c.expiryDate, (counts.get(c.expiryDate) ?? 0) + 1);
	return [...counts.entries()]
		.sort(([a], [b]) => a.localeCompare(b))
		.map(([expiryDate, contractCount]) => ({ expiryDate, contractCount }));
}

export function underlyingDetail(symbol: string): UnderlyingDetail | null {
	tick();
	const u = underlyings.get(symbol);
	if (!u) return null;
	const d = u.def;
	const expiryDates = [
		...new Set(contracts.filter((c) => c.def === d).map((c) => c.expiryDate)),
	].sort();
	return {
		id: d.id,
		symbol: d.symbol,
		name: d.name,
		isin: d.isin,
		insCode: d.insCode,
		kind: d.kind,
		lastPrice: u.spot,
		closePrice: u.closePrice,
		prevClose: u.prevClose,
		expiryDates,
	};
}
