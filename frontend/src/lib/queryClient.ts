import { QueryClient } from "@tanstack/react-query";

// No automatic retries: a failed request shows the error state with a retry
// button, which is easier to see and to test than a silent retry loop.
export const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			retry: false,
			refetchOnWindowFocus: true,
		},
	},
});
