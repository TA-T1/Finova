import { createRoot } from "react-dom/client";
import "@fontsource-variable/vazirmatn";
import "./index.css";
import { router } from "./routes/routes.tsx";
import { RouterProvider } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/queryClient";

createRoot(document.getElementById("root")!).render(
	<QueryClientProvider client={queryClient}>
		<RouterProvider router={router} />
	</QueryClientProvider>
);
