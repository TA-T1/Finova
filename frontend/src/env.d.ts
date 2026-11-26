/// <reference types="vite/client" />

interface ImportMetaEnv {
	/** "false" turns the mock layer off and calls the real backend. Default: on. */
	readonly VITE_USE_MOCK?: string;
	/** Backend origin, no trailing slash. Default: http://localhost:8080 */
	readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
	readonly env: ImportMetaEnv;
}
