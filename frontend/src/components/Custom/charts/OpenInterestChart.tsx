import { useMemo } from "react";
import { formatInt } from "@/utils/format";
import type { StrikeOpenInterest } from "@/types/marketTypes";
import { linear, niceTicks, shortNumber } from "./scale";

interface Props {
	data: StrikeOpenInterest[];
	/** Underlying price: a line is drawn at the nearest strike. */
	price?: number | null;
	/** Strike of the max pain: highlighted. */
	maxPain?: number | null;
	height?: number;
}

const W = 760;
const M = { top: 16, right: 12, bottom: 44, left: 56 };

/** Open interest by strike: calls (blue) and puts (amber) side by side. */
export default function OpenInterestChart({ data, price, maxPain, height = 280 }: Props) {
	const geo = useMemo(() => {
		if (data.length === 0) return null;
		const top = Math.max(...data.flatMap((d) => [d.callOpenInterest, d.putOpenInterest]), 1);
		const ticks = niceTicks(0, top, 4);
		const sy = linear([0, ticks[ticks.length - 1]], [height - M.bottom, M.top]);
		const band = linear([0, data.length], [M.left, W - M.right]);
		return { ticks, sy, band };
	}, [data, height]);
	if (!geo) return null;
	const { ticks, sy, band } = geo;
	const slot = (W - M.left - M.right) / data.length;
	const barW = Math.max(Math.min(slot * 0.36, 22), 2);
	const nearest = price != null ? data.reduce((b, d, i) => (Math.abs(d.strikePrice - price) < Math.abs(data[b].strikePrice - price) ? i : b), 0) : -1;
	// Label about 10 strikes at most.
	const every = Math.ceil(data.length / 10);

	return (
		<svg style={{ direction: "ltr" }} viewBox={`0 0 ${W} ${height}`} role="img" aria-label="موقعیت‌های باز به تفکیک قیمت اعمال" className="h-auto w-full">
			{ticks.map((t) => (
				<g key={t}>
					<line x1={M.left} x2={W - M.right} y1={sy(t)} y2={sy(t)} className="stroke-neutral-100" />
					<text x={M.left - 8} y={sy(t)} dy="0.32em" textAnchor="end" className="fill-neutral-500 text-[11px] tabular-nums">{shortNumber(t)}</text>
				</g>
			))}
			{data.map((d, i) => {
				const cx = band(i) + slot / 2;
				const pain = d.strikePrice === maxPain;
				return (
					<g key={d.strikePrice}>
						{pain && <rect x={band(i)} y={M.top} width={slot} height={height - M.bottom - M.top} className="fill-violet-100/70" />}
						<rect x={cx - barW - 1} y={sy(d.callOpenInterest)} width={barW} height={height - M.bottom - sy(d.callOpenInterest)} className="fill-sky-500">
							<title>{`${formatInt(d.strikePrice)} · اختیار خرید: ${formatInt(d.callOpenInterest)}`}</title>
						</rect>
						<rect x={cx + 1} y={sy(d.putOpenInterest)} width={barW} height={height - M.bottom - sy(d.putOpenInterest)} className="fill-amber-500">
							<title>{`${formatInt(d.strikePrice)} · اختیار فروش: ${formatInt(d.putOpenInterest)}`}</title>
						</rect>
						{i % every === 0 && (
							<text x={cx} y={height - M.bottom + 14} textAnchor="middle" className="fill-neutral-600 text-[10px] tabular-nums">{shortNumber(d.strikePrice)}</text>
						)}
						{i === nearest && (
							<g>
								<line x1={cx} x2={cx} y1={M.top} y2={height - M.bottom} stroke="#e11d48" strokeDasharray="4 3" />
								<text x={cx} y={height - 6} textAnchor="middle" className="fill-rose-600 text-[10px]">قیمت فعلی</text>
							</g>
						)}
					</g>
				);
			})}
			<line x1={M.left} x2={W - M.right} y1={height - M.bottom} y2={height - M.bottom} className="stroke-neutral-300" />
		</svg>
	);
}
