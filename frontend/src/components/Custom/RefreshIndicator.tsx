import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatInt, formatTime } from "@/utils/format";

interface Props {
	/** Time of the last successful fetch (ms), e.g. `dataUpdatedAt` of a query. 0 = none yet. */
	updatedAt: number;
	isFetching: boolean;
	onRefresh: () => void;
	/** Auto-refresh period in ms, used for the countdown. */
	intervalMs: number;
	className?: string;
}

/** "Last updated" time, a countdown to the next auto-refresh and a manual refresh button. */
export default function RefreshIndicator({ updatedAt, isFetching, onRefresh, intervalMs, className }: Props) {
	const [now, setNow] = useState(() => Date.now());
	const [hidden, setHidden] = useState(() => document.hidden);

	useEffect(() => {
		const tick = setInterval(() => setNow(Date.now()), 1000);
		const onVis = () => {
			setHidden(document.hidden);
			setNow(Date.now());
		};
		document.addEventListener("visibilitychange", onVis);
		return () => {
			clearInterval(tick);
			document.removeEventListener("visibilitychange", onVis);
		};
	}, []);

	const remaining = updatedAt ? Math.max(0, Math.ceil((updatedAt + intervalMs - now) / 1000)) : null;

	return (
		<div className={cn("flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-600", className)}>
			<span>
				آخرین بروزرسانی:{" "}
				<span className="font-medium text-neutral-800 tabular-nums">
					{updatedAt ? formatTime(updatedAt) : "—"}
				</span>
			</span>
			<span className="text-xs text-neutral-400" aria-live="off">
				{hidden
					? "بروزرسانی خودکار متوقف است (تب پنهان)"
					: remaining !== null && !isFetching
						? `بروزرسانی بعدی تا ${formatInt(remaining)} ثانیه`
						: ""}
			</span>
			<button
				type="button"
				onClick={onRefresh}
				disabled={isFetching}
				className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-neutral-300 bg-white px-2.5 py-1 text-sm text-neutral-700 hover:bg-neutral-50 disabled:cursor-wait disabled:opacity-60"
			>
				<RefreshCw className={cn("size-3.5", isFetching && "animate-spin")} aria-hidden />
				بروزرسانی
			</button>
		</div>
	);
}
