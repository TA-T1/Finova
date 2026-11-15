import { createBrowserRouter } from "react-router-dom";
import PublicLayout from "@/layouts/PublicLayout/PublicLayout";
import Landing from "@/pages/Landing";
import Temp from "@/pages/Temp";
import AppLayout from "@/layouts/AppLayout/AppLayout";
import MarketWatchPage from "@/pages/MarketWatch/MarketWatchPage";
import ChainPage from "@/pages/Chain/ChainPage";
import SymbolPage from "@/pages/Symbol/SymbolPage";

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
