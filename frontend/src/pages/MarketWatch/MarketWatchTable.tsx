import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { ChevronDown, ChevronUp, ChevronsUpDown } from "lucide-react";
import ChangeBadge from "@/components/Custom/ChangeBadge";
import MoneynessBadge from "@/components/Custom/MoneynessBadge";
import NumberCell from "@/components/Custom/NumberCell";
import OptionTypeBadge from "@/components/Custom/OptionTypeBadge";
import { cn } from "@/lib/utils";
import type { MarketSortKey, OptionQuote, SortDir } from "@/types/marketTypes";
import { changePercent, daysLeft, formatInt, formatJalali } from "@/utils/format";

interface Column {
	id: string;
	label: string;
	/** Header text under the label, e.g. the second value of a combined column. */
	sub?: string;
	/** The field sent as `sort`; missing when the API cannot sort by this column. */
	sortKey?: MarketSortKey;
	/** The first column stays in view while the table scrolls sideways. */
	sticky?: boolean;
	cell: (q: OptionQuote) => ReactNode;
}

const dash = <span className="text-neutral-300">—</span>;

// The 16 columns of the brief, in order. In RTL the first one is on the right.
const COLUMNS: Column[] = [
	{
		id: "symbol", label: "نماد قرارداد", sortKey: "symbol", sticky: true,
		cell: (q) => <span className="font-semibold text-neutral-900">{q.symbol}</span>,
	},
	{
		id: "underlying", label: "دارایی پایه", sortKey: "underlyingSymbol",
		cell: (q) => (
			<Link
				to={`/symbol/${encodeURIComponent(q.underlyingSymbol)}`}
				className="text-sky-700 hover:underline"
			>
				{q.underlyingSymbol}
			</Link>
		),
	},
	{ id: "type", label: "نوع", sortKey: "optionType", cell: (q) => <OptionTypeBadge type={q.optionType} /> },
	{ id: "strike", label: "قیمت اعمال", sortKey: "strikePrice", cell: (q) => <NumberCell value={q.strikePrice} /> },
	{
		id: "expiry", label: "سررسید", sub: "روز باقی‌مانده", sortKey: "expiryDate",
		cell: (q) => {
			const d = q.daysToExpiry ?? daysLeft(q.expiryDate);
			return (
				<div className="leading-tight">
					<div className="tabular-nums">{formatJalali(q.expiryDate)}</div>
					<div className={cn("text-xs tabular-nums", d <= 7 ? "font-medium text-rose-600" : "text-neutral-500")}>
						{formatInt(d)} روز
					</div>
				</div>
			);
		},
	},
	{
		id: "last", label: "آخرین قیمت", sub: "تغییر٪", sortKey: "lastPrice",
		cell: (q) => (
			<div className="leading-tight">
				<NumberCell value={q.lastPrice} className="font-medium" />
				<div className="text-xs"><ChangeBadge value={changePercent(q.lastPrice, q.prevClose)} /></div>
			</div>
		),
	},
	{
		id: "bidask", label: "بهترین خرید", sub: "بهترین فروش",
		cell: (q) => (
			<div className="leading-tight">
				<NumberCell value={q.bestBid} className="text-emerald-700" />
				<div className="text-xs"><NumberCell value={q.bestAsk} className="text-rose-600" /></div>
			</div>
		),
	},
	{ id: "volume", label: "حجم", sortKey: "volume", cell: (q) => <NumberCell value={q.volume} /> },
	{ id: "value", label: "ارزش معاملات", cell: (q) => <NumberCell value={q.value} kind="compact" /> },
	{ id: "oi", label: "موقعیت‌های باز", sortKey: "openInterest", cell: (q) => <NumberCell value={q.openInterest} /> },
	{ id: "iv", label: "نوسان ضمنی", sub: "IV", sortKey: "impliedVolatility", cell: (q) => <NumberCell value={q.impliedVolatility} kind="fraction" /> },
	{ id: "delta", label: "دلتا", sortKey: "delta", cell: (q) => <NumberCell value={q.delta} kind="decimal" digits={2} /> },
	{
		id: "bs", label: "قیمت بلک‌شولز", sub: "اختلاف با بازار٪", sortKey: "priceGapPercent",
		cell: (q) => (
			<div className="leading-tight">
				{q.theoreticalPrice === null ? dash : (
					<NumberCell value={q.theoreticalPrice} kind="decimal" digits={q.theoreticalPrice < 100 ? 1 : 0} />
				)}
				<div className="text-xs"><ChangeBadge value={q.priceGapPercent} digits={1} /></div>
			</div>
		),
	},
	{
		id: "be", label: "نقطه سر‌به‌سر", sub: "فاصله از قیمت پایه٪", sortKey: "breakEvenPercent",
		cell: (q) => (
			<div className="leading-tight">
				<NumberCell value={q.breakEven} />
				<div className="text-xs"><ChangeBadge value={q.breakEvenPercent} digits={1} /></div>
			</div>
		),
	},
	{ id: "lev", label: "اهرم", sortKey: "leverage", cell: (q) => <NumberCell value={q.leverage} kind="decimal" digits={1} /> },
	{ id: "status", label: "وضعیت", sub: "ITM / OTM", cell: (q) => <MoneynessBadge value={q.moneyness} /> },
];

interface Props {
	rows: OptionQuote[];
	sort: MarketSortKey;
	order: SortDir;
	onSort: (key: MarketSortKey) => void;
	/** Dim the rows while a new page or sort is loading. */
	loading?: boolean;
}

export default function MarketWatchTable({ rows, sort, order, onSort, loading }: Props) {
	return (
		<div className="max-h-[68vh] overflow-auto">
			<table className={cn("w-full min-w-[1500px] border-separate border-spacing-0 text-sm", loading && "opacity-60 transition-opacity")}>
				<thead>
					<tr>
						{COLUMNS.map((c) => {
							const active = !!c.sortKey && c.sortKey === sort;
							return (
								<th
									key={c.id}
									scope="col"
									aria-sort={active ? (order === "asc" ? "ascending" : "descending") : "none"}
									className={cn(
										"sticky top-0 border-b border-neutral-200 bg-neutral-50 px-3 py-2 text-start align-bottom text-xs font-semibold whitespace-nowrap text-neutral-600",
										c.sticky ? "start-0 z-30 border-e" : "z-20",
									)}
								>
									{c.sortKey ? (
										<button
											type="button"
											onClick={() => onSort(c.sortKey!)}
											className={cn(
												"inline-flex cursor-pointer items-center gap-1 text-start hover:text-sky-700",
												active && "text-sky-700",
											)}
										>
											<span className="leading-tight">
												{c.label}
												{c.sub && <span className="block font-normal text-neutral-400">{c.sub}</span>}
											</span>
											{active ? (
												order === "asc" ? <ChevronUp className="size-3.5" aria-hidden /> : <ChevronDown className="size-3.5" aria-hidden />
											) : (
												<ChevronsUpDown className="size-3.5 text-neutral-300" aria-hidden />
											)}
										</button>
									) : (
										<span className="leading-tight">
											{c.label}
											{c.sub && <span className="block font-normal text-neutral-400">{c.sub}</span>}
										</span>
									)}
								</th>
							);
						})}
					</tr>
				</thead>
				<tbody>
					{rows.map((q) => (
						<tr key={q.contractId} className="group">
							{COLUMNS.map((c) => (
								<td
									key={c.id}
									className={cn(
										"border-b border-neutral-100 bg-white px-3 py-2 whitespace-nowrap group-hover:bg-sky-50/60",
										c.sticky && "sticky start-0 z-10 border-e border-e-neutral-200",
									)}
								>
									{c.cell(q)}
								</td>
							))}
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
}
