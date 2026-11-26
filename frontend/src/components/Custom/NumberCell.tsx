import { cn } from "@/lib/utils";
import {
	formatCompact,
	formatDecimal,
	formatFraction,
	formatInt,
	formatPercent,
} from "@/utils/format";

type Kind = "int" | "decimal" | "compact" | "percent" | "fraction";

interface Props {
	value: number | null | undefined;
	/** int: 1,234 | decimal: 0.45 | compact: 1.2 میلیارد | percent: value is already x100 | fraction: 0.35 -> 35% */
	kind?: Kind;
	digits?: number;
	className?: string;
}

/** A number in Persian digits with separators; "—" when there is no value. */
export default function NumberCell({ value, kind = "int", digits, className }: Props) {
	let text: string;
	switch (kind) {
		case "decimal":
			text = formatDecimal(value, digits ?? 2);
			break;
		case "compact":
			text = formatCompact(value);
			break;
		case "percent":
			text = formatPercent(value, digits ?? 1);
			break;
		case "fraction":
			text = formatFraction(value, digits ?? 1);
			break;
		default:
			text = formatInt(value);
	}
	const empty = value === null || value === undefined;
	return (
		<span className={cn("tabular-nums", empty && "text-neutral-300", className)}>{text}</span>
	);
}
