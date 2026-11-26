// Display helpers for the market pages. Everything uses the browser's Intl with
// the fa-IR locale, so digits are Persian and separators are the Persian ones
// ("٬" thousands, "٫" decimal). No extra library is needed.

export const EMPTY = "—";

const intFmt = new Intl.NumberFormat("fa-IR", { maximumFractionDigits: 0 });

const decimalFmts = new Map<number, Intl.NumberFormat>();
function decimalFmt(digits: number, signed = false): Intl.NumberFormat {
	const key = digits * 2 + (signed ? 1 : 0);
	let f = decimalFmts.get(key);
	if (!f) {
		f = new Intl.NumberFormat("fa-IR", {
			minimumFractionDigits: digits,
			maximumFractionDigits: digits,
			signDisplay: signed ? "exceptZero" : "auto",
		});
		decimalFmts.set(key, f);
	}
	return f;
}

const isNum = (n: number | null | undefined): n is number =>
	typeof n === "number" && Number.isFinite(n);

/** 1234567 -> ۱٬۲۳۴٬۵۶۷ ; null -> — */
export function formatInt(n: number | null | undefined): string {
	return isNum(n) ? intFmt.format(Math.round(n)) : EMPTY;
}

/** 0.4567, 2 -> ۰٫۴۶ ; null -> — */
export function formatDecimal(
	n: number | null | undefined,
	digits = 2,
): string {
	return isNum(n) ? decimalFmt(digits).format(n) : EMPTY;
}

/** A percentage that is already x100: 12.345, 1 -> ۱۲٫۳٪ . `signed` adds + or −. */
export function formatPercent(
	n: number | null | undefined,
	digits = 1,
	signed = false,
): string {
	return isNum(n) ? `${decimalFmt(digits, signed).format(n)}٪` : EMPTY;
}

/** A decimal fraction as a percentage: 0.3512 -> ۳۵٫۱٪ */
export function formatFraction(
	n: number | null | undefined,
	digits = 1,
): string {
	return isNum(n) ? formatPercent(n * 100, digits) : EMPTY;
}

/** Large values in a short form: 1.2e9 -> ۱٫۲ میلیارد */
export function formatCompact(n: number | null | undefined): string {
	if (!isNum(n)) return EMPTY;
	const abs = Math.abs(n);
	const units: [number, string][] = [
		[1e12, "هزار میلیارد"], // 10^12 (values are in rial)
		[1e9, "میلیارد"],
		[1e6, "میلیون"],
	];
	for (const [size, label] of units) {
		if (abs >= size) return `${decimalFmt(abs >= size * 100 ? 0 : 1).format(n / size)} ${label}`;
	}
	return intFmt.format(Math.round(n));
}

/** Percent change between two prices; null when it cannot be computed. */
export function changePercent(
	last: number | null | undefined,
	prev: number | null | undefined,
): number | null {
	if (!isNum(last) || !isNum(prev) || prev <= 0) return null;
	return ((last - prev) / prev) * 100;
}

const TEHRAN = "Asia/Tehran";

const timeFmt = new Intl.DateTimeFormat("fa-IR", {
	hour: "2-digit",
	minute: "2-digit",
	second: "2-digit",
	hour12: false,
	timeZone: TEHRAN,
});

/** Clock time in Tehran: ۱۲:۳۰:۰۱ */
export function formatTime(ms: number): string {
	return timeFmt.format(new Date(ms));
}

const jalaliShort = new Intl.DateTimeFormat("fa-IR-u-ca-persian", {
	year: "numeric",
	month: "2-digit",
	day: "2-digit",
	timeZone: "UTC",
});
const jalaliLong = new Intl.DateTimeFormat("fa-IR-u-ca-persian", {
	year: "numeric",
	month: "long",
	day: "numeric",
	timeZone: "UTC",
});

/** The YYYY-MM-DD part of an ISO date or date-time string. */
export const ymd = (iso: string): string => iso.slice(0, 10);

function utcDate(iso: string): Date {
	return new Date(`${ymd(iso)}T00:00:00Z`);
}

/** "2026-12-20T00:00:00Z" -> ۱۴۰۵/۰۹/۳۰ (Jalali) */
export function formatJalali(iso: string): string {
	return jalaliShort.format(utcDate(iso));
}

/** "2026-12-20T00:00:00Z" -> ۳۰ آذر ۱۴۰۵ (Jalali, month name) */
export function formatJalaliLong(iso: string): string {
	return jalaliLong.format(utcDate(iso));
}

/** Today's date in Tehran as YYYY-MM-DD. */
export function todayTehran(now = Date.now()): string {
	// "en-CA" formats as YYYY-MM-DD.
	return new Intl.DateTimeFormat("en-CA", { timeZone: TEHRAN }).format(new Date(now));
}

/** Whole days from today (Tehran) to an expiry date; negative once expired. */
export function daysLeft(expiryIso: string, now = Date.now()): number {
	const ms = utcDate(expiryIso).getTime() - utcDate(todayTehran(now)).getTime();
	return Math.round(ms / 86_400_000);
}
