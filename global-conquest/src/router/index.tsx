import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";
import type { AuthContextValue } from "../auth";
import {
  AdminPage,
  AdminPlayerStatsPage,
  AdminStatsPage,
  AppShell,
  GamePage,
  GameReplayPage,
  LeaderboardPage,
  LobbyPage,
  LoginPage,
  MapEditorPage,
  MapsPage,
  ProfilePage,
  RootLayout,
  SignupPage,
} from "./views";
import { validatePlayerStatsSearch, validateStatsSearch } from "./pages/statsFormat";

type RouterContext = {
  auth: AuthContextValue;
};

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: ({ context }) => {
    if (context.auth.isAuthenticated) {
      throw redirect({ to: "/app/lobby" });
    }
    throw redirect({ to: "/login" });
  },
  component: () => null,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  beforeLoad: ({ context }) => {
    if (context.auth.isAuthenticated) {
      throw redirect({ to: "/app/lobby" });
    }
  },
  component: LoginPage,
});

const signupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/signup",
  beforeLoad: ({ context }) => {
    if (context.auth.isAuthenticated) {
      throw redirect({ to: "/app/lobby" });
    }
  },
  component: SignupPage,
});

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/app",
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: "/login" });
    }
  },
  component: AppShell,
});

const appIndexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/app/lobby" });
  },
  component: () => null,
});

const lobbyRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/lobby",
  component: LobbyPage,
});

const profileRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/profile",
  component: ProfilePage,
});

const gameRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/game/$gameID",
  component: GamePage,
});

// A separate route (not a mode toggled within GamePage) so the replay
// view is a fully distinct component tree from the live game: it never
// mounts the action-submission panel, never subscribes to the live
// game_state_updated socket channel, and never shares GamePage's `game`
// state slot. See project-docs/game_replay/00_Game_Replay_Design.md.
const gameReplayRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/game/$gameID/replay",
  component: GameReplayPage,
});

const leaderboardRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/leaderboard",
  component: LeaderboardPage,
});

const adminRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/admin",
  beforeLoad: ({ context }) => {
    if (context.auth.user?.role !== "admin") {
      throw redirect({ to: "/app/lobby" });
    }
  },
  component: AdminPage,
});

const requireAdmin = ({ context }: { context: RouterContext }) => {
  if (context.auth.user?.role !== "admin") {
    throw redirect({ to: "/app/lobby" });
  }
};

// Map building is open to every signed-in user (the /app guard covers
// auth); the editor itself goes read-only on maps the viewer doesn't own.
const mapsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/maps",
  component: MapsPage,
});

// The map editor is one page for both drawing a new map and editing an
// existing one; "new" is a static segment, so it wins over $mapID.
const newMapRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/maps/new",
  // ?generate=true opens the editor on its Generate tab.
  validateSearch: (search: Record<string, unknown>): { generate?: boolean } =>
    search.generate === true || search.generate === "true" ? { generate: true } : {},
  component: MapEditorPage,
});

const editMapRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/maps/$mapID",
  component: MapEditorPage,
});

// Admin player statistics. Filters live in the search params so they
// survive navigating between the overview and a player's page.
const adminStatsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/admin/stats",
  validateSearch: validateStatsSearch,
  beforeLoad: requireAdmin,
  component: AdminStatsPage,
});

const adminPlayerStatsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/admin/stats/$playerKey",
  validateSearch: validatePlayerStatsSearch,
  beforeLoad: requireAdmin,
  component: AdminPlayerStatsPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  signupRoute,
  appRoute.addChildren([appIndexRoute, lobbyRoute, profileRoute, leaderboardRoute, gameRoute, gameReplayRoute, adminRoute, mapsRoute, newMapRoute, editMapRoute, adminStatsRoute, adminPlayerStatsRoute]),
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  context: {
    auth: {
      token: null,
      user: null,
      isAuthenticated: false,
      setSession: () => undefined,
      clearSession: () => undefined,
    },
  },
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
