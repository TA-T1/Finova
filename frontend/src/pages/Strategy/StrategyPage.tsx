import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Plus, Trash2 } from "lucide-react";
import LineChart from "@/components/Custom/charts/LineChart";
import { EmptyState, ErrorState } from "@/components/Custom/DataStates";
import NumberCell from "@/components/Custom/NumberCell";
import SearchableSelect from "@/components/Custom/SearchableSelect";
import { useChain, useStrategyAnalysis, useStrategyTemplates, useSymbol, useUnderlyings } from "@/hooks/useMarketQueries";
import { cn } from "@/lib/utils";
import { buildStrategy, getErrorMessage } from "@/services/marketService";
import type { LegInput, LegType, Side, StrategyLeg } from "@/types/strategyTypes";
import { daysLeft, formatCompact, formatDecimal, formatInt, formatJalaliLong } from "@/utils/format";

/** One leg being edited. `meta` is only for display; the server gets `input`. */
interface Entry {
	key: number;
	input: LegInput;
	meta: { type: LegType; symbol: string; strikePrice: number | null };
}

const OUTLOOK_FA = { bullish: "صعودی", bearish: "نزولی", neutral: "خنثی", volatile: "نوسانی" } as const;
const TYPE_FA: Record<LegType, string> = { call: "اختیار خرید", put: "اختیار فروش", underlying: "سهم پایه" };
let nextKey = 1;

const fromLeg = (l: StrategyLeg): Entry => ({
	key: nextKey++,
	input: { contractId: l.contractId, underlyingId: l.underlyingId, side: l.side, quantity: l.quantity, price: l.price },
	meta: { type: l.type, symbol: l.symbol, strikePrice: l.strikePrice },
});

function Stat({ label, children, tone }: { label: string; children: React.ReactNode; tone?: "good" | "bad" }) {
	return (
		<div className="rounded-lg border border-neutral-200 bg-white p-3">
			<div className="text-xs text-neutral-500">{label}</div>
			<div className={cn("mt-1 text-lg font-bold tabular-nums", tone === "good" && "text-emerald-700", tone === "bad" && "text-rose-600")}>{children}</div>
		</div>
	);
}

const money = (v: number | null, unlimited: string) => (v === null ? unlimited : `${formatCompact(v)} ریال`);

export default function StrategyPage() {
	const [params, setParams] = useSearchParams();
	const underlyings = useUnderlyings();
	const templates = useStrategyTemplates();
	const symbol = params.get("symbol") ?? underlyings.data?.items[0]?.symbol;
	const detail = useSymbol(symbol);
	const expiries = detail.data?.expiries ?? [];
	const expiryParam = params.get("expiry");
	const expiry = expiryParam && expiries.some((e) => e.expiryDate === expiryParam) ? expiryParam : expiries[0]?.expiryDate;
	const chain = useChain(symbol, expiry);

	const [entries, setEntries] = useState<Entry[]>([]);
	const [templateId, setTemplateId] = useState<string>("");
	const [quantity, setQuantity] = useState(1);
	const [building, setBuilding] = useState(false);
	const [buildError, setBuildError] = useState<string | null>(null);

	// Another underlying or expiry means another set of contracts: start over.
	useEffect(() => setEntries([]), [symbol, expiry]);

	const legs = useMemo(() => entries.map((e) => e.input), [entries]);
	const analysis = useStrategyAnalysis(legs);
	const a = analysis.data?.analysis;
	const underlyingId = underlyings.data?.items.find((u) => u.symbol === symbol)?.id;

	const setQuery = (patch: Record<string, string | undefined>) =>
		setParams((prev) => {
			const next = new URLSearchParams(prev);
			for (const [k, v] of Object.entries(patch)) {
				if (v === undefined) next.delete(k);
				else next.set(k, v);
			}
			return next;
		}, { replace: true });

	const options = useMemo(
		() => (underlyings.data?.items ?? []).map((u) => ({ value: u.symbol, label: u.symbol, hint: u.name ?? undefined })),
		[underlyings.data],
	);

	const build = async () => {
		if (!symbol || !expiry || !templateId) return;
		setBuilding(true);
		setBuildError(null);
		try {
			const res = await buildStrategy({ templateId, underlying: symbol, expiry, quantity });
			setEntries(res.legs.map(fromLeg));
		} catch (e) {
			setBuildError(getErrorMessage(e));
		} finally {
			setBuilding(false);
		}
	};

	// --- adding a leg by hand ---
	const [add, setAdd] = useState<{ type: LegType; side: Side; strike?: number }>({ type: "call", side: "buy" });
	const strikes = chain.data?.strikes ?? [];
	const addLeg = () => {
		if (add.type === "underlying") {
			if (underlyingId === undefined) return;
			setEntries((p) => [...p, {
				key: nextKey++,
				input: { underlyingId, side: add.side, quantity: 1000 },
				meta: { type: "underlying", symbol: symbol ?? "", strikePrice: null },
			}]);
			return;
		}
		const row = strikes.find((s) => s.strikePrice === (add.strike ?? strikes[0]?.strikePrice));
		const side = row?.[add.type];
		if (!row || !side || side.contractId === undefined) return;
		setEntries((p) => [...p, {
			key: nextKey++,
			input: { contractId: side.contractId, side: add.side, quantity: 1 },
			meta: { type: add.type as LegType, symbol: side.symbol, strikePrice: row.strikePrice },
		}]);
	};

	const patch = (key: number, p: Partial<LegInput>) =>
		setEntries((all) => all.map((e) => (e.key === key ? { ...e, input: { ...e.input, ...p } } : e)));

	const spot = chain.data?.underlyingPrice ?? detail.data?.price ?? null;
	const points = a?.payoff.map((p) => ({ x: p.underlyingPrice, y: p.pnl })) ?? [];
	const priceOf = (e: Entry) => analysis.data?.legs.find((l) => l.symbol === e.meta.symbol && l.side === e.input.side)?.price;

	return (
		<div className="space-y-4">
			<header>
				<h1 className="text-xl font-bold text-neutral-900">ساخت استراتژی</h1>
				<p className="mt-1 text-sm text-neutral-500">
					پاها را خودتان اضافه کنید یا از یک الگو شروع کنید؛ سود و زیان در سررسید را ببینید. محاسبه با قیمت پای‌ها و بدون کارمزد و مالیات است. <b className="font-medium text-neutral-700">قیمت‌ها به ریال</b> هستند.
				</p>
			</header>

			<section aria-label="دارایی و سررسید" className="flex flex-wrap items-end gap-x-5 gap-y-3 rounded-lg border border-neutral-200 bg-white p-3">
				<div className="w-52">
					<span className="mb-1 block text-xs text-neutral-500">دارایی پایه</span>
					<SearchableSelect ariaLabel="دارایی پایه" allLabel="انتخاب دارایی" options={options} value={symbol} onChange={(v) => v && setQuery({ symbol: v, expiry: undefined })} />
				</div>
				<label className="block">
					<span className="mb-1 block text-xs text-neutral-500">سررسید</span>
					<select
						value={expiry ?? ""}
						disabled={expiries.length === 0}
						onChange={(e) => setQuery({ expiry: e.target.value })}
						className="w-56 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm disabled:opacity-50"
					>
						{expiries.map((e) => (
							<option key={e.expiryDate} value={e.expiryDate}>
								{formatJalaliLong(e.expiryDate)} ({formatInt(daysLeft(e.expiryDate))} روز)
							</option>
						))}
					</select>
				</label>
				{spot !== null && (
					<div className="pb-1 text-sm text-neutral-600">
						قیمت دارایی پایه: <NumberCell value={spot} className="font-semibold text-neutral-900" />
					</div>
				)}
			</section>

			<section aria-label="الگو" className="rounded-lg border border-neutral-200 bg-white p-3">
				<div className="flex flex-wrap items-end gap-x-4 gap-y-3">
					<label className="block">
						<span className="mb-1 block text-xs text-neutral-500">شروع از الگو</span>
						<select value={templateId} onChange={(e) => setTemplateId(e.target.value)} className="w-64 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm">
							<option value="">یک الگو انتخاب کنید…</option>
							{(templates.data?.items ?? []).map((t) => (
								<option key={t.id} value={t.id}>{t.nameFa} — {OUTLOOK_FA[t.outlook]}</option>
							))}
						</select>
					</label>
					<label className="block">
						<span className="mb-1 block text-xs text-neutral-500">تعداد قرارداد</span>
						<input type="number" min={1} value={quantity} onChange={(e) => setQuantity(Math.max(1, Math.floor(+e.target.value || 1)))} className="w-24 rounded-md border border-neutral-300 px-3 py-1.5 text-sm tabular-nums" />
					</label>
					<button type="button" onClick={build} disabled={!templateId || !expiry || building} className="cursor-pointer rounded-md bg-sky-600 px-4 py-1.5 text-sm text-white hover:bg-sky-700 disabled:cursor-not-allowed disabled:opacity-50">
						{building ? "در حال ساخت…" : "ساخت از الگو"}
					</button>
				</div>
				{templateId && (
					<p className="mt-2 text-xs text-neutral-500">
						{templates.data?.items.find((t) => t.id === templateId)?.legs.map((l) => `${l.side === "buy" ? "خرید" : "فروش"} ${TYPE_FA[l.type]}`).join(" + ")}
						{" · قیمت‌های اعمال از میان قیمت‌های اعمال موجود در سررسید انتخاب می‌شوند."}
					</p>
				)}
				{buildError && <p role="alert" className="mt-2 text-sm text-rose-600">{buildError}</p>}
			</section>

			<section aria-label="پاها" className="rounded-lg border border-neutral-200 bg-white">
				<div className="flex flex-wrap items-end justify-between gap-3 border-b border-neutral-200 p-3">
					<h2 className="font-semibold text-neutral-900">پاهای استراتژی</h2>
					<div className="flex flex-wrap items-end gap-2">
						<label className="block">
							<span className="mb-1 block text-xs text-neutral-500">نوع</span>
							<select value={add.type} onChange={(e) => setAdd({ ...add, type: e.target.value as LegType })} className="rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm">
								<option value="call">اختیار خرید</option>
								<option value="put">اختیار فروش</option>
								<option value="underlying">سهم پایه</option>
							</select>
						</label>
						{add.type !== "underlying" && (
							<label className="block">
								<span className="mb-1 block text-xs text-neutral-500">قیمت اعمال</span>
								<select value={add.strike ?? strikes[0]?.strikePrice ?? ""} onChange={(e) => setAdd({ ...add, strike: +e.target.value })} className="w-32 rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm tabular-nums">
									{strikes.filter((s) => s[add.type as "call" | "put"]).map((s) => (
										<option key={s.strikePrice} value={s.strikePrice}>{formatInt(s.strikePrice)}</option>
									))}
								</select>
							</label>
						)}
						<div role="group" aria-label="خرید یا فروش" className="inline-flex overflow-hidden rounded-md border border-neutral-300">
							{(["buy", "sell"] as const).map((s) => (
								<button key={s} type="button" aria-pressed={add.side === s} onClick={() => setAdd({ ...add, side: s })}
									className={cn("cursor-pointer px-3 py-1.5 text-sm not-first:border-s not-first:border-neutral-300", add.side === s ? (s === "buy" ? "bg-emerald-600 text-white" : "bg-rose-600 text-white") : "bg-white text-neutral-700")}>
									{s === "buy" ? "خرید" : "فروش"}
								</button>
							))}
						</div>
						<button type="button" onClick={addLeg} disabled={!chain.data} className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-sky-600 px-3 py-1.5 text-sm text-sky-700 hover:bg-sky-50 disabled:cursor-not-allowed disabled:opacity-50">
							<Plus className="size-4" aria-hidden /> افزودن پا
						</button>
					</div>
				</div>

				{entries.length === 0 ? (
					<EmptyState title="هنوز پایی اضافه نشده است" hint="از یک الگو شروع کنید یا با «افزودن پا» قراردادها را خودتان انتخاب کنید." />
				) : (
					<div className="overflow-x-auto">
						<table className="w-full min-w-[640px] text-sm">
							<thead className="bg-neutral-50 text-xs text-neutral-600">
								<tr>
									<th className="px-3 py-2 text-start font-semibold">خرید/فروش</th>
									<th className="px-3 py-2 text-start font-semibold">قرارداد</th>
									<th className="px-3 py-2 text-start font-semibold">قیمت اعمال</th>
									<th className="px-3 py-2 text-start font-semibold">تعداد<span className="block font-normal text-neutral-400">قرارداد / سهم</span></th>
									<th className="px-3 py-2 text-start font-semibold">قیمت (هر سهم)<span className="block font-normal text-neutral-400">خالی = قیمت بازار</span></th>
									<th className="w-10" />
								</tr>
							</thead>
							<tbody>
								{entries.map((e) => (
									<tr key={e.key} className="border-t border-neutral-100">
										<td className="px-3 py-2">
											<button type="button" onClick={() => patch(e.key, { side: e.input.side === "buy" ? "sell" : "buy" })} title="تغییر خرید و فروش"
												className={cn("cursor-pointer rounded px-2 py-0.5 text-xs font-medium", e.input.side === "buy" ? "bg-emerald-50 text-emerald-700" : "bg-rose-50 text-rose-700")}>
												{e.input.side === "buy" ? "خرید" : "فروش"}
											</button>
										</td>
										<td className="px-3 py-2"><span className="font-medium">{e.meta.symbol}</span><span className="ms-2 text-xs text-neutral-500">{TYPE_FA[e.meta.type]}</span></td>
										<td className="px-3 py-2"><NumberCell value={e.meta.strikePrice} /></td>
										<td className="px-3 py-2">
											<input type="number" min={1} aria-label="تعداد" value={e.input.quantity} onChange={(ev) => patch(e.key, { quantity: Math.max(1, Math.floor(+ev.target.value || 1)) })} className="w-24 rounded-md border border-neutral-300 px-2 py-1 tabular-nums" />
										</td>
										<td className="px-3 py-2">
											<input type="number" min={0} aria-label="قیمت" placeholder={priceOf(e) !== undefined ? String(Math.round(priceOf(e)!)) : ""} value={e.input.price ?? ""}
												onChange={(ev) => patch(e.key, { price: ev.target.value === "" ? undefined : Math.max(0, +ev.target.value) })} className="w-28 rounded-md border border-neutral-300 px-2 py-1 tabular-nums" />
										</td>
										<td className="px-2">
											<button type="button" aria-label="حذف پا" onClick={() => setEntries((all) => all.filter((x) => x.key !== e.key))} className="cursor-pointer rounded p-1.5 text-neutral-400 hover:bg-rose-50 hover:text-rose-600">
												<Trash2 className="size-4" aria-hidden />
											</button>
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				)}
			</section>

			{entries.length > 0 && (
				<section aria-label="نتیجه" className="space-y-3">
					{analysis.isError && !a ? (
						<ErrorState message={getErrorMessage(analysis.error)} onRetry={() => analysis.refetch()} retrying={analysis.isFetching} />
					) : !a ? (
						<div className="h-48 animate-pulse rounded-lg bg-neutral-100" aria-busy="true" />
					) : (
						<>
							{analysis.isError && <p role="alert" className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">{getErrorMessage(analysis.error)}</p>}
							<div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
								<Stat label={a.netPremium >= 0 ? "هزینهٔ ورود (بدهکار)" : "دریافتی ورود (بستانکار)"}>{money(Math.abs(a.netPremium), "")}</Stat>
								<Stat label="حداکثر سود" tone="good">{money(a.maxProfit, "نامحدود")}</Stat>
								<Stat label="حداکثر زیان" tone="bad">{money(a.maxLoss, "نامحدود")}</Stat>
								<Stat label="نقطهٔ سر‌به‌سر">
									{a.breakEvens.length === 0 ? <span className="text-neutral-300">—</span> : (
										<span className="flex flex-wrap gap-x-3">{a.breakEvens.map((b) => <NumberCell key={b} value={b} />)}</span>
									)}
								</Stat>
								<Stat label="دلتای موقعیت">{a.greeks ? formatDecimal(a.greeks.delta, 1) : <span className="text-neutral-300">—</span>}</Stat>
							</div>

							<div className={cn("rounded-lg border border-neutral-200 bg-white p-4", analysis.isFetching && "opacity-70 transition-opacity")}>
								<div className="mb-1 flex items-center justify-between">
									<h2 className="font-semibold text-neutral-900">سود و زیان در سررسید</h2>
									<span className="text-xs text-neutral-500">محور افقی: قیمت دارایی پایه · محور عمودی: سود/زیان (ریال)</span>
								</div>
								<LineChart
									points={points}
									signed
									formatX={(x) => formatInt(x)}
									ariaLabel="نمودار سود و زیان استراتژی در سررسید"
									vlines={[
										...(spot !== null ? [{ x: spot, label: "قیمت فعلی", color: "#e11d48" }] : []),
										...a.breakEvens.map((b) => ({ x: b, label: "سر‌به‌سر", color: "#64748b" })),
									]}
								/>
							</div>
						</>
					)}
				</section>
			)}
		</div>
	);
}
