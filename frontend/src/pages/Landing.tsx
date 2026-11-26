import { useNavigate } from "react-router-dom";

function Landing() {
	const Navigate = useNavigate();

	return (
		<div className="w-full h-screen font-3xl flex flex-col place-self-center justify-center">
			<div className="flex gap-2 rounded-md place-self-center">
				<button
					className="text-white bg-sky-600 rounded-md p-2 cursor-pointer"
					onClick={() => {
						Navigate("/temp");
					}}
				>
					Temp route
				</button>
				<button
					className="text-white bg-sky-600 rounded-md p-2 cursor-pointer"
					onClick={() => Navigate("/market-watch")}
				>
					دیده‌بان بازار
				</button>
				<button
					className="text-white bg-sky-600 rounded-md p-2 cursor-pointer"
					onClick={() => Navigate("/chain")}
				>
					زنجیره اختیار
				</button>
			</div>
			<p className="text-5xl text-center ">This is your landing</p>
		</div>
	);
}

export default Landing;
