import { Link, useParams } from "react-router-dom";

/** Placeholder: the content of the symbol page is a later task. The route exists so links work. */
export default function SymbolPage() {
	const { symbol } = useParams<{ symbol: string }>();
	return (
		<div className="mx-auto max-w-xl rounded-lg border border-dashed border-neutral-300 bg-white p-10 text-center">
			<h1 className="text-xl font-bold text-neutral-900">{symbol}</h1>
			<p className="mt-2 text-neutral-500">صفحهٔ این نماد به‌زودی اضافه می‌شود.</p>
			<div className="mt-5 flex justify-center gap-3 text-sm">
				<Link to="/market-watch" className="text-sky-700 hover:underline">دیده‌بان بازار</Link>
				<Link to={`/chain?symbol=${encodeURIComponent(symbol ?? "")}`} className="text-sky-700 hover:underline">زنجیره اختیار</Link>
			</div>
		</div>
	);
}
