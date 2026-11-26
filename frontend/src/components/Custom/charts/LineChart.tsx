import { useId, useMemo } from "react";
import { cn } from "@/lib/utils";
import { linear, niceTicks, shortNumber } from "./scale";

export interface Point {
	x: number;
	y: number;
}

export interface VLine {
	x: number;
	label: string;
	color?: string;
	dashed?: boolean;
}

interface Props {
	points: Point[];
	/** Format an x tick; default is the plain number. */
	formatX?: (x: number) => string;
	formatY?: (y: number) => string;
	/** Draw a horizontal line at y = 0 and colour the area green above it and red below. */
	signed?: boolean;
	/** Draw the line as steps (equity after each trade) instead of a straight line. */
	step?: boolean;
	vlines?: VLine[];
	height?: number;
	ariaLabel: string;
	className?: string;
}

const W = 760;
const M = { top: 30, right: 16, bottom: 28, left: 64 };

/** A line chart in plain SVG. The axes are left-to-right even on an RTL page (numbers and dates are LTR). */
export default function LineChart({ points, formatX = String, formatY = shortNumber, signed, step, vlines = [], height = 260, ariaLabel, className }: Props) {
	const id = useId();
	const geo = useMemo(() => {
		if (points.length === 0) return null;
		const xs = points.map((p) => p.x);
		const ys = points.map((p) => p.y);
		const xMin = Math.min(...xs);
		const xMax = Math.max(...xs);
		let yMin = Math.min(...ys, ...(signed ? [0] : []));
		let yMax = Math.max(...ys, ...(signed ? [0] : []));
		if (yMin === yMax) {
			yMin -= 1;
			yMax += 1;
		}
		const yTicks = niceTicks(yMin, yMax, 5);
		yMin = Math.min(yMin, yTicks[0]);
		yMax = Math.max(yMax, yTicks[yTicks.length - 1]);
		const sx = linear([xMin, xMax], [M.left, W - M.right]);
		const sy = linear([yMin, yMax], [height - M.bottom, M.top]);
		return { sx, sy, yTicks, xTicks: niceTicks(xMin, xMax, 6).filter((t) => t >= xMin && t <= xMax) };
	}, [points, signed, height]);

	if (!geo) return null;
	const { sx, sy, yTicks, xTicks } = geo;
	const y0 = sy(0);

	const coords: [number, number][] = [];
	points.forEach((p, i) => {
		if (step && i > 0) coords.push([sx(p.x), sy(points[i - 1].y)]);
		coords.push([sx(p.x), sy(p.y)]);
	});
	const path = coords.map(([x, y], i) => `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
	const base = signed ? y0 : height - M.bottom;
	const area = `${path} L${coords[coords.length - 1][0].toFixed(1)},${base} L${coords[0][0].toFixed(1)},${base} Z`;

	return (
		<svg
			style={{ direction: "ltr" }}
			viewBox={`0 0 ${W} ${height}`}
			role="img"
			aria-label={ariaLabel}
			className={cn("h-auto w-full", className)}
		>
			<defs>
				<clipPath id={`${id}-up`}><rect x={M.left} y={0} width={W - M.left} height={Math.max(y0, 0)} /></clipPath>
				<clipPath id={`${id}-down`}><rect x={M.left} y={y0} width={W - M.left} height={Math.max(height - y0, 0)} /></clipPath>
			</defs>
			{yTicks.map((t) => (
				<g key={`y${t}`}>
					<line x1={M.left} x2={W - M.right} y1={sy(t)} y2={sy(t)} className="stroke-neutral-100" />
					<text x={M.left - 8} y={sy(t)} dy="0.32em" textAnchor="end" className="fill-neutral-500 text-[11px] tabular-nums">{formatY(t)}</text>
				</g>
			))}
			{xTicks.map((t) => (
				<text key={`x${t}`} x={sx(t)} y={height - 8} textAnchor="middle" className="fill-neutral-500 text-[11px] tabular-nums">{formatX(t)}</text>
			))}
			{signed ? (
				<>
					<path d={area} clipPath={`url(#${id}-up)`} className="fill-emerald-500/20" />
					<path d={area} clipPath={`url(#${id}-down)`} className="fill-rose-500/20" />
					<line x1={M.left} x2={W - M.right} y1={y0} y2={y0} className="stroke-neutral-400" />
				</>
			) : (
				<path d={area} className="fill-sky-500/10" />
			)}
			{vlines.map((v, i) => (
				<g key={`v${v.label}${v.x}`}>
					<line x1={sx(v.x)} x2={sx(v.x)} y1={M.top - 4 - (i % 2) * 0} y2={height - M.bottom} stroke={v.color ?? "#64748b"} strokeDasharray={v.dashed === false ? undefined : "4 3"} />
					{/* Neighbouring labels sit on two levels so they do not overlap. */}
					<text x={sx(v.x)} y={M.top - 8 - (i % 2) * 12} textAnchor="middle" className="fill-neutral-600 text-[10px]">{v.label}</text>
				</g>
			))}
			<path d={path} fill="none" strokeWidth={2} strokeLinejoin="round" className="stroke-sky-600" />
		</svg>
	);
}
