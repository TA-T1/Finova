import { cn } from "@/lib/utils";
import { formatPercent } from "@/utils/format";

interface Props {
	/** Percent change, already x100 (1.5 means +1.5%). null -> "—". */
	value: number | null | undefined;
	digits?: number;
	className?: string;
}

/** A signed percentage: green with ▲ when positive, red with ▼ when negative. */
export default function ChangeBadge({ value, digits = 2, className }: Props) {
	if (value === null || value === undefined || !Number.isFinite(value)) {
		return <span className={cn("text-neutral-300", className)}>—</span>;
	}
	const up = value > 0;
	const down = value < 0;
	return (
		<span
			className={cn(
				"inline-flex items-center gap-0.5 tabular-nums font-medium",
				up && "text-emerald-600",
				down && "text-rose-600",
				!up && !down && "text-neutral-500",
				className,
			)}
		>
			{up && <span aria-hidden className="text-[0.65em]">▲</span>}
			{down && <span aria-hidden className="text-[0.65em]">▼</span>}
			{formatPercent(Math.abs(value), digits)}
		</span>
	);
}
