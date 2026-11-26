import { createBrowserRouter } from "react-router-dom";
import PublicLayout from "@/layouts/PublicLayout/PublicLayout";
import Landing from "@/pages/Landing";
import Temp from "@/pages/Temp";
import AppLayout from "@/layouts/AppLayout/AppLayout";
import MarketWatchPage from "@/pages/MarketWatch/MarketWatchPage";
import ChainPage from "@/pages/Chain/ChainPage";
import SymbolPage from "@/pages/Symbol/SymbolPage";
import StrategyPage from "@/pages/Strategy/StrategyPage";
import BacktestPage from "@/pages/Backtest/BacktestPage";

export const router = createBrowserRouter([
	{
		path: "/",
		element: <PublicLayout />,
		// errorElement: (
		// 	<Error404 />
		// ),
		children: [
			{
				index: true,
				element: <Landing />,
			},
			{
				path: "/temp",
				element: <Temp />,
			},
			// {
			// 	path: "/AboutUs",
			// 	element: <AboutUs />,
			// },
			// {
		],
	},
	{
		element: <AppLayout />,
		children: [
			{ path: "/market-watch", element: <MarketWatchPage /> },
			{ path: "/chain", element: <ChainPage /> },
			{ path: "/symbol/:symbol", element: <SymbolPage /> },
			{ path: "/strategy", element: <StrategyPage /> },
			{ path: "/backtests", element: <BacktestPage /> },
		],
	},
	// {
	// 	element: <PrivateLayout />,
	// 	children: [
	// 		{
	// 			path: "/EditProfile",
	// 			element: <EditProfile />,
	// 		},
	// 		{
	// 			path: "/DashBoard",
	// 			element: <DashBoard />,
	// 		},
	// 	],
	// },
	// {
	// 	element: <AnotherLayout />,
	// 	children: [
	// 		{
	// 			path: "/login",
	// 			element: <Login />,
	// 		},
	// 		{
	// 			path: "/temp",
	// 			element: <Temp />,
	// 		},
	// 	],
	// },
]);
