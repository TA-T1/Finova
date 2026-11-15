import { useCallback, useEffect, useMemo } from "react";
import { Link, useSearchParams } from "react-router-dom";
import ChangeBadge from "@/components/Custom/ChangeBadge";
import { EmptyState, ErrorState, StaleBanner, TableSkeleton } from "@/components/Custom/DataStates";
import NumberCell from "@/components/Custom/NumberCell";
import RefreshIndicator from "@/components/Custom/RefreshIndicator";
import SearchableSelect from "@/components/Custom/SearchableSelect";
import { REFRESH_MS, useChain, useUnderlying, useUnderlyings } from "@/hooks/useMarketQueries";
import { getErrorMessage } from "@/services/marketService";
import { changePercent, daysLeft, formatInt, formatJalaliLong, ymd } from "@/utils/format";
import ChainTable from "./ChainTable";

/** The soonest expiry that has not passed; if all have passed, the first. */
function nearestExpiry(dates: string[]): string | undefined {
	return dates.find((d) => daysLeft(d) >= 0) ?? dates[0];
}

export default function ChainPage() {
	// The selection lives in the URL (?symbol=...&expiry=YYYY-MM-DD), so the page can be shared and reloaded.
	const [params, setParams] = useSearchParams();
	const symbolParam = params.get("symbol") ?? undefined;
	const expiryParam = params.get("expiry") ?? undefined;

	/** Changes some query params and keeps all the others. undefined removes one. */
	const setQuery = useCallback(
		(patch: Record<string, string | undefined>, replace = false) =>
			setParams(
				(prev) => {
					const next = new URLSearchParams(prev);
					for (const [k, v] of Object.entries(patch)) {
						if (v === undefined) next.delete(k);
						else next.set(k, v);
					}
					return next;
				},
				{ replace },
			),
		[setParams],
	);

	const underlyings = useUnderlyings();

	// No symbol in the URL: use the first underlying.
	const symbol = symbolParam ?? underlyings.data?.items[0]?.symbol;
	useEffect(() => {
		if (!symbolParam && symbol) setQuery({ symbol }, true);
	}, [symbolParam, symbol, setQuery]);

	const detail = useUnderlying(symbol);
	const expiryDates = detail.data?.expiryDates;

	// No expiry, or one this underlying does not have: use the nearest.
	const expiry = expiryParam && expiryDates?.includes(expiryParam) ? expiryParam : undefined;
	useEffect(() => {
		if (!symbol || !expiryDates) return;
		if (!expiryParam || !expiryDates.includes(expiryParam)) {
			const next = nearestExpiry(expiryDates);
			if (next) setQuery({ expiry: next }, true);
		}
	}, [symbol, expiryParam, expiryDates, setQuery]);

	const chain = useChain(symbol, expiry);

	const options = useMemo(
		() => (underlyings.data?.items ?? []).map((u) => ({ value: u.symbol, label: u.symbol, hint: u.name ?? undefined })),
		[underlyings.data],
	);

	const refetchAll = () => {
		underlyings.refetch();
		detail.refetch();
		chain.refetch();
	};

	const u = detail.data;
	const price = u?.lastPrice ?? null;
	const change = u ? changePercent(u.lastPrice, u.prevClose) : null;
	const days = expiry ? daysLeft(expiry) : null;
	const items = chain.data?.items;

	let body;
	const fatal = (!underlyings.data && underlyings.isError && underlyings) || (!u && detail.isError && detail) || (!items && chain.isError && chain);
	if (fatal) {
		body = <ErrorState message={getErrorMessage(fatal.error)} onRetry={refetchAll} retrying={fatal.isFetching} />;
	} else if (!symbol && underlyings.data) {
		body = <EmptyState title="دارایی پایه‌ای برای نمایش وجود ندارد" hint="هنوز داده‌ای ثبت نشده است." />;
	} else if (!items) {
		body = <TableSkeleton rows={9} cols={9} />;
	} else if (items.length === 0) {
		body = <EmptyState title="قراردادی برای این سررسید پیدا نشد" hint="دارایی پایه یا سررسید دیگری را انتخاب کنید." />;
	} else {
		const err = chain.isError ? chain : detail.isError ? detail : null;
		body = (
			<>
				{err && <StaleBanner message={getErrorMessage(err.error)} onRetry={refetchAll} />}
				<ChainTable items={items} underlyingPrice={price} loading={chain.isPlaceholderData} />
			</>
		);
	}

	return (
		<div className="space-y-4">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-xl font-bold text-neutral-900">زنجیره اختیار معامله</h1>
					<p className="mt-1 text-sm text-neutral-500">
						اختیار خرید و فروش یک دارایی پایه در یک سررسید، در دو طرف قیمت اعمال. <b className="font-medium text-neutral-700">قیمت‌ها به ریال</b> هستند.
					</p>
				</div>
				<RefreshIndicator
					updatedAt={Math.max(detail.dataUpdatedAt, chain.dataUpdatedAt)}
					isFetching={detail.isFetching || chain.isFetching}
					onRefresh={refetchAll}
					intervalMs={REFRESH_MS}
				/>
			</header>

			<section aria-label="انتخاب دارایی و سررسید" className="flex flex-wrap items-center gap-x-6 gap-y-3 rounded-lg border border-neutral-200 bg-white p-3">
				<div className="w-52">
					<span className="mb-1 block text-xs text-neutral-500">دارایی پایه</span>
					<SearchableSelect
						ariaLabel="دارایی پایه"
						allLabel="انتخاب دارایی"
						options={options}
						value={symbol}
						onChange={(v) => v && setQuery({ symbol: v, expiry: undefined })}
					/>
				</div>

				<label className="block">
					<span className="mb-1 block text-xs text-neutral-500">سررسید</span>
					<select
						name="expiry"
						value={expiry ?? ""}
						disabled={!expiryDates}
						onChange={(e) => setQuery({ expiry: e.target.value })}
						className="w-56 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm disabled:opacity-50"
					>
						{!expiry && <option value="">…</option>}
						{(expiryDates ?? []).map((d) => (
							<option key={d} value={d}>
								{formatJalaliLong(d)} ({formatInt(daysLeft(d))} روز)
							</option>
						))}
					</select>
				</label>

				{u && (
					<div className="flex flex-wrap items-center gap-x-6 gap-y-2 border-s border-neutral-200 ps-6">
						<div>
							<Link to={`/symbol/${encodeURIComponent(u.symbol)}`} className="text-sm font-semibold text-sky-700 hover:underline">
								{u.symbol}
							</Link>
							{u.name && <span className="ms-2 text-xs text-neutral-400">{u.name}</span>}
							<div className="flex items-baseline gap-2">
								<NumberCell value={price} className="text-xl font-bold text-neutral-900" />
								<ChangeBadge value={change} />
							</div>
						</div>
						{days !== null && (
							<div className="rounded-md bg-neutral-100 px-3 py-1.5 text-sm text-neutral-700">
								<span className="tabular-nums font-semibold">{formatInt(days)}</span> روز تا سررسید
								{expiry && <span className="ms-2 text-xs text-neutral-400 tabular-nums">{ymd(expiry)}</span>}
							</div>
						)}
					</div>
				)}
			</section>

			<section aria-label="جدول زنجیره" className="relative overflow-hidden rounded-lg border border-neutral-200 bg-white">
				{(detail.isFetching || chain.isFetching) && items && (
					<div className="absolute inset-x-0 top-0 z-40 h-0.5 animate-pulse bg-sky-500" aria-hidden />
				)}
				{body}
			</section>

			<p className="text-xs text-neutral-500">
				<span className="inline-block size-3 rounded-sm bg-sky-100 align-middle" /> اختیار خرید در سود (قیمت اعمال کمتر از قیمت دارایی پایه)
				<span className="mx-3" />
				<span className="inline-block size-3 rounded-sm bg-amber-100 align-middle" /> اختیار فروش در سود (قیمت اعمال بیشتر از قیمت دارایی پایه)
				<span className="mx-3" />
				<span className="inline-block size-3 rounded-sm bg-yellow-200 align-middle" /> نزدیک‌ترین قیمت اعمال به قیمت دارایی پایه
			</p>
		</div>
	);
}
