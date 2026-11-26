import { Link, useParams } from "react-router-dom";
import ChangeBadge from "@/components/Custom/ChangeBadge";
import OpenInterestChart from "@/components/Custom/charts/OpenInterestChart";
import { EmptyState, ErrorState } from "@/components/Custom/DataStates";
import NumberCell from "@/components/Custom/NumberCell";
import RefreshIndicator from "@/components/Custom/RefreshIndicator";
import { REFRESH_MS, useSymbol } from "@/hooks/useMarketQueries";
import { getErrorMessage } from "@/services/marketService";
import type { SymbolDetail } from "@/types/marketTypes";
import { daysLeft, formatCompact, formatDecimal, formatInt, formatJalaliLong } from "@/utils/format";

const KIND_FA: Record<SymbolDetail["kind"], string> = { stock: "سهام", fund: "صندوق", index: "شاخص", other: "سایر" };

function Card({ title, children, hint }: { title: string; children: React.ReactNode; hint?: string }) {
	return (
		<div className="rounded-lg border border-neutral-200 bg-white p-4">
			<div className="text-xs text-neutral-500">{title}</div>
			<div className="mt-1">{children}</div>
			{hint && <div className="mt-1 text-xs text-neutral-400">{hint}</div>}
		</div>
	);
}

/** Put/call ratio reading: above 1 more puts than calls are open (a more defensive market). */
function pcrHint(pcr: number | null): string {
	if (pcr === null) return "موقعیت باز اختیار خرید صفر است";
	if (pcr > 1) return "موقعیت باز اختیار فروش بیشتر از خرید است";
	if (pcr < 1) return "موقعیت باز اختیار خرید بیشتر از فروش است";
	return "برابر";
}

export default function SymbolPage() {
	const { symbol } = useParams<{ symbol: string }>();
	const q = useSymbol(symbol);
	const d = q.data;

	if (!d && q.isError) {
		const err = q.error as { response?: { status?: number } } | null;
		if (err?.response?.status === 404) {
			return <EmptyState title={`برای «${symbol}» قرارداد اختیار فعالی پیدا نشد`} hint="نماد را بررسی کنید یا از دیده‌بان بازار انتخاب کنید." />;
		}
		return <ErrorState message={getErrorMessage(q.error)} onRetry={() => q.refetch()} retrying={q.isFetching} />;
	}
	if (!d) {
		return <div className="h-64 animate-pulse rounded-lg bg-neutral-100" aria-busy="true" aria-label="در حال بارگذاری" />;
	}

	const oi = d.openInterest;
	const total = oi.calls + oi.puts;
	const nearest = d.expiries[0];

	return (
		<div className="space-y-4">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<div className="flex items-baseline gap-3">
						<h1 className="text-2xl font-bold text-neutral-900">{d.symbol}</h1>
						{d.name && <span className="text-neutral-500">{d.name}</span>}
						<span className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">{KIND_FA[d.kind]}</span>
					</div>
					<div className="mt-1 flex items-baseline gap-3">
						<NumberCell value={d.price} className="text-3xl font-bold text-neutral-900" />
						<span className="text-sm text-neutral-500">ریال</span>
						<ChangeBadge value={d.priceChangePercent} />
						{d.priceChange !== null && (
							<span className="text-sm text-neutral-500 tabular-nums" dir="ltr">
								{d.priceChange > 0 ? "+" : ""}
								{formatInt(d.priceChange)}
							</span>
						)}
					</div>
				</div>
				<div className="flex flex-wrap items-center gap-3">
					<Link to={`/chain?symbol=${encodeURIComponent(d.symbol)}`} className="rounded-md bg-sky-600 px-3.5 py-1.5 text-sm text-white hover:bg-sky-700">
						زنجیره اختیار
					</Link>
					<Link to={`/strategy?symbol=${encodeURIComponent(d.symbol)}`} className="rounded-md border border-neutral-300 bg-white px-3.5 py-1.5 text-sm text-neutral-700 hover:bg-neutral-50">
						ساخت استراتژی
					</Link>
					<RefreshIndicator updatedAt={q.dataUpdatedAt} isFetching={q.isFetching} onRefresh={() => q.refetch()} intervalMs={REFRESH_MS} />
				</div>
			</header>

			<section aria-label="شاخص‌ها" className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
				<Card title="نسبت پوت به کال (PCR)" hint={pcrHint(oi.pcr)}>
					<NumberCell value={oi.pcr} kind="decimal" digits={2} className="text-2xl font-bold" />
				</Card>
				<Card title="موقعیت باز اختیار خرید / فروش" hint={total > 0 ? `${formatDecimal((oi.calls / total) * 100, 0)}٪ خرید · ${formatDecimal((oi.puts / total) * 100, 0)}٪ فروش` : undefined}>
					<div className="flex items-baseline gap-2 text-lg font-semibold tabular-nums">
						<span className="text-sky-700">{formatInt(oi.calls)}</span>
						<span className="text-neutral-300">/</span>
						<span className="text-amber-700">{formatInt(oi.puts)}</span>
					</div>
				</Card>
				<Card title="Max Pain" hint={d.maxPain ? `سررسید ${formatJalaliLong(d.maxPain.expiryDate)} · کمترین پرداخت ${formatCompact(d.maxPain.totalPayout)} ریال` : undefined}>
					{d.maxPain ? <NumberCell value={d.maxPain.strikePrice} className="text-2xl font-bold text-violet-700" /> : <span className="text-neutral-300">—</span>}
				</Card>
				<Card title="نزدیک‌ترین سررسید" hint={nearest ? `${formatInt(nearest.contractCount)} قرارداد` : undefined}>
					{nearest ? (
						<div className="text-lg font-semibold">
							{formatJalaliLong(nearest.expiryDate)}
							<div className="text-sm font-normal text-neutral-500">{formatInt(daysLeft(nearest.expiryDate))} روز مانده</div>
						</div>
					) : (
						<span className="text-neutral-300">—</span>
					)}
				</Card>
			</section>

			<section aria-label="موقعیت‌های باز" className="rounded-lg border border-neutral-200 bg-white p-4">
				<div className="mb-2 flex flex-wrap items-center justify-between gap-2">
					<h2 className="font-semibold text-neutral-900">موقعیت‌های باز به تفکیک قیمت اعمال</h2>
					<div className="flex items-center gap-4 text-xs text-neutral-600">
						<span><span className="me-1 inline-block size-2.5 rounded-sm bg-sky-500 align-middle" />اختیار خرید</span>
						<span><span className="me-1 inline-block size-2.5 rounded-sm bg-amber-500 align-middle" />اختیار فروش</span>
						{d.maxPain && <span><span className="me-1 inline-block size-2.5 rounded-sm bg-violet-200 align-middle" />Max Pain</span>}
					</div>
				</div>
				{d.strikeOpenInterest.length === 0 ? (
					<EmptyState title="موقعیت بازی ثبت نشده است" />
				) : (
					<OpenInterestChart data={d.strikeOpenInterest} price={d.price} maxPain={d.maxPain?.strikePrice} />
				)}
				<p className="mt-1 text-xs text-neutral-400">مجموع همهٔ سررسیدهای فعال. Max Pain برای نزدیک‌ترین سررسید محاسبه شده است.</p>
			</section>

			<section aria-label="سررسیدها" className="rounded-lg border border-neutral-200 bg-white p-4">
				<h2 className="mb-3 font-semibold text-neutral-900">سررسیدها</h2>
				{d.expiries.length === 0 ? (
					<EmptyState title="سررسید فعالی وجود ندارد" />
				) : (
					<ul className="flex flex-wrap gap-2">
						{d.expiries.map((e) => (
							<li key={e.expiryDate}>
								<Link
									to={`/chain?symbol=${encodeURIComponent(d.symbol)}&expiry=${e.expiryDate}`}
									className="block rounded-md border border-neutral-200 px-3 py-2 text-sm hover:border-sky-300 hover:bg-sky-50"
								>
									<div className="font-medium text-neutral-900">{formatJalaliLong(e.expiryDate)}</div>
									<div className="text-xs text-neutral-500">
										{formatInt(daysLeft(e.expiryDate))} روز · {formatInt(e.contractCount)} قرارداد
									</div>
								</Link>
							</li>
						))}
					</ul>
				)}
			</section>
		</div>
	);
}
