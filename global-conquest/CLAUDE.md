# Global Conquest — Frontend CLAUDE.md

> AI assistant context for the React/TypeScript SPA at `global-conquest/src/`.

---

## 1. Frontend Purpose

This client is a render layer only. It sends player input to the server and displays what the server returns.

- REST (`/api/*`) — auth, lobby metadata, initial game bootstrap
- WebSocket (`/ws?token=...`) — all real-time events: game state, chat, typing presence
- Session token is the same credential for both: Bearer header on REST, query param on WebSocket

All game rules, dice rolls, phase transitions, and victory conditions run in `backend/internal/risk/`. The client never computes outcomes.

---

## 2. Tech Stack

| Concern | Tool | Version |
|---|---|---|
| Language | TypeScript | ~5.9.3 (strict) |
| Framework | React | 19.2.0 |
| Routing | TanStack Router | 1.163.x |
| HTTP client | Axios | 1.13.x |
| Styling | Tailwind CSS | 4.2.x |
| Build | Vite | 7.2.x |
| Game rendering | SVG + HTML Canvas | no Phaser |
| State management | React Context + `useState` | no Zustand/Redux |
| Testing | — | no test framework |

---

## 3. Directory Responsibilities

```
src/
├── api/          Axios modules, one file per domain: auth, games, users, chat, health
├── auth/         AuthContext, AuthProvider, useAuth, localStorage token persistence
├── realtime/     WebSocket hook (useGameSocket), SocketProvider, WsEnvelope types
├── router/       TanStack Router tree + all page components
│   └── pages/    One file per page. GamePage.tsx and LobbyPage.tsx are the core ones.
│                 styles.ts — shared Tailwind class strings
│                 gameShared.ts — map constants (territory positions, edges, colors)
└── assets/       risk0.png — static board background image
```

---

## 4. Routing and Auth Guards

File: `src/router/index.tsx`

Route tree:
```
/ → redirect to /login or /app/lobby
/login, /signup → AuthPages (redirect away if already authenticated)
/app (protected — beforeLoad checks auth.isAuthenticated)
  /lobby → LobbyPage
  /profile → ProfilePage
  /game/$gameID → GamePage
  /admin → AdminPage (redirects non-admins)
```

`createRootRouteWithContext<AuthContextValue>()` injects the auth context into every route's `beforeLoad`. Auth guards `throw redirect({ to: "/login" })` — they don't render a component.

---

## 5. Page Component Pattern

Pages use `useState` + `useEffect` + `useCallback`. There are no custom hooks beyond `useAuth` and `useSocket`. All pages follow this async-load pattern:

```typescript
// GamePage.tsx, LobbyPage.tsx
const loadGame = useCallback(async (cancelled = false) => {
  setError("");
  try {
    const data = await getGameBootstrap(gameID);
    if (cancelled) return;
    setGame(data);
  } catch (err) {
    if (cancelled) return;
    const apiErr = err as ApiError;
    if (apiErr.status === 401) { auth.clearSession(); await navigate({ to: "/login" }); return; }
    setError(apiErr.message || "Failed to load.");
  } finally {
    if (!cancelled) setLoading(false);
  }
}, [gameID]);

useEffect(() => {
  let cancelled = false;
  void loadGame(cancelled);
  return () => { cancelled = true; };
}, [loadGame]);
```

The `cancelled` flag and the `return () => { cancelled = true; }` cleanup appear in every data-loading effect. HTTP 401 always calls `auth.clearSession()` then navigates to `/login`.

There are no shared component files. All UI is inlined in the page file. Shared Tailwind class strings live in `src/router/pages/styles.ts` (`inputClass`, `buttonPrimaryClass`, `buttonGhostClass`).

---

## 6. Networking

### REST

`src/api/client.ts` — single Axios instance, 10s timeout, request interceptor adds `Authorization: Bearer <token>` from localStorage.

All domain modules call `request<T>(config)`, which wraps Axios and normalizes every error to `ApiError { status, message, details }`. Each module has a normalizer function (e.g., `normalizeGame()` in `src/api/games.ts`, `normalizeUser()` in `src/api/users.ts`) that defensively parses backend responses and provides fallback defaults.

Dev: Vite proxies `/api` → `http://127.0.0.1:8080` (see `vite.config.ts`).
Prod: `VITE_API_BASE_URL` env var baked at build time.

### WebSocket

Core files: `src/realtime/socket.ts`, `src/realtime/SocketProvider.tsx`, `src/realtime/context.ts`

`useGameSocket` manages a single WebSocket per session. Access it via `useSocket()`.

**Sending:**
```typescript
socket.send("game_action", { action: "attack", from: "Ukraine", to: "Afghanistan", attacker_dice: 3 }, { gameId });
```

**Subscribing (and cleaning up):**
```typescript
useEffect(() => {
  const off = socket.on("game_state_updated", (env) => {
    const payload = env.payload as Record<string, unknown>;
    setGame(parseGameState(payload));
  });
  return off;
}, [socket]);
```

`socket.on()` returns an unsubscribe function — always use it as the `useEffect` cleanup.
`socket.on("*", fn)` receives every message.

**Envelope (`src/realtime/types.ts`):**
```typescript
interface WsEnvelope {
  type: string;
  id?: string;
  correlation_id?: string;
  game_id?: string;
  payload?: unknown;
}
```

Reconnection: 250ms × 2^attempt, capped at 8s. Messages sent while disconnected are queued and flushed on reconnect. URL uses `VITE_WS_URL` env var or current window location + token query param.

---

## 7. Game Rendering

File: `src/router/pages/GamePage.tsx`

Board: `risk0.png` as canvas background, interactive game elements as an SVG overlay. No Phaser, no animation library.

SVG structure:
- `<g transform="translate(...) scale(...)">` — centered/scaled container
- `<line>` elements for territory adjacency edges (drawn first, underneath nodes)
- `<circle>` elements for territory nodes with `onClick` handlers
- `<text>` labels for army counts inside each circle
- Selection state: stroke color on the selected circle

Map constants in `src/router/pages/gameShared.ts`:
- `MAP_TERRITORIES: Record<string, { x: number, y: number }>` — pixel positions
- `MAP_EDGES: [Territory, Territory][]` — adjacency pairs
- `MAP_PLAYER_COLORS: string[]` — indexed by player index
- `CONTINENT_TERRITORIES: Record<string, Territory[]>` — groupings for bonus display

Territory colors, army counts, and ownership all come from `game.territories` (the server snapshot). The phase-specific control panel (`game.phase` string) renders as conditional blocks in `GamePage.tsx` — there are no separate phase components.

**Initial game state** comes from `GET /games/:id/bootstrap` via `getGameBootstrap()` in `src/api/games.ts`. After that, every `game_state_updated` WebSocket message replaces the entire local game state.

---

## 8. State Management

Two React contexts, both at the top of the tree in `src/main.tsx`:

- `AuthContext` (`src/auth/context.ts`) — `{ token, user, isAuthenticated, setSession, clearSession }`, initialized from `localStorage` keys `gc.auth.token` and `gc.auth.user`
- `SocketContext` (`src/realtime/context.ts`) — the `GameSocket` instance from `useGameSocket`

Everything else is page-local `useState`. `GamePage.tsx` owns its game state. `LobbyPage.tsx` owns its lobby state. No state crosses between pages.

---

## 9. Commands

```bash
npm install          # install deps
npm run dev          # Vite dev server — proxies /api and /ws to :8080
npm run build        # tsc -b && vite build → dist/
npm run lint         # ESLint
npm run preview      # serve dist/ locally
```

Backend must be on `:8080`: `cd backend && make dev`.

---

## 10. Canonical Examples

| Pattern | Location |
|---|---|
| Async data load with cancelled flag | `src/router/pages/GamePage.tsx` — `loadGame` |
| WebSocket send | `src/router/pages/LobbyPage.tsx` — `handleCreateGame` |
| WebSocket subscribe + cleanup | `src/router/pages/GamePage.tsx` — `game_state_updated` listener |
| Auth guard | `src/router/index.tsx` — `beforeLoad` on `/app` |
| Defensive response parsing | `src/api/games.ts` — `normalizeGame` |
| Token injection + error normalization | `src/api/client.ts` |
| Reconnection + message queue | `src/realtime/socket.ts` — `useGameSocket` |
| SVG board with click handlers | `src/router/pages/GamePage.tsx` — SVG section |
| Map data | `src/router/pages/gameShared.ts` |
| Shared Tailwind classes | `src/router/pages/styles.ts` |
