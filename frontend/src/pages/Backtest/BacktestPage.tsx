import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import LineChart from "@/components/Custom/charts/LineChart";
import { EmptyState, ErrorState } from "@/components/Custom/DataStates";
import NumberCell from "@/components/Custom/NumberCell";
import SearchableSelect from "@/components/Custom/SearchableSelect";
import { useBacktest, useBacktests, useStrategyTemplates, useUnderlyings } from "@/hooks/useMarketQueries";
import { cn } from "@/lib/utils";
import { BACKTEST_MOCK, getErrorMessage } from "@/services/marketService";
import type { BacktestDetail, BacktestTrade, ExitReason } from "@/types/backtestTypes";
import { formatDecimal, formatFraction, formatInt, formatJalali } from "@/utils/format";

const EXIT_FA: Record<ExitReason, string> = { expiry: "سررسید", target: "هدف سود", stop: "حد ضرر" };
const SOURCE_FA = { snapshot: "قیمت بازار", model: "مدل", mixed: "ترکیبی" } as const;
const dayMs = (iso: string) => Date.parse(`${iso.slice(0, 10)}T00:00:00Z`);

function Metric({ label, hint, children, tone }: { label: string; hint?: string; children: React.ReactNode; tone?: "good" | "bad" }) {
	return (
		<div className="rounded-lg border border-neutral-200 bg-white p-3">
			<div className="text-xs text-neutral-500">{label}</div>
			<div className={cn("mt-1 text-xl font-bold tabular-nums", tone === "good" && "text-emerald-700", tone === "bad" && "text-rose-600")}>{children}</div>
			{hint && <div className="mt-0.5 text-xs text-neutral-400">{hint}</div>}
		</div>
	);
}

const sign = (v: number | null): "good" | "bad" | undefined => (v === null || v === 0 ? undefined : v > 0 ? "good" : "bad");

function Legs({ t }: { t: BacktestTrade }) {
	return (
		<span className="text-xs text-neutral-600" dir="ltr">
			{t.legs.map((l, i) => (
				<span key={i} className="me-2 whitespace-nowrap">
					<span className={l.side === "buy" ? "text-emerald-700" : "text-rose-600"}>{l.side === "buy" ? "+" : "−"}</span>
					{l.type === "underlying" ? "stock" : `${l.type === "call" ? "C" : "P"}${formatInt(l.strike)}`}
				</span>
			))}
		</span>
	);
}

function Result({ run }: { run: BacktestDetail }) {
	const m = run.metrics;
	const curve = useMemo(() => run.equityCurve.map((p) => ({ x: dayMs(p.date), y: p.equity })), [run]);
	const modelShare = m.modelPricedShare ?? 0;

	return (
		<div className="space-y-4">
			{modelShare > 0 && (
				<div role="note" className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
					<b>{formatFraction(modelShare, 0)} از معامله‌ها با قیمت مدل محاسبه شده‌اند.</b> برای سررسیدهای گذشته تاریخچهٔ قیمت اختیار در دسترس نیست؛
					قیمت‌ها از مدل بلک‌شولز با نوسان تاریخی ۲۰ روزه به دست آمده‌اند (بدون لبخند نوسان و فاصلهٔ خرید و فروش). نتایج رفتار استراتژی را روی قیمت‌های مدل نشان می‌دهند، نه سود واقعی بازار.
					کارمزدها تقریبی هستند.
				</div>
			)}

			<section aria-label="شاخص‌ها" className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7">
				<Metric label="تعداد معامله" hint={m.skipped ? `${formatInt(m.skipped)} سررسید رد شد` : undefined}>{formatInt(m.trades)}</Metric>
				<Metric label="نرخ برد">{formatFraction(m.winRate, 0)}</Metric>
				<Metric label="میانگین سود هر معامله" tone={sign(m.avgPnl)} hint="ریال، پس از کارمزد">{formatInt(m.avgPnl)}</Metric>
				<Metric label="بازده کل" tone={sign(m.totalReturn)}>{formatFraction(m.totalReturn, 1)}</Metric>
				<Metric label="حداکثر افت سرمایه" tone={m.maxDrawdown > 0 ? "bad" : undefined}>{formatFraction(m.maxDrawdown, 1)}</Metric>
				<Metric label="ضریب سود" hint="سود کل / زیان کل">{m.profitFactor === null ? "—" : formatDecimal(m.profitFactor, 2)}</Metric>
				<Metric label="جمع کارمزد" hint="ریال">{formatInt(m.totalFees)}</Metric>
			</section>

			<section aria-label="منحنی سرمایه" className="rounded-lg border border-neutral-200 bg-white p-4">
				<div className="mb-1 flex flex-wrap items-center justify-between gap-2">
					<h2 className="font-semibold text-neutral-900">منحنی سرمایه</h2>
					<span className="text-xs text-neutral-500">شروع از ۱٫۰ · هر معامله {formatFraction(Number(run.params.allocation ?? 0.05), 0)} سرمایه را در معرض ریسک می‌گذارد · بدون سود مرکب</span>
				</div>
				{curve.length < 2 ? (
					<EmptyState title="معامله‌ای برای رسم منحنی نیست" />
				) : (
					<LineChart
						points={curve}
						step
						formatX={(x) => formatJalali(new Date(x).toISOString())}
						formatY={(y) => formatDecimal(y, 2)}
						vlines={[]}
						ariaLabel="منحنی سرمایه"
					/>
				)}
			</section>

			<section aria-label="نتیجه به تفکیک سررسید" className="overflow-hidden rounded-lg border border-neutral-200 bg-white">
				<h2 className="border-b border-neutral-200 p-3 font-semibold text-neutral-900">نتیجه به تفکیک سررسید</h2>
				<div className="max-h-[60vh] overflow-auto">
					<table className="w-full min-w-[980px] border-separate border-spacing-0 text-sm">
						<thead>
							<tr className="text-xs text-neutral-600">
								{["سررسید", "ورود", "خروج", "پاها", "قیمت پایه (ورود → خروج)", "هزینهٔ ورود", "سود/زیان", "بازده نسبت به ریسک", "قیمت‌گذاری"].map((h) => (
									<th key={h} className="sticky top-0 z-10 border-b border-neutral-200 bg-neutral-50 px-3 py-2 text-start font-semibold whitespace-nowrap">{h}</th>
								))}
							</tr>
						</thead>
						<tbody>
							{[...run.trades].reverse().map((t) => (
								<tr key={t.expiry} className="hover:bg-sky-50/60">
									<td className="border-b border-neutral-100 px-3 py-2 tabular-nums whitespace-nowrap">{formatJalali(t.expiry)}</td>
									<td className="border-b border-neutral-100 px-3 py-2 tabular-nums whitespace-nowrap">{formatJalali(t.entryDate)}</td>
									<td className="border-b border-neutral-100 px-3 py-2 whitespace-nowrap">
										<span className="tabular-nums">{formatJalali(t.exitDate)}</span>
										{t.exitReason !== "expiry" && <span className="ms-1 text-xs text-neutral-500">({EXIT_FA[t.exitReason]})</span>}
									</td>
									<td className="border-b border-neutral-100 px-3 py-2"><Legs t={t} /></td>
									<td className="border-b border-neutral-100 px-3 py-2 whitespace-nowrap">
										<NumberCell value={t.spotEntry} /> <span className="text-neutral-300">→</span> <NumberCell value={t.spotExit} />
									</td>
									<td className="border-b border-neutral-100 px-3 py-2"><NumberCell value={t.netPremium} /></td>
									<td className="border-b border-neutral-100 px-3 py-2"><NumberCell value={t.pnl} className={t.pnl > 0 ? "font-medium text-emerald-700" : t.pnl < 0 ? "font-medium text-rose-600" : ""} /></td>
									<td className="border-b border-neutral-100 px-3 py-2"><NumberCell value={t.returnOnRisk * 100} kind="percent" digits={0} className={t.returnOnRisk >= 0 ? "text-emerald-700" : "text-rose-600"} /></td>
									<td className="border-b border-neutral-100 px-3 py-2 text-xs whitespace-nowrap text-neutral-500">{SOURCE_FA[t.priceSource]}</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
				{run.skipped.length > 0 && (
					<details className="border-t border-neutral-200 p-3 text-sm text-neutral-600">
						<summary className="cursor-pointer">{formatInt(run.skipped.length)} سررسید معامله نشد</summary>
						<ul className="mt-2 space-y-0.5 text-xs">
							{run.skipped.map((s) => (
								<li key={s.expiry}><span className="tabular-nums">{formatJalali(s.expiry)}</span> — {s.reason}</li>
							))}
						</ul>
					</details>
				)}
			</section>
		</div>
	);
}

export default function BacktestPage() {
	const [params, setParams] = useSearchParams();
	const underlyings = useUnderlyings(BACKTEST_MOCK);
	const templates = useStrategyTemplates();
	const underlying = params.get("underlying") ?? undefined;
	const template = params.get("template") ?? undefined;
	const runs = useBacktests({ underlying, template, limit: 20 });
	const [picked, setPicked] = useState<number>();

	const items = runs.data?.items;
	const runId = items?.find((r) => r.id === picked)?.id ?? items?.[0]?.id;
	const run = useBacktest(underlying && template ? runId : undefined);
	useEffect(() => setPicked(undefined), [underlying, template]);

	const set = (k: string, v: string | undefined) =>
		setParams((prev) => {
			const next = new URLSearchParams(prev);
			if (v === undefined) next.delete(k);
			else next.set(k, v);
			return next;
		}, { replace: true });

	const options = useMemo(
		() => (underlyings.data?.items ?? []).map((u) => ({ value: u.symbol, label: u.symbol, hint: u.name ?? undefined })),
		[underlyings.data],
	);
	const tpl = templates.data?.items.find((t) => t.id === template);

	let body;
	if (!underlying || !template) {
		body = <EmptyState title="یک دارایی پایه و یک الگو انتخاب کنید" hint="نتیجهٔ بک‌تست آن استراتژی روی سررسیدهای گذشته نمایش داده می‌شود." />;
	} else if (runs.isError && !items) {
		body = <ErrorState message={getErrorMessage(runs.error)} onRetry={() => runs.refetch()} retrying={runs.isFetching} />;
	} else if (!items) {
		body = <div className="h-48 animate-pulse rounded-lg bg-neutral-100" aria-busy="true" />;
	} else if (items.length === 0) {
		body = <EmptyState title="بک‌تستی برای این ترکیب ثبت نشده است" hint="دارایی پایه یا الگوی دیگری را امتحان کنید." />;
	} else if (run.isError && !run.data) {
		body = <ErrorState message={getErrorMessage(run.error)} onRetry={() => run.refetch()} retrying={run.isFetching} />;
	} else if (!run.data) {
		body = <div className="h-48 animate-pulse rounded-lg bg-neutral-100" aria-busy="true" />;
	} else {
		body = (
			<>
				{items.length > 1 && (
					<label className="block">
						<span className="mb-1 block text-xs text-neutral-500">اجرا</span>
						<select value={runId} onChange={(e) => setPicked(+e.target.value)} className="rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm">
							{items.map((r) => (
								<option key={r.id} value={r.id}>#{r.id} · {r.startDate} تا {r.endDate} · {r.tradeCount} معامله</option>
							))}
						</select>
					</label>
				)}
				<Result run={run.data} />
			</>
		);
	}

	return (
		<div className="space-y-4">
			<header>
				<h1 className="text-xl font-bold text-neutral-900">بک‌تست استراتژی</h1>
				<p className="mt-1 text-sm text-neutral-500">
					یک قاعده، مثلاً «۳۰ روز مانده به سررسید یک بول کال اسپرد با یک قیمت اعمال فاصله باز کن»، روی سررسیدهای گذشته اجرا و اندازه‌گیری شده است.
				</p>
			</header>

			<section aria-label="انتخاب" className="flex flex-wrap items-end gap-x-5 gap-y-3 rounded-lg border border-neutral-200 bg-white p-3">
				<div className="w-52">
					<span className="mb-1 block text-xs text-neutral-500">دارایی پایه</span>
					<SearchableSelect ariaLabel="دارایی پایه" allLabel="انتخاب دارایی" options={options} value={underlying} onChange={(v) => set("underlying", v)} />
				</div>
				<label className="block">
					<span className="mb-1 block text-xs text-neutral-500">الگوی استراتژی</span>
					<select value={template ?? ""} onChange={(e) => set("template", e.target.value || undefined)} className="w-64 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm">
						<option value="">انتخاب الگو…</option>
						{(templates.data?.items ?? []).map((t) => (
							<option key={t.id} value={t.id}>{t.nameFa}</option>
						))}
					</select>
				</label>
				{tpl && (
					<p className="pb-1.5 text-xs text-neutral-500">
						{tpl.legs.map((l) => `${l.side === "buy" ? "خرید" : "فروش"} ${l.type === "underlying" ? "سهم" : l.type === "call" ? "کال" : "پوت"}`).join(" + ")}
					</p>
				)}
			</section>

			{body}
		</div>
	);
}
