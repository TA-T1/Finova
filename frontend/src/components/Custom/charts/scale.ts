// Small helpers for the SVG charts: axis ticks and linear scales.

/** "Nice" tick values covering [min, max], about `count` of them. */
export function niceTicks(min: number, max: number, count = 5): number[] {
	if (!Number.isFinite(min) || !Number.isFinite(max)) return [0];
	if (min === max) {
		const pad = Math.abs(min) || 1;
		min -= pad / 2;
		max += pad / 2;
	}
	const raw = (max - min) / Math.max(count, 1);
	const mag = 10 ** Math.floor(Math.log10(raw));
	const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? 10 * mag;
	const out: number[] = [];
	for (let v = Math.ceil(min / step) * step; v <= max + step * 1e-9; v += step) out.push(Math.round(v / step) * step);
	return out;
}

export interface Linear {
	(v: number): number;
	domain: [number, number];
}

/** Maps a data range onto a pixel range. */
export function linear(domain: [number, number], range: [number, number]): Linear {
	const [d0, d1] = domain;
	const span = d1 - d0 || 1;
	const f = ((v: number) => range[0] + ((v - d0) / span) * (range[1] - range[0])) as Linear;
	f.domain = domain;
	return f;
}

/** Axis labels for money: 1.2e9 -> "1.2B" style, in short Latin form to keep ticks narrow. */
export function shortNumber(v: number): string {
	const a = Math.abs(v);
	const sign = v < 0 ? "-" : "";
	if (a >= 1e9) return `${sign}${+(a / 1e9).toFixed(2)}B`;
	if (a >= 1e6) return `${sign}${+(a / 1e6).toFixed(2)}M`;
	if (a >= 1e3) return `${sign}${+(a / 1e3).toFixed(1)}K`;
	return `${sign}${+a.toFixed(2)}`;
}
