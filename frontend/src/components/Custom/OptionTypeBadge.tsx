import { cn } from "@/lib/utils";
import type { OptionType } from "@/types/marketTypes";

const STYLE: Record<OptionType, { label: string; title: string; cls: string }> = {
	call: { label: "اختیار خرید", title: "Call", cls: "bg-sky-50 text-sky-700 ring-sky-200" },
	put: { label: "اختیار فروش", title: "Put", cls: "bg-amber-50 text-amber-700 ring-amber-200" },
};

/** Call / put pill. Blue and amber on purpose, so it never looks like up/down. */
export default function OptionTypeBadge({ type, className }: { type: OptionType; className?: string }) {
	const s = STYLE[type];
	return (
		<span
			title={s.title}
			className={cn(
				"inline-block rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset",
				s.cls,
				className,
			)}
		>
			{s.label}
		</span>
	);
}
