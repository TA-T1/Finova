import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Search } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SelectOption {
	value: string;
	label: string;
	/** Extra text that is searchable and shown in grey, e.g. the full name. */
	hint?: string;
}

interface Props {
	options: SelectOption[];
	value: string | undefined;
	onChange: (value: string | undefined) => void;
	/** Shown on the button when nothing is selected, and as the first ("all") item. */
	allLabel: string;
	ariaLabel: string;
	className?: string;
}

// Arabic ي/ك typed on an Arabic keyboard should still find Persian ی/ک.
const norm = (s: string) => s.replace(/ي/g, "ی").replace(/ك/g, "ک").toLowerCase();

/** A select with a search box (no UI library: the project only has sonner installed). */
export default function SearchableSelect({ options, value, onChange, allLabel, ariaLabel, className }: Props) {
	const [open, setOpen] = useState(false);
	const [query, setQuery] = useState("");
	const [active, setActive] = useState(0);
	const root = useRef<HTMLDivElement>(null);
	const input = useRef<HTMLInputElement>(null);
	const listId = useId();

	// "All" is item 0 when the search box is empty.
	const items = useMemo(() => {
		const q = norm(query.trim());
		const found = q
			? options.filter((o) => norm(`${o.label} ${o.hint ?? ""} ${o.value}`).includes(q))
			: options;
		return q ? found : [{ value: "", label: allLabel }, ...found];
	}, [options, query, allLabel]);

	useEffect(() => {
		if (!open) return;
		const onDown = (e: MouseEvent) => {
			if (!root.current?.contains(e.target as Node)) setOpen(false);
		};
		document.addEventListener("mousedown", onDown);
		input.current?.focus();
		return () => document.removeEventListener("mousedown", onDown);
	}, [open]);

	const choose = (v: string) => {
		onChange(v === "" ? undefined : v);
		setOpen(false);
		setQuery("");
	};

	const onKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "ArrowDown") {
			e.preventDefault();
			setActive((a) => Math.min(a + 1, items.length - 1));
		} else if (e.key === "ArrowUp") {
			e.preventDefault();
			setActive((a) => Math.max(a - 1, 0));
		} else if (e.key === "Enter") {
			e.preventDefault();
			if (items[active]) choose(items[active].value);
		} else if (e.key === "Escape") {
			setOpen(false);
		}
	};

	const selected = options.find((o) => o.value === value);

	return (
		<div ref={root} className={cn("relative", className)}>
			<button
				type="button"
				role="combobox"
				aria-label={ariaLabel}
				aria-expanded={open}
				aria-controls={listId}
				onClick={() => {
					setOpen((o) => !o);
					setActive(0);
				}}
				className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm text-neutral-800 hover:border-neutral-400"
			>
				<span className={cn("truncate", !selected && "text-neutral-500")}>{selected?.label ?? allLabel}</span>
				<ChevronDown className="size-4 shrink-0 text-neutral-400" aria-hidden />
			</button>

			{open && (
				<div className="absolute start-0 top-full z-30 mt-1 w-64 max-w-[85vw] rounded-md border border-neutral-200 bg-white shadow-lg">
					<div className="flex items-center gap-2 border-b border-neutral-100 px-2.5 py-2">
						<Search className="size-4 text-neutral-400" aria-hidden />
						<input
							ref={input}
							name="select-search"
							value={query}
							onChange={(e) => {
								setQuery(e.target.value);
								setActive(0);
							}}
							onKeyDown={onKeyDown}
							placeholder="جستجو…"
							aria-label="جستجو"
							className="w-full bg-transparent text-sm outline-none placeholder:text-neutral-400"
						/>
					</div>
					<ul id={listId} role="listbox" aria-label={ariaLabel} className="max-h-60 overflow-auto py-1">
						{items.length === 0 && <li className="px-3 py-2 text-sm text-neutral-400">موردی پیدا نشد</li>}
						{items.map((o, i) => {
							const isSel = (o.value || undefined) === value;
							return (
								<li
									key={o.value || "__all"}
									role="option"
									aria-selected={isSel}
									onMouseEnter={() => setActive(i)}
									onClick={() => choose(o.value)}
									className={cn(
										"flex cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-sm",
										i === active && "bg-sky-50",
									)}
								>
									<span className="truncate">
										{o.label}
										{o.hint && <span className="ms-2 text-xs text-neutral-400">{o.hint}</span>}
									</span>
									{isSel && <Check className="size-4 shrink-0 text-sky-600" aria-hidden />}
								</li>
							);
						})}
					</ul>
				</div>
			)}
		</div>
	);
}
