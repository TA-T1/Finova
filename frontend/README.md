# React + TypeScript + Zustand Project Setup & Usage Guide

This document explains how to set up and work with the project.

---

## 🚀 Getting Started

### Prerequisites

-   **Node.js** (recommended: latest LTS version)
-   **npm**

### Installation

Clone the repository and install dependencies:

```bash
npm install
```

### Running the Development Server

Start the development server with:

```bash
npm run dev
```

It runs on **[http://localhost:3000](http://localhost:3000)**.

### Build

```bash
npm run build
```

---

## 📂 Project Structure

```
src/
├── assets/               # Static files like images
├── components/           # Reusable components
│   ├── Custom/           # Custom components (buttons, inputs, toast, etc.)
│   └── ui/               # shadcn/ui components (pre-styled components)
├── layouts/              # Layout components
├── pages/                # Page components
├── routes/
│   └── route.tsx         # Central route configuration
├── services/             # API service layer
│   ├── services.ts       # Generic request templates (GET, POST, PUT, DELETE)
│   └── authService.ts    # Example: authentication services
├── store/                # Zustand stores
│   └── useProfileStore.ts # Example store for user profile state
├── types/                # TypeScript types & interfaces
│   └── authTypes.ts      # Example: auth-related types
├── utils/                # Utility functions
│   └── translateNumbers.ts # Example utility function
└── toast/                # Toast notifications setup
```

---

## 🗄️ State Management (Zustand)

Stores are located in `src/store/`. Example usage of a persisted profile store:

```ts
import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { UserState } from "@/types/userTypes";

const useProfileStore = create<UserState>()(
	persist(
		(set) => ({
			username: null,
			setUsername: (username: string) => set({ username }),
		}),
		{
			name: "profile-storage",
			storage: createJSONStorage(() => sessionStorage),
		}
	)
);

export default useProfileStore;
```

Example usage in a component:

```ts
import useProfileStore from "@/store/useProfileStore";

const username = useProfileStore((state) => state.username);
const setUsername = useProfileStore((state) => state.setUsername);
```

---

## 🛠️ Services

`services.ts` provides generic HTTP methods (GET, POST, PUT, DELETE, etc.). Specific services like `authService.ts` use these templates.

Example (`authService.ts`):

```ts
import type { LoginPayload, LoginResponse } from "@/types/authTypes";
import { postData } from "./services";

export const loginService = async (
	credentials: LoginPayload
): Promise<LoginResponse> => {
	return postData({
		endPoint: `/v1/auth/login`,
		data: credentials,
	});
};
```

---

## 📏 Conventions & Notes

-   **TypeScript types** should be placed in `src/types/`.
-   **Reusable utilities** belong in `src/utils/`.
-   **Assets** (images, icons, etc.) go inside `src/assets/`.
-   **Shadcn UI components** are in `src/components/ui/`.
-   **Toast notifications** are set up in the `toast/` folder.
-   Zustand stores should be persisted when needed and properly named to match file names.

---

✅ You’re ready to start coding!

---

## 📈 Market watch and option chain (mock data)

Two pages, in Persian and right-to-left:

| Route | Page |
|---|---|
| `/market-watch` | table of all option contracts: sorting, filters, pagination, 30-second refresh |
| `/chain?symbol=خودرو&expiry=2026-11-27` | calls and puts of one underlying and expiry, around the strike |
| `/symbol/:symbol` | placeholder |

There is **no backend for these pages yet**, so they run on mock data by default.

### Mock mode

Copy `.env.example` to `.env` to change the settings.

| Variable | Default | Meaning |
|---|---|---|
| `VITE_USE_MOCK` | `true` | `false` calls the real backend (**not tested against a server yet**) |
| `VITE_API_BASE_URL` | `http://localhost:8080` | backend origin, used when mock mode is off |

In mock mode, add a switch to the page URL to see the other states:

| URL switch | Shows |
|---|---|
| `?mockError=1` | every request fails: the error state with the retry button |
| `?mockEmpty=1` | every list is empty: the empty state |

The mock market (4 underlyings, 144 contracts) moves a little on every refresh. Sorting,
filtering and paging are done in `src/mocks/`, the same way the server is expected to do them.

### Where things are

```
src/types/marketTypes.ts       every API shape, in one place (update this when the API is final)
src/services/marketService.ts  typed API functions; the ONLY place that decides mock or real
src/mocks/                     mock market + mock API (safe to keep for demos)
src/hooks/useMarketQueries.ts  data hooks: 30 s refresh, pause while the tab is hidden
src/components/Custom/         shared components: NumberCell, ChangeBadge, OptionTypeBadge,
                               MoneynessBadge, RefreshIndicator, SearchableSelect, Pagination,
                               DataStates (skeleton, error, empty)
src/pages/MarketWatch, Chain   the two pages
```

What the backend has to provide for the real API: `docs/frontend-api-requests.md`.

