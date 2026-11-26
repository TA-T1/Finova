// Profit and loss of an option strategy at expiry. Used by the mock strategy API
// (the real server does the same sums, docs/api-contract.md 2.2) and to unit-test
// the numbers. No imports from the app, so it runs anywhere.

export interface PayoffLeg {
	type: "call" | "put" | "underlying";
	side: "buy" | "sell";
	/** Contracts for an option leg, shares for an underlying leg. */
	quantity: number;
	/** Per share, rial. */
	price: number;
	strike: number | null;
	contractSize: number | null;
}

export interface PayoffSummary {
	netPremium: number;
	maxProfit: number | null;
	maxLoss: number | null;
	breakEvens: number[];
	payoff: { underlyingPrice: number; pnl: number }[];
}

const sign = (l: PayoffLeg) => (l.side === "buy" ? 1 : -1);
const units = (l: PayoffLeg) => (l.type === "underlying" ? l.quantity : l.quantity * (l.contractSize ?? 1));

/** Profit or loss of the whole position if the underlying ends at `s`. */
export function pnlAt(legs: PayoffLeg[], s: number): number {
	let total = 0;
	for (const l of legs) {
		let value: number;
		if (l.type === "underlying") value = s;
		else if (l.type === "call") value = Math.max(s - (l.strike ?? 0), 0);
		else value = Math.max((l.strike ?? 0) - s, 0);
		total += sign(l) * units(l) * (value - l.price);
	}
	return total;
}

/** Sum of +-price x quantity x contractSize (a share leg: quantity alone). Positive = debit. */
export function netPremium(legs: PayoffLeg[]): number {
	return legs.reduce((a, l) => a + sign(l) * l.price * units(l), 0);
}

/** Slope of the payoff above the highest strike: calls and shares gain with the price. */
function upperSlope(legs: PayoffLeg[]): number {
	return legs.reduce((a, l) => (l.type === "put" ? a : a + sign(l) * units(l)), 0);
}

export function summarize(legs: PayoffLeg[], spot: number): PayoffSummary {
	const strikes = [...new Set(legs.filter((l) => l.strike !== null).map((l) => l.strike as number))].sort((a, b) => a - b);
	const knots = [0, ...strikes];
	const far = Math.max(spot, ...strikes, 1) * 10;
	const values = knots.map((k) => pnlAt(legs, k));
	const atFar = pnlAt(legs, far);
	const slope = upperSlope(legs);

	// Candidates for the extremes: every knot, and (when bounded) the far point.
	const all = [...values, atFar];
	const maxProfit = slope > 0 ? null : Math.max(...all);
	const maxLoss = slope < 0 ? null : Math.max(-Math.min(...all), 0);

	// Break-evens: where the piecewise-linear payoff crosses 0, segment by segment.
	const pts = [...knots, far];
	const vals = [...values, atFar];
	const be: number[] = [];
	for (let i = 0; i < pts.length - 1; i++) {
		const [x0, x1, y0, y1] = [pts[i], pts[i + 1], vals[i], vals[i + 1]];
		if (y0 === 0 && x0 > 0) be.push(x0);
		else if (y0 * y1 < 0) be.push(x0 + ((0 - y0) * (x1 - x0)) / (y1 - y0));
	}
	const last = vals[vals.length - 1];
	if (last === 0 && !be.includes(pts[pts.length - 1])) be.push(pts[pts.length - 1]);

	// Payoff curve: 41 points from 50% to 150% of the spot, plus every strike.
	const xs = new Set<number>(strikes);
	for (let i = 0; i <= 40; i++) xs.add(spot * (0.5 + i / 40));
	const payoff = [...xs].sort((a, b) => a - b).map((x) => ({ underlyingPrice: x, pnl: pnlAt(legs, x) }));

	return {
		netPremium: netPremium(legs),
		maxProfit,
		maxLoss,
		breakEvens: [...new Set(be.map((x) => Math.round(x * 100) / 100))].sort((a, b) => a - b),
		payoff,
	};
}
