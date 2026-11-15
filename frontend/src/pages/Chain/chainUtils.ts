import type { OptionQuote, OptionType } from "@/types/marketTypes";

/** A call is in the money when strike < underlying price; a put when strike > price. */
export function isInTheMoney(type: OptionType, strike: number, underlyingPrice: number | null): boolean {
	if (underlyingPrice === null) return false;
	return type === "call" ? strike < underlyingPrice : strike > underlyingPrice;
}

export interface StrikeRow {
	strike: number;
	call?: OptionQuote;
	put?: OptionQuote;
}

/** Pairs the call and the put of each strike, ascending by strike. */
export function pairByStrike(items: OptionQuote[]): StrikeRow[] {
	const map = new Map<number, StrikeRow>();
	for (const q of items) {
		const row = map.get(q.strikePrice) ?? { strike: q.strikePrice };
		row[q.optionType] = q;
		map.set(q.strikePrice, row);
	}
	return [...map.values()].sort((a, b) => a.strike - b.strike);
}

/**
 * Where the underlying price sits among the (ascending) strikes:
 *  - nearest: the strike closest to the price
 *  - lineAfter: index of the last strike <= price, so the price line goes right
 *    after that row; -1 when the price is below every strike
 */
export function locatePrice(
	strikes: number[],
	price: number | null,
): { nearest: number | null; lineAfter: number | null } {
	if (price === null || strikes.length === 0) return { nearest: null, lineAfter: null };
	let best = 0;
	let after = -1;
	strikes.forEach((k, i) => {
		if (Math.abs(k - price) < Math.abs(strikes[best] - price)) best = i;
		if (k <= price) after = i;
	});
	return { nearest: strikes[best], lineAfter: after };
}
