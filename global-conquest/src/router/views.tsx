import { useCallback, useEffect, useState } from "react";
import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../api/client";
import { login, signup } from "../api/auth";
import { createGame, joinGame, listGames, type GameRecord } from "../api/games";
import { getUserByUsername, type UserRecord } from "../api/users";
import { useAuth } from "../auth";

export function RootLayout() {
  return <Outlet />;
}

export function LoginPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({ username: "", password: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>("");

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await login({
        username: form.username.trim(),
        password: form.password,
      });
      if (!res.token || !res.user.id || !res.user.username) {
        throw new Error("invalid login response from server");
      }
      auth.setSession(res.token, {
        id: res.user.id,
        username: res.user.username,
        role: res.user.role,
      });
      await navigate({ to: "/app/lobby" });
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.message || "Login failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="page">
      <h1>Login</h1>
      <form className="auth-form" onSubmit={onSubmit}>
        <label>
          Username
          <input
            type="text"
            autoComplete="username"
            minLength={3}
            maxLength={24}
            value={form.username}
            onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            autoComplete="current-password"
            minLength={8}
            maxLength={128}
            value={form.password}
            onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
            required
          />
        </label>
        {error ? <p className="form-error">{error}</p> : null}
        <button type="submit" disabled={submitting}>
          {submitting ? "Signing in..." : "Login"}
        </button>
      </form>
      <p>
        Need an account? <Link to="/signup">Sign up</Link>
      </p>
    </main>
  );
}

export function SignupPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({
    username: "",
    password: "",
    confirmPassword: "",
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>("");

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError("");
    if (form.password !== form.confirmPassword) {
      setError("Passwords do not match");
      return;
    }

    setSubmitting(true);
    try {
      await signup({
        username: form.username.trim(),
        password: form.password,
      });
      const loginRes = await login({
        username: form.username.trim(),
        password: form.password,
      });
      if (!loginRes.token || !loginRes.user.id || !loginRes.user.username) {
        throw new Error("invalid login response from server");
      }
      auth.setSession(loginRes.token, {
        id: loginRes.user.id,
        username: loginRes.user.username,
        role: loginRes.user.role,
      });
      await navigate({ to: "/app/lobby" });
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.message || "Signup failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="page">
      <h1>Sign Up</h1>
      <form className="auth-form" onSubmit={onSubmit}>
        <label>
          Username
          <input
            type="text"
            autoComplete="username"
            minLength={3}
            maxLength={24}
            value={form.username}
            onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={128}
            value={form.password}
            onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
            required
          />
        </label>
        <label>
          Confirm Password
          <input
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={128}
            value={form.confirmPassword}
            onChange={(e) => setForm((prev) => ({ ...prev, confirmPassword: e.target.value }))}
            required
          />
        </label>
        {error ? <p className="form-error">{error}</p> : null}
        <button type="submit" disabled={submitting}>
          {submitting ? "Creating account..." : "Create account"}
        </button>
      </form>
      <p>
        Already have an account? <Link to="/login">Login</Link>
      </p>
    </main>
  );
}

export function AppShell() {
  const navigate = useNavigate();
  const auth = useAuth();

  const onLogout = async () => {
    auth.clearSession();
    await navigate({ to: "/login" });
  };

  return (
    <main className="page">
      <header className="app-header">
        <h1>Global Conquest</h1>
        <nav className="app-nav">
          <Link to="/app/lobby">Lobby</Link>
          <Link to="/app/profile">Profile</Link>
          <button type="button" onClick={onLogout}>
            Logout
          </button>
        </nav>
      </header>
      <section>
        <Outlet />
      </section>
    </main>
  );
}

export function LobbyPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [games, setGames] = useState<GameRecord[]>([]);
  const [playerCount, setPlayerCount] = useState(3);
  const [creatingGame, setCreatingGame] = useState(false);
  const [createError, setCreateError] = useState("");
  const [joiningGameID, setJoiningGameID] = useState("");
  const [joinError, setJoinError] = useState("");

  const loadGames = useCallback(async (cancelled = false) => {
    setError("");
    try {
      const loadedGames = await listGames();
      if (cancelled) return;
      setGames(loadedGames);
    } catch (err) {
      if (cancelled) return;
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      setError(apiErr.message || "Failed to load lobby data");
    }
  }, [auth, navigate]);

  useEffect(() => {
    let cancelled = false;

    const run = async () => {
      setLoading(true);
      await loadGames(cancelled);
      if (!cancelled) setLoading(false);
    };

    void run();
    return () => {
      cancelled = true;
    };
  }, [loadGames]);

  const onCreateGame = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setCreateError("");
    if (playerCount < 3 || playerCount > 6) {
      setCreateError("Player count must be between 3 and 6.");
      return;
    }

    setCreatingGame(true);
    try {
      await createGame({ playerCount });
      await loadGames();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      setCreateError(apiErr.message || "Failed to create game");
    } finally {
      setCreatingGame(false);
    }
  };

  const onJoinGame = async (gameID: string) => {
    setJoinError("");
    setJoiningGameID(gameID);
    try {
      await joinGame(gameID);
      await loadGames();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      setJoinError(apiErr.message || "Failed to join game");
    } finally {
      setJoiningGameID("");
    }
  };

  const currentUserID = auth.user?.id ?? "";
  const gamesSorted = [...games].sort((a, b) => {
    if (a.status === b.status) return b.createdAt.localeCompare(a.createdAt);
    if (a.status === "lobby") return -1;
    if (b.status === "lobby") return 1;
    return 0;
  });

  return (
    <div className="lobby">
      <div className="lobby-head">
        <div>
          <h2>Lobby</h2>
          <p>Welcome back, {auth.user?.username}.</p>
        </div>
        <button type="button" onClick={() => void loadGames()} disabled={loading || creatingGame || !!joiningGameID}>
          Refresh
        </button>
      </div>
      {loading ? <p>Loading lobby data...</p> : null}
      {error ? <p className="form-error">{error}</p> : null}
      {!loading && !error ? (
        <>
          <div className="create-game-panel">
            <h3>Create Game</h3>
            <form className="create-game-form" onSubmit={onCreateGame}>
              <label>
                Player Count
                <input
                  type="number"
                  min={3}
                  max={6}
                  value={playerCount}
                  onChange={(e) => setPlayerCount(Number(e.target.value))}
                  required
                />
              </label>
              <button type="submit" disabled={creatingGame}>
                {creatingGame ? "Creating game..." : "Create Game"}
              </button>
            </form>
            {createError ? <p className="form-error">{createError}</p> : null}
          </div>

          <h3>Games</h3>
          {gamesSorted.length === 0 ? <p>No games yet. Create one below.</p> : null}
          <ul className="game-list">
            {gamesSorted.map((g) => {
              const currentPlayers = g.playerIds.length;
              const maxPlayers = g.playerCount ?? 0;
              const hasJoined = currentUserID !== "" && g.playerIds.includes(currentUserID);
              const isLobby = g.status === "lobby";
              const canJoin = isLobby && maxPlayers > 0 && currentPlayers < maxPlayers && !hasJoined;
              const isFull = isLobby && maxPlayers > 0 && currentPlayers >= maxPlayers;
              const statusLabel = canJoin
                ? "Open"
                : hasJoined
                  ? "Joined"
                  : isFull
                    ? "Full"
                    : isLobby
                      ? "Unavailable"
                      : "In Progress";

              return (
                <li key={g.id} className="game-item">
                  <div className="game-meta">
                    <div className="game-top">
                      <span className="game-id">{g.id}</span>
                      <span className="game-badge">{statusLabel}</span>
                    </div>
                    <div>Owner: {g.ownerUserId || "unknown"}</div>
                    <div>Status: {g.status}</div>
                    <div>Players: {maxPlayers > 0 ? `${currentPlayers}/${maxPlayers}` : "unknown"}</div>
                  </div>
                  <div className="game-actions">
                    {canJoin ? (
                      <button type="button" onClick={() => void onJoinGame(g.id)} disabled={joiningGameID === g.id}>
                        {joiningGameID === g.id ? "Joining..." : "Join Game"}
                      </button>
                    ) : (
                      <button type="button" disabled>
                        {hasJoined ? "Already Joined" : isFull ? "Lobby Full" : "Not Joinable"}
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
          {joinError ? <p className="form-error">{joinError}</p> : null}
        </>
      ) : null}
    </div>
  );
}

export function ProfilePage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [profile, setProfile] = useState<UserRecord | null>(null);

  useEffect(() => {
    const username = auth.user?.username;
    if (!username) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    const run = async () => {
      setLoading(true);
      setError("");
      try {
        const loaded = await getUserByUsername(username);
        if (!cancelled) setProfile(loaded);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        setError(apiErr.message || "Failed to load profile");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void run();
    return () => {
      cancelled = true;
    };
  }, [auth, navigate]);

  return (
    <div>
      <h2>Profile</h2>
      {loading ? <p>Loading profile...</p> : null}
      {error ? <p className="form-error">{error}</p> : null}
      {!loading && !error ? (
        <>
          <p>Username: {profile?.username ?? auth.user?.username ?? "-"}</p>
          <p>User ID: {profile?.id ?? auth.user?.id ?? "-"}</p>
          <p>Role: {profile?.role ?? auth.user?.role ?? "-"}</p>
        </>
      ) : null}
    </div>
  );
}
