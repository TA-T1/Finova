// Black-Scholes price and delta, used only by the mock market so that the
// numbers on screen are consistent (price, IV, delta and break-even agree).
// The real backend computes these in backend/internal/options.

import type { OptionType } from "@/types/marketTypes";

/** Standard normal CDF (Abramowitz & Stegun 26.2.17, error < 1e-7). */
export function normCdf(x: number): number {
	const t = 1 / (1 + 0.2316419 * Math.abs(x));
	const d = 0.3989422804 * Math.exp((-x * x) / 2);
	const p =
		d *
		t *
		(0.31938153 +
			t * (-0.356563782 + t * (1.781477937 + t * (-1.821255978 + t * 1.330274429))));
	return x > 0 ? 1 - p : p;
}

function d1d2(S: number, K: number, T: number, r: number, sigma: number) {
	const sd = sigma * Math.sqrt(T);
	const d1 = (Math.log(S / K) + (r + (sigma * sigma) / 2) * T) / sd;
	return { d1, d2: d1 - sd };
}

/** Price per share. T in years, r and sigma as decimal fractions. */
export function bsPrice(
	kind: OptionType,
	S: number,
	K: number,
	T: number,
	r: number,
	sigma: number,
): number {
	if (T <= 0 || sigma <= 0) {
		return kind === "call" ? Math.max(S - K, 0) : Math.max(K - S, 0);
	}
	const { d1, d2 } = d1d2(S, K, T, r, sigma);
	const disc = Math.exp(-r * T);
	return kind === "call"
		? S * normCdf(d1) - K * disc * normCdf(d2)
		: K * disc * normCdf(-d2) - S * normCdf(-d1);
}

/** Call delta is N(d1) (0..1); put delta is N(d1) - 1 (-1..0). */
export function bsDelta(
	kind: OptionType,
	S: number,
	K: number,
	T: number,
	r: number,
	sigma: number,
): number {
	if (T <= 0 || sigma <= 0) {
		const itm = kind === "call" ? S > K : S < K;
		return itm ? (kind === "call" ? 1 : -1) : 0;
	}
	const { d1 } = d1d2(S, K, T, r, sigma);
	return kind === "call" ? normCdf(d1) : normCdf(d1) - 1;
}

const normPdf = (x: number) => 0.3989422804 * Math.exp((-x * x) / 2);

export interface Greeks {
	delta: number;
	gamma: number;
	/** per +1 volatility point */
	vega: number;
	/** per calendar day */
	theta: number;
	/** per +1 rate point */
	rho: number;
}

/** Greeks in the units of docs/api-contract.md: vega per vol point, theta per day, rho per rate point. */
export function bsGreeks(kind: OptionType, S: number, K: number, T: number, r: number, sigma: number): Greeks {
	if (T <= 0 || sigma <= 0) {
		return { delta: bsDelta(kind, S, K, T, r, sigma), gamma: 0, vega: 0, theta: 0, rho: 0 };
	}
	const { d1, d2 } = d1d2(S, K, T, r, sigma);
	const disc = Math.exp(-r * T);
	const sd = sigma * Math.sqrt(T);
	const call = kind === "call";
	const thetaYear =
		-(S * normPdf(d1) * sigma) / (2 * Math.sqrt(T)) +
		(call ? -r * K * disc * normCdf(d2) : r * K * disc * normCdf(-d2));
	return {
		delta: call ? normCdf(d1) : normCdf(d1) - 1,
		gamma: normPdf(d1) / (S * sd),
		vega: (S * normPdf(d1) * Math.sqrt(T)) / 100,
		theta: thetaYear / 365,
		rho: ((call ? 1 : -1) * K * T * disc * normCdf(call ? d2 : -d2)) / 100,
	};
}
