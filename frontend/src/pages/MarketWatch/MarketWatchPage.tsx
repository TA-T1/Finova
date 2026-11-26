import { useMemo, useState } from "react";
import { EmptyState, ErrorState, StaleBanner, TableSkeleton } from "@/components/Custom/DataStates";
import Pagination from "@/components/Custom/Pagination";
import RefreshIndicator from "@/components/Custom/RefreshIndicator";
import SearchableSelect from "@/components/Custom/SearchableSelect";
import { REFRESH_MS, useExpiries, useMarketStatus, useMarketWatch, useUnderlyings } from "@/hooks/useMarketQueries";
import { cn } from "@/lib/utils";
import { getErrorMessage } from "@/services/marketService";
import type { MarketSortKey, MarketWatchParams, OptionType, SortDir } from "@/types/marketTypes";
import { formatInt, formatJalaliLong, formatTime } from "@/utils/format";
import MarketWatchTable from "./MarketWatchTable";

/** Numbers are most useful biggest-first, so their first click sorts descending. */
const TEXT_KEYS: MarketSortKey[] = ["symbol", "underlyingSymbol", "optionType", "expiryDate", "moneyness"];

interface State {
	underlying?: string;
	type?: OptionType;
	expiry?: string;
	sort: MarketSortKey;
	order: SortDir;
	page: number;
	pageSize: number;
}

const INITIAL: State = { sort: "volume", order: "desc", page: 1, pageSize: 25 };

const TYPE_TABS: { value: OptionType | undefined; label: string }[] = [
	{ value: undefined, label: "همه" },
	{ value: "call", label: "اختیار خرید" },
	{ value: "put", label: "اختیار فروش" },
];

export default function MarketWatchPage() {
	const [s, setS] = useState<State>(INITIAL);
	/** Any change of filter, sort or page size goes back to page 1. */
	const update = (patch: Partial<State>) => setS((prev) => ({ ...prev, page: 1, ...patch }));

	const params: MarketWatchParams = {
		underlying: s.underlying,
		type: s.type,
		expiry: s.expiry,
		sort: s.sort,
		order: s.order,
		page: s.page,
		pageSize: s.pageSize,
	};

	const watch = useMarketWatch(params);
	const underlyings = useUnderlyings();
	const expiries = useExpiries(s.underlying);
	const status = useMarketStatus();

	const underlyingOptions = useMemo(
		() => (underlyings.data?.items ?? []).map((u) => ({ value: u.symbol, label: u.symbol, hint: u.name ?? undefined })),
		[underlyings.data],
	);

	const onSort = (key: MarketSortKey) => {
		if (key === s.sort) update({ order: s.order === "asc" ? "desc" : "asc" });
		else update({ sort: key, order: TEXT_KEYS.includes(key) ? "asc" : "desc" });
	};

	const filtered = !!(s.underlying || s.type || s.expiry);
	const data = watch.data;

	let body;
	if (!data && watch.isError) {
		body = <ErrorState message={getErrorMessage(watch.error)} onRetry={() => watch.refetch()} retrying={watch.isFetching} />;
	} else if (!data) {
		body = <TableSkeleton rows={10} cols={9} />;
	} else if (data.total === 0) {
		body = (
			<EmptyState
				title="قراردادی با این فیلترها پیدا نشد"
				hint={filtered ? "فیلترها را تغییر دهید یا پاک کنید." : "فعلاً قراردادی برای نمایش وجود ندارد."}
			/>
		);
	} else {
		body = (
			<>
				{watch.isError && <StaleBanner message={getErrorMessage(watch.error)} onRetry={() => watch.refetch()} />}
				<MarketWatchTable rows={data.items} sort={s.sort} order={s.order} onSort={onSort} loading={watch.isPlaceholderData} />
				<Pagination
					page={data.page}
					pageSize={data.pageSize}
					total={data.total}
					onPage={(page) => setS((prev) => ({ ...prev, page }))}
					onPageSize={(pageSize) => update({ pageSize })}
				/>
			</>
		);
	}

	return (
		<div className="space-y-4">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-xl font-bold text-neutral-900">دیده‌بان بازار اختیار معامله</h1>
					<p className="mt-1 text-sm text-neutral-500">
						همه قراردادها با قیمت، حجم و شاخص‌های محاسبه‌شده. <b className="font-medium text-neutral-700">قیمت‌ها به ریال</b> هستند.
					</p>
				</div>
				<div className="flex flex-wrap items-center gap-3">
					{status.data && (
						<span
							className={cn(
								"rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ring-inset",
								status.data.marketOpen
									? "bg-emerald-50 text-emerald-700 ring-emerald-200"
									: "bg-neutral-100 text-neutral-600 ring-neutral-200",
							)}
							title={status.data.lastUpdate ? `آخرین قیمت ثبت‌شده: ${formatTime(Date.parse(status.data.lastUpdate))}` : undefined}
						>
							{status.data.marketOpen ? "بازار باز است" : "بازار بسته است"}
							{status.data.lastUpdate && !status.data.marketOpen && (
								<span className="ms-1 tabular-nums">· آخرین به‌روزرسانی {formatTime(Date.parse(status.data.lastUpdate))}</span>
							)}
						</span>
					)}
				<RefreshIndicator
					updatedAt={watch.dataUpdatedAt}
					isFetching={watch.isFetching}
					onRefresh={() => watch.refetch()}
					intervalMs={REFRESH_MS}
				/>
				</div>
			</header>

			<section aria-label="فیلترها" className="flex flex-wrap items-end gap-x-5 gap-y-3 rounded-lg border border-neutral-200 bg-white p-3">
				<div className="w-52">
					<span className="mb-1 block text-xs text-neutral-500">دارایی پایه</span>
					<SearchableSelect
						ariaLabel="دارایی پایه"
						allLabel="همه دارایی‌ها"
						options={underlyingOptions}
						value={s.underlying}
						onChange={(underlying) => update({ underlying })}
					/>
				</div>

				<div>
					<span className="mb-1 block text-xs text-neutral-500">نوع قرارداد</span>
					<div role="group" aria-label="نوع قرارداد" className="inline-flex overflow-hidden rounded-md border border-neutral-300">
						{TYPE_TABS.map((t) => (
							<button
								key={t.label}
								type="button"
								aria-pressed={s.type === t.value}
								onClick={() => update({ type: t.value })}
								className={cn(
									"cursor-pointer px-3.5 py-1.5 text-sm not-first:border-s not-first:border-neutral-300",
									s.type === t.value ? "bg-sky-600 text-white" : "bg-white text-neutral-700 hover:bg-neutral-50",
								)}
							>
								{t.label}
							</button>
						))}
					</div>
				</div>

				<label className="block">
					<span className="mb-1 block text-xs text-neutral-500">سررسید</span>
					<select
						name="expiry"
						value={s.expiry ?? ""}
						onChange={(e) => update({ expiry: e.target.value || undefined })}
						className="w-56 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm"
					>
						<option value="">همه سررسیدها</option>
						{(expiries.data?.items ?? []).map((e) => (
							<option key={e.expiryDate} value={e.expiryDate}>
								{formatJalaliLong(e.expiryDate)} ({formatInt(e.daysToExpiry)} روز)
							</option>
						))}
					</select>
				</label>

				{filtered && (
					<button
						type="button"
						onClick={() => update({ underlying: undefined, type: undefined, expiry: undefined })}
						className="cursor-pointer pb-1.5 text-sm text-sky-700 hover:underline"
					>
						پاک کردن فیلترها
					</button>
				)}
			</section>

			<section aria-label="جدول قراردادها" className="relative overflow-hidden rounded-lg border border-neutral-200 bg-white">
				{watch.isFetching && data && (
					<div className="absolute inset-x-0 top-0 z-40 h-0.5 animate-pulse bg-sky-500" aria-hidden />
				)}
				{body}
			</section>
		</div>
	);
}
