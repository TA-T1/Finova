import { NavLink, Outlet } from "react-router-dom";
import { cn } from "@/lib/utils";
import { USE_MOCK } from "@/services/marketService";

const NAV = [
	{ to: "/market-watch", label: "دیده‌بان بازار" },
	{ to: "/chain", label: "زنجیره اختیار" },
];

/** Shell for the market pages: a top bar with the menu, then the page. */
export default function AppLayout() {
	return (
		<div className="min-h-screen bg-neutral-50 text-neutral-900 [color-scheme:light]">
			<header className="sticky top-0 z-50 border-b border-neutral-200 bg-white">
				<div className="mx-auto flex max-w-[1700px] items-center gap-6 px-4 py-2.5">
					<NavLink to="/" className="text-lg font-extrabold tracking-tight text-sky-700">
						فینوا
					</NavLink>
					<nav aria-label="منوی اصلی" className="flex gap-1">
						{NAV.map((n) => (
							<NavLink
								key={n.to}
								to={n.to}
								className={({ isActive }) =>
									cn(
										"rounded-md px-3 py-1.5 text-sm hover:bg-neutral-100",
										isActive ? "bg-sky-50 font-semibold text-sky-700" : "text-neutral-600",
									)
								}
							>
								{n.label}
							</NavLink>
						))}
					</nav>
					{USE_MOCK && (
						<span
							title="داده‌ها نمونه هستند و از سرور نمی‌آیند (VITE_USE_MOCK)"
							className="ms-auto rounded-full bg-violet-50 px-2.5 py-0.5 text-xs font-medium text-violet-700 ring-1 ring-violet-200 ring-inset"
						>
							داده نمونه
						</span>
					)}
				</div>
			</header>
			<main className="mx-auto max-w-[1700px] px-4 py-5">
				<Outlet />
			</main>
		</div>
	);
}
