/// <reference types="vite/client" />

interface ImportMetaEnv {
	/** "false" turns the mock layer off and calls the real backend. Default: on. */
	readonly VITE_USE_MOCK?: string;
	/** "true": the strategy page calls the real API (needs the /strategies endpoints). Default: mock. */
	readonly VITE_STRATEGY_API?: string;
	/** "true": the backtest page calls the real API (needs /backtests and saved runs). Default: mock. */
	readonly VITE_BACKTEST_API?: string;
	/** Backend origin, no trailing slash. Default: http://localhost:8080 */
	readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
	readonly env: ImportMetaEnv;
}
