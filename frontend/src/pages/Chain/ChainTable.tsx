import { Fragment, useMemo } from "react";
import ChangeBadge from "@/components/Custom/ChangeBadge";
import NumberCell from "@/components/Custom/NumberCell";
import { cn } from "@/lib/utils";
import type { OptionQuote } from "@/types/marketTypes";
import { isInTheMoney, locatePrice, pairByStrike } from "./chainUtils";
import { changePercent, formatInt } from "@/utils/format";

// Both sides show the same eight fields, mirrored around the strike column.
// The page is RTL, so the FIRST column in the markup is on the RIGHT:
//
//   call side (right)                         strike    put side (left)
//   delta IV  OI  vol ask bid %chg last   |  strike  |  last %chg bid ask vol OI  IV delta
//   outer edge ........................ next to strike ........................ outer edge
interface Field {
	id: string;
	label: string;
	cell: (q: OptionQuote) => React.ReactNode;
}

const FIELDS: Record<string, Field> = {
	last: { id: "last", label: "آخرین", cell: (q) => <NumberCell value={q.lastPrice} className="font-medium" /> },
	change: { id: "change", label: "تغییر٪", cell: (q) => <ChangeBadge value={changePercent(q.lastPrice, q.prevClose)} /> },
	bid: { id: "bid", label: "بهترین خرید", cell: (q) => <NumberCell value={q.bestBid} className="text-emerald-700" /> },
	ask: { id: "ask", label: "بهترین فروش", cell: (q) => <NumberCell value={q.bestAsk} className="text-rose-600" /> },
	volume: { id: "volume", label: "حجم", cell: (q) => <NumberCell value={q.volume} /> },
	oi: { id: "oi", label: "موقعیت باز", cell: (q) => <NumberCell value={q.openInterest} /> },
	iv: { id: "iv", label: "IV", cell: (q) => <NumberCell value={q.impliedVolatility} kind="fraction" /> },
	delta: { id: "delta", label: "دلتا", cell: (q) => <NumberCell value={q.delta} kind="decimal" digits={2} /> },
};

const INNER_TO_OUTER = ["last", "change", "bid", "ask", "volume", "oi", "iv", "delta"].map((k) => FIELDS[k]);
const CALL_FIELDS = [...INNER_TO_OUTER].reverse(); // outer edge first, ends next to the strike
const PUT_FIELDS = INNER_TO_OUTER; // starts next to the strike

const COLS = CALL_FIELDS.length + 1 + PUT_FIELDS.length;

interface Props {
	items: OptionQuote[];
	underlyingPrice: number | null;
	loading?: boolean;
}

const sideHead = "sticky z-20 border-b border-neutral-200 bg-neutral-50 px-2 py-1.5 text-center text-xs font-semibold whitespace-nowrap text-neutral-600";

export default function ChainTable({ items, underlyingPrice, loading }: Props) {
	const rows = useMemo(() => pairByStrike(items), [items]);

	// The strike nearest to the underlying price, and where the price falls
	// between two strikes (the line is drawn after index `lineAfter`).
	const { nearest, lineAfter } = useMemo(
		() => locatePrice(rows.map((r) => r.strike), underlyingPrice),
		[rows, underlyingPrice],
	);

	const sideCell = (q: OptionQuote | undefined, f: Field, itm: boolean, key: string) => (
		<td
			key={key}
			className={cn(
				"border-b border-neutral-100 px-2 py-1.5 text-center whitespace-nowrap",
				itm ? (q?.optionType === "call" ? "bg-sky-50" : "bg-amber-50") : "bg-white",
			)}
		>
			{q ? f.cell(q) : <span className="text-neutral-300">—</span>}
		</td>
	);

	// The underlying's price as its own thin row between the two strikes it falls
	// between, so it never covers any data.
	const priceLine = (
		<tr key="price-line" aria-label="قیمت دارایی پایه">
			<td colSpan={COLS} className="border-y-2 border-dashed border-rose-300 bg-rose-50 p-0">
				<div className="flex h-6 items-center justify-center">
					<span className="rounded-full bg-rose-500 px-3 py-px text-xs font-medium whitespace-nowrap text-white">
						قیمت دارایی پایه: {formatInt(underlyingPrice)}
					</span>
				</div>
			</td>
		</tr>
	);

	return (
		<div className="max-h-[70vh] overflow-auto">
			<table className={cn("w-full min-w-[1000px] border-separate border-spacing-0 text-sm", loading && "opacity-60 transition-opacity")}>
				<thead>
					<tr>
						<th colSpan={CALL_FIELDS.length} className={cn(sideHead, "top-0 h-8 bg-sky-50 text-sky-800")}>
							اختیار خرید (Call)
						</th>
						<th rowSpan={2} className={cn(sideHead, "top-0 border-x border-neutral-200 text-sm text-neutral-800")}>
							قیمت اعمال
						</th>
						<th colSpan={PUT_FIELDS.length} className={cn(sideHead, "top-0 h-8 bg-amber-50 text-amber-800")}>
							اختیار فروش (Put)
						</th>
					</tr>
					<tr>
						{CALL_FIELDS.map((f) => (
							<th key={`c${f.id}`} className={cn(sideHead, "top-8")}>{f.label}</th>
						))}
						{PUT_FIELDS.map((f) => (
							<th key={`p${f.id}`} className={cn(sideHead, "top-8")}>{f.label}</th>
						))}
					</tr>
				</thead>
				<tbody>
					{lineAfter === -1 && priceLine}
					{rows.map((r, i) => {
						const callItm = r.call ? isInTheMoney("call", r.strike, underlyingPrice) : false;
						const putItm = r.put ? isInTheMoney("put", r.strike, underlyingPrice) : false;
						const isNearest = r.strike === nearest;
						return (
							<Fragment key={r.strike}>
								<tr>
									{CALL_FIELDS.map((f) => sideCell(r.call, f, callItm, `c${f.id}`))}
									<td
										className={cn(
											"border-x border-b border-neutral-200 bg-neutral-100 px-3 py-1.5 text-center font-bold whitespace-nowrap text-neutral-900",
											isNearest && "bg-yellow-100 ring-2 ring-yellow-400 ring-inset",
										)}
										title={isNearest ? "نزدیک‌ترین قیمت اعمال به قیمت دارایی پایه" : undefined}
									>
										<NumberCell value={r.strike} />
									</td>
									{PUT_FIELDS.map((f) => sideCell(r.put, f, putItm, `p${f.id}`))}
								</tr>
								{lineAfter === i && i < rows.length - 1 && priceLine}
							</Fragment>
						);
					})}
					{lineAfter === rows.length - 1 && rows.length > 0 && priceLine}
				</tbody>
			</table>
		</div>
	);
}
