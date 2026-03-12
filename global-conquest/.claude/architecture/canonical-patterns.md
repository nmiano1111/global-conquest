# Canonical Frontend Implementation Patterns

> This document describes the preferred patterns for generating UI and rendering code in this codebase.
> All patterns are derived from existing source files. No theoretical best practices are included.

---

## 1. React Component Pattern

**Description:** Functional components with hooks, no class components, no external state library.

**Canonical files:**
- `src/router/pages/GamePage.tsx`
- `src/router/pages/LobbyPage.tsx`

**Pattern:**

```
1. Declare local state with useState (loading, error, data, selections)
2. Access auth and socket via useAuth() / useSocket() hooks
3. Load data in a useCallback-wrapped async function; pass a cancelled flag
4. Mount/unmount the loader in useEffect with cleanup
5. Subscribe to socket events in a separate useEffect; return the unsubscribe
6. Derive computed values with useMemo; memoize callbacks with useCallback
7. Render conditional UI based on computed values
```

**Why canonical:**
`GamePage.tsx` applies every hook correctly: async data loading with cancellation guard, socket subscriptions with cleanup, and computed turn/phase values that drive conditional rendering. It is the most complete example in the repo.

**Data loading boilerplate (used verbatim in GamePage and LobbyPage):**

```typescript
const loadData = useCallback(async (cancelled = false) => {
  setError("");
  try {
    const data = await apiCall();
    if (cancelled) return;
    setState(data);
  } catch (err) {
    if (cancelled) return;
    setError(toMessage(err));
  }
}, [dependencies]);

useEffect(() => {
  let cancelled = false;
  const run = async () => {
    setLoading(true);
    await loadData(cancelled);
    if (!cancelled) setLoading(false);
  };
  void run();
  return () => { cancelled = true; };
}, [loadData]);
```

---

## 2. Phaser Scene Pattern

**Not present.** This codebase does not use Phaser. The game map is rendered with an SVG overlay on a `<canvas>` background element. See **Section 5 (Game State Rendering)** for the actual rendering pattern.

---

## 3. WebSocket Client Pattern

**Description:** Native WebSocket with auto-reconnect, send queue, and a pub/sub listener map.

**Canonical files:**
- `src/realtime/socket.ts` — core hook (`useGameSocket`)
- `src/realtime/SocketProvider.tsx` — context provider
- `src/realtime/types.ts` — `WsEnvelope` type

**Pattern:**

```
1. Construct WS URL from env or auto-detect (ws/wss based on location.protocol)
2. Append auth token as query param: ?token={encodeURIComponent(token)}
3. On open: flush sendQueueRef (messages sent before connection was ready)
4. On message: parse JSON → WsEnvelope → emit to listenersRef (by type and "*")
5. On close (not by user): exponential backoff reconnect (250ms base, 2× per attempt, 8s cap)
6. send(type, payload, opts) → enqueue if not open, else send immediately
7. on(type, fn) → register listener, returns unsubscribe function
```

**Wire format (`WsEnvelope`):**

```typescript
type WsEnvelope = {
  type: string;           // message type, e.g. "game_action", "game_state_updated"
  id?: string;            // client-generated UUID (auto-set by send())
  correlation_id?: string;// server echoes this back to correlate responses
  game_id?: string;       // routes message to a specific game room
  payload?: unknown;      // decoded JSON object
};
```

**How to subscribe (from consuming code):**

```typescript
const { on, send, status } = useSocket();

// Subscribe in useEffect; always return the unsubscribe
useEffect(() => {
  const off = on("some_event", (msg: WsEnvelope) => { /* handle */ });
  return off;
}, [on]);
```

**Why canonical:**
`socket.ts` is the single source of truth for all WebSocket behavior. `SocketProvider.tsx` is the only place the URL is constructed and the socket is created.

---

## 4. Server Event Handling Pattern

**Description:** Each server event type is handled in a dedicated `useEffect` that filters by `game_id`, extracts the typed payload, and calls `setState`.

**Canonical file:**
- `src/router/pages/GamePage.tsx` lines 103–234

**Pattern:**

```
1. Call on("event_type", handler) inside useEffect
2. Extract payload with runtime type guards (typeof check, fallback values)
3. Filter irrelevant rooms: if (gameID !== currentGameID) return
4. Derive new state from payload and call setState (never mutate)
5. Return the off() unsubscribe from the useEffect
```

**Implemented events (GamePage):**

| Event | Payload fields | Action |
|---|---|---|
| `game_state_updated` | `game_id, action, phase, current_player, territories, players, pending_reinforcements, occupy, event` | Merge territory state, update phase/player, handle dice results, append event log |
| `game_chat_message` | `game_id, user_name, body, created_at` | Append to `chatMessages` |
| `game_chat_history` | `messages[]` | Replace `chatMessages` |
| `error` | `code, message` | Set `actionError` |

**Implemented events (LobbyPage):**

| Event | Payload fields | Action |
|---|---|---|
| `lobby_chat_message` | `id, user_name, body, created_at` | Upsert into `chatMessages` |
| `lobby_typing_state` | `users: string[]` | Replace `typingUsers` |

**Example:**

```typescript
useEffect(() => {
  const off = on("game_state_updated", (msg: WsEnvelope) => {
    const p = msg.payload as Record<string, unknown> | undefined;
    const gameID = typeof p?.game_id === "string" ? p.game_id : msg.game_id;
    if (gameID !== currentGameID) return;

    const territories = p?.territories as Record<string, TerritoryState> | undefined;
    if (territories) {
      setGame(prev => prev ? { ...prev, territories } : prev);
    }
  });
  return off;
}, [currentGameID, on]);
```

---

## 5. Game State Rendering Pattern

**Description:** Server territory state is mapped to SVG visual properties. No game engine — the map is a `<canvas>` background with a `<svg>` overlay.

**Canonical file:**
- `src/router/pages/GamePage.tsx` lines 652–737
- `src/router/pages/gameShared.ts` — territory positions and adjacency

**Pattern:**

```
1. Receive territory state: Record<string, { armies: number, owner: number }>
2. Compute playerColors[] via useMemo from players array + MAP_PLAYER_COLORS fallback
3. For each territory in TERRITORY_POSITIONS (from gameShared.ts):
   a. Resolve owner index and army count from territory state
   b. Derive fill = playerColors[owner] or neutral grey
   c. Derive stroke/strokeWidth from isSelected flag
4. Render <circle> at (pos.x, pos.y) with derived fill and click handler
5. Render <text> child for army count
6. Render graph edges as <line> elements between adjacent territories
```

**Territory data shape:**

```typescript
// gameShared.ts
export const TERRITORY_POSITIONS: Record<string, { x: number; y: number }> = { ... };
export const TERRITORY_EDGES: [string, string][] = [ ... ];
export const MAP_PLAYER_COLORS: string[] = ["#ef4444", "#3b82f6", "#22c55e", "#a855f7"];
```

**SVG rendering snippet:**

```tsx
<svg style={{ position: "absolute", inset: 0, width: "100%", height: "100%" }}>
  {/* Edges */}
  {TERRITORY_EDGES.map(([a, b]) => (
    <line x1={pos[a].x} y1={pos[a].y} x2={pos[b].x} y2={pos[b].y}
          stroke="#475569" strokeWidth={1.5} key={`${a}-${b}`} />
  ))}

  {/* Territory nodes */}
  {Object.entries(TERRITORY_POSITIONS).map(([name, pos]) => {
    const t = territoryState?.[name];
    const owner = typeof t?.owner === "number" ? t.owner : -1;
    const armies = typeof t?.armies === "number" ? t.armies : 0;
    const fill = owner >= 0 ? playerColors[owner] : "#e2e8f0";
    const isSelected = name === selectedTerritory;
    return (
      <g key={name} onClick={() => onMapTerritoryClick(name)} style={{ cursor: "pointer" }}>
        <circle cx={pos.x} cy={pos.y} r={34} fill={fill}
                stroke={isSelected ? "#0b1220" : "#0f172a"}
                strokeWidth={isSelected ? 5 : 2.7} />
        <text x={pos.x} y={pos.y + 1} textAnchor="middle" dominantBaseline="middle"
              fill="#fff" fontSize={20} fontWeight={700}>{armies}</text>
      </g>
    );
  })}
</svg>
```

**Why canonical:**
This is the only map rendering in the codebase. All territory visualization goes through this SVG layer.

---

## 6. UI Interaction → Network Message Pattern

**Description:** User gesture validates local state, calls `send()`, then waits for a server-pushed `game_state_updated` event. No optimistic mutation of server state.

**Canonical file:**
- `src/router/pages/GamePage.tsx` lines 420–510

**Pattern:**

```
1. User clicks/submits a UI element
2. Validate client-side preconditions (is it my turn? is selection valid? is socket connected?)
3. Set actionError and return early if invalid
4. Call send("game_action", payload, { game_id: gameID })
5. Do NOT update game state locally — wait for "game_state_updated" from server
6. Optionally clear input fields (e.g., setChatDraft(""))
```

**`sendAction` helper used in GamePage:**

```typescript
const sendAction = (payload: Record<string, unknown>) => {
  setActionError("");
  if (wsStatus !== "connected") {
    setActionError("Socket disconnected — please wait");
    return;
  }
  send("game_action", payload, { game_id: gameID });
};
```

**Action payloads by game phase:**

| Phase | User gesture | Message type | Payload |
|---|---|---|---|
| reinforce | Click territory + "Place" button | `game_action` | `{ action: "place_reinforcement", territory, armies }` |
| attack | Select from/to + "Roll Dice" | `game_action` | `{ action: "attack", from, to, attacker_dice, defender_dice }` |
| occupy | Slider + "Move Troops" | `game_action` | `{ action: "occupy", armies }` |
| any | "End Turn" button | `game_action` | `{ action: "end_turn" }` |
| any | Chat form submit | `game_chat_send` | `{ body, username }` |

**Why canonical:**
`GamePage.tsx` contains every in-game action and consistently uses this validate → send → wait pattern. The `sendAction` helper is the single call site for all game actions.

---

## 7. State Management Pattern

**Description:** React Context API for global session state; `useState` for all component-local state. No Redux, Zustand, or other external store.

**Canonical files:**
- `src/auth/AuthProvider.tsx` — session state
- `src/realtime/SocketProvider.tsx` — socket instance
- `src/main.tsx` — provider tree composition

**Provider tree (outermost → innermost):**

```
AuthProvider          → token, user, isAuthenticated, setSession(), clearSession()
  └─ SocketProvider   → socket instance (url built from auth token)
       └─ RouterProvider → routes with auth context
```

**Auth state shape:**

```typescript
type AuthContextValue = {
  token: string | null;
  user: StoredUser | null;
  isAuthenticated: boolean;
  setSession: (token: string, user: StoredUser) => void;
  clearSession: () => void;
};
```

**Auth persistence:** `src/auth/storage.ts` — `localStorage` keys `gc.auth.token` and `gc.auth.user`.

**Access pattern in components:**

```typescript
const auth = useAuth();     // from src/auth/useAuth.ts
const { on, send, status } = useSocket(); // from src/realtime/useSocket.ts
```

**Component-level state responsibility (GamePage pattern):**

```
loading / error          → async fetch lifecycle
chatMessages             → accumulated from server events
chatDraft                → controlled input
game (GameBootstrap)     → loaded once + patched by server events
selectedTerritory        → UI selection for current action
actionError              → validation feedback to user
diceResult               → last dice roll result for display
```

**Why canonical:**
`main.tsx` is the only place providers are composed. `AuthProvider` is the only source of session data.

---

## 8. Animation / Rendering Pattern

**Description:** No animation library. All visual changes are immediate React re-renders driven by state updates. Tailwind transition utilities are used for hover/focus effects only.

**Canonical file:**
- `src/router/pages/GamePage.tsx` lines 652–737 (map)
- `src/router/pages/gameShared.ts` (event log highlighting)

**Map update cycle:**

```
server push "game_state_updated"
  → on() listener calls setGame(prev => { ...prev, territories: newTerritories })
  → React re-renders SVG layer
  → circle fill, strokeWidth, army text update in place
  → no animation, no transition
```

**Event log text highlighting:**

```typescript
// gameShared.ts
export function buildHighlightRegex(players: string[], territories: string[]): RegExp {
  const terms = [...players, ...territories].map(escapeRegex);
  return new RegExp(`(${terms.join("|")})`, "gi");
}

// GamePage renders event messages by splitting on this regex
const parts = msg.text.split(highlightRegex);
parts.map((part, i) =>
  highlightRegex.test(part)
    ? <strong key={i}>{part}</strong>
    : <span key={i}>{part}</span>
);
```

**Selection highlight:** `isSelected` flag on territory node changes `strokeWidth` from 2.7 → 5 and `stroke` to a darker value. No CSS transition.

**Why canonical:**
There is one rendering path (SVG), one update trigger (setState in socket handler), and no animation abstraction to maintain.

---

## 9. Frontend Testing Pattern

**Not present.** There are no test files and no testing libraries in `package.json`. There is no test runner configured.

If tests are added, the existing patterns suggest using Vitest (compatible with the Vite build) and React Testing Library for component tests.

---

## Appendix: API Client Pattern

All REST calls go through the shared Axios instance with a Bearer token interceptor.

**Canonical files:**
- `src/api/client.ts` — Axios instance + `request<T>()` helper
- `src/api/games.ts` — example domain module

**Pattern:**

```
1. Import request() from client.ts
2. Call request<unknown>({ method, url, data })
3. Pass result through a normalize function (handles snake_case/camelCase, missing fields)
4. Return the normalized typed value
```

**Normalize pattern:**

```typescript
function normalizeGame(value: unknown): GameRecord {
  const r = asRecord(value);
  if (!r) return defaultGameRecord;
  return {
    id: readString(r.id ?? r.ID),
    ownerUserId: readString(r.owner_user_id ?? r.OwnerUserID),
    // ...
  };
}
```

**Why canonical:**
`client.ts` is the only Axios instance. Every API module calls `request()` — no direct `axios.get()` calls exist in the codebase.

---

## Appendix: Route Guard Pattern

**Canonical file:** `src/router/index.tsx`

```typescript
createRoute({
  path: "/app",
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: "/login" });
    }
  },
  component: AppShell,
});
```

Auth context is passed into the router via `createRouter({ context: { auth } })` in `App.tsx`.
