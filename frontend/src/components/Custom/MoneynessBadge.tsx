import { cn } from "@/lib/utils";
import type { Moneyness } from "@/types/marketTypes";

const STYLE: Record<Moneyness, { label: string; title: string; cls: string }> = {
	ITM: { label: "ITM", title: "در سود (In The Money)", cls: "bg-emerald-50 text-emerald-700 ring-emerald-200" },
	ATM: { label: "ATM", title: "نزدیک قیمت (At The Money)", cls: "bg-neutral-100 text-neutral-700 ring-neutral-300" },
	OTM: { label: "OTM", title: "خارج از سود (Out Of The Money)", cls: "bg-neutral-50 text-neutral-500 ring-neutral-200" },
};

export default function MoneynessBadge({ value, className }: { value: Moneyness | null; className?: string }) {
	if (!value) return <span className="text-neutral-300">—</span>;
	const s = STYLE[value];
	return (
		<span
			title={s.title}
			className={cn("inline-block rounded px-1.5 py-0.5 text-[11px] font-semibold ring-1 ring-inset", s.cls, className)}
		>
			{s.label}
		</span>
	);
}
