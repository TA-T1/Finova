import { AlertTriangle, Inbox } from "lucide-react";

/** Pulsing placeholder rows, shown while the first load is running. */
export function TableSkeleton({ rows = 8, cols = 8 }: { rows?: number; cols?: number }) {
	return (
		<div role="status" aria-label="در حال بارگذاری" className="animate-pulse space-y-2 p-3">
			<span className="sr-only">در حال بارگذاری…</span>
			{Array.from({ length: rows }, (_, r) => (
				<div key={r} className="flex gap-3">
					{Array.from({ length: cols }, (_, c) => (
						<div
							key={c}
							className="h-6 flex-1 rounded bg-neutral-200"
							style={{ opacity: 1 - r * 0.07 }}
						/>
					))}
				</div>
			))}
		</div>
	);
}

/** Full error state with a retry button, for when there is nothing to show. */
export function ErrorState({
	message,
	onRetry,
	retrying,
}: {
	message: string;
	onRetry: () => void;
	retrying?: boolean;
}) {
	return (
		<div role="alert" className="flex flex-col items-center gap-3 px-4 py-14 text-center">
			<AlertTriangle className="size-9 text-rose-500" aria-hidden />
			<p className="font-medium text-neutral-800">دریافت اطلاعات انجام نشد</p>
			<p className="max-w-md text-sm text-neutral-500">{message}</p>
			<button
				type="button"
				onClick={onRetry}
				disabled={retrying}
				className="cursor-pointer rounded-md bg-sky-600 px-4 py-1.5 text-sm text-white hover:bg-sky-700 disabled:cursor-wait disabled:opacity-60"
			>
				{retrying ? "در حال تلاش…" : "تلاش دوباره"}
			</button>
		</div>
	);
}

/** Nothing matches the current filters. */
export function EmptyState({ title, hint }: { title: string; hint?: string }) {
	return (
		<div className="flex flex-col items-center gap-2 px-4 py-14 text-center">
			<Inbox className="size-9 text-neutral-300" aria-hidden />
			<p className="font-medium text-neutral-700">{title}</p>
			{hint && <p className="max-w-md text-sm text-neutral-500">{hint}</p>}
		</div>
	);
}

/** Small banner for "the refresh failed, but this is the previous data". */
export function StaleBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
	return (
		<div role="alert" className="flex items-center justify-between gap-3 border-b border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
			<span>بروزرسانی ناموفق بود؛ اطلاعات قبلی نمایش داده می‌شود. {message}</span>
			<button type="button" onClick={onRetry} className="cursor-pointer whitespace-nowrap rounded border border-amber-300 bg-white px-2 py-0.5 hover:bg-amber-100">
				تلاش دوباره
			</button>
		</div>
	);
}
