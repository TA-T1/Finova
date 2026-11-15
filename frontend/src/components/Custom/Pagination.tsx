import { ChevronLeft, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatInt } from "@/utils/format";

interface Props {
	page: number; // 1-based
	pageSize: number;
	total: number;
	onPage: (page: number) => void;
	onPageSize?: (size: number) => void;
	pageSizes?: number[];
}

/** Page numbers to show: first, last, and the current one with its neighbours. */
function pageList(page: number, pages: number): (number | "…")[] {
	const want = new Set([1, pages, page - 1, page, page + 1].filter((p) => p >= 1 && p <= pages));
	const sorted = [...want].sort((a, b) => a - b);
	const out: (number | "…")[] = [];
	sorted.forEach((p, i) => {
		if (i > 0 && p - sorted[i - 1] > 1) out.push("…");
		out.push(p);
	});
	return out;
}

const btn =
	"inline-flex h-8 min-w-8 cursor-pointer items-center justify-center rounded-md border border-neutral-300 bg-white px-2 text-sm text-neutral-700 hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-40";

/** In RTL "previous" points right and "next" points left. */
export default function Pagination({ page, pageSize, total, onPage, onPageSize, pageSizes = [25, 50, 100] }: Props) {
	const pages = Math.max(1, Math.ceil(total / pageSize));
	const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
	const to = Math.min(page * pageSize, total);

	return (
		<div className="flex flex-wrap items-center justify-between gap-3 border-t border-neutral-200 px-3 py-2.5 text-sm text-neutral-600">
			<span className="tabular-nums">
				{formatInt(from)} تا {formatInt(to)} از {formatInt(total)} قرارداد
			</span>

			<nav aria-label="صفحه‌بندی" className="flex items-center gap-1">
				<button type="button" className={btn} disabled={page <= 1} onClick={() => onPage(page - 1)} aria-label="صفحه قبل">
					<ChevronRight className="size-4" aria-hidden />
				</button>
				{pageList(page, pages).map((p, i) =>
					p === "…" ? (
						<span key={`gap${i}`} className="px-1 text-neutral-400">…</span>
					) : (
						<button
							key={p}
							type="button"
							onClick={() => onPage(p)}
							aria-current={p === page ? "page" : undefined}
							className={cn(btn, "tabular-nums", p === page && "border-sky-600 bg-sky-600 text-white hover:bg-sky-600")}
						>
							{formatInt(p)}
						</button>
					),
				)}
				<button type="button" className={btn} disabled={page >= pages} onClick={() => onPage(page + 1)} aria-label="صفحه بعد">
					<ChevronLeft className="size-4" aria-hidden />
				</button>
			</nav>

			{onPageSize && (
				<label className="flex items-center gap-2">
					تعداد در صفحه
					<select
						name="pageSize"
						value={pageSize}
						onChange={(e) => onPageSize(Number(e.target.value))}
						className="rounded-md border border-neutral-300 bg-white px-2 py-1 text-sm"
					>
						{pageSizes.map((s) => (
							<option key={s} value={s}>{formatInt(s)}</option>
						))}
					</select>
				</label>
			)}
		</div>
	);
}
