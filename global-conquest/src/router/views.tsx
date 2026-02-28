import { useCallback, useEffect, useRef, useState } from "react";
import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../api/client";
import { login, signup } from "../api/auth";
import { listLobbyMessages, normalizeLobbyMessage, postLobbyMessage, type LobbyMessage } from "../api/chat";
import { createGame, joinGame, listGames, type GameRecord } from "../api/games";
import {
  getUserByUsername,
  listAdminUsers,
  revokeUserSessions,
  updateUserAccess,
  type AdminUserRecord,
  type UserRecord,
} from "../api/users";
import { useAuth } from "../auth";
import { useSocket } from "../realtime";

const inputClass =
  "w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500 focus:ring-2 focus:ring-slate-200";

const buttonPrimaryClass =
  "inline-flex items-center justify-center rounded-lg border border-slate-900 bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60";

const buttonGhostClass =
  "inline-flex items-center justify-center rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60";

export function RootLayout() {
  return <Outlet />;
}

export function LoginPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({ username: "", password: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

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
    <main className="mx-auto flex min-h-screen w-full max-w-5xl items-center px-4 py-10">
      <section className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Welcome Back</h1>
        <p className="mt-1 text-sm text-slate-600">Sign in to continue to your lobby.</p>

        <form className="mt-6 grid gap-4" onSubmit={onSubmit}>
          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
            Username
            <input
              className={inputClass}
              type="text"
              autoComplete="username"
              minLength={3}
              maxLength={24}
              value={form.username}
              onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
              required
            />
          </label>

          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
            Password
            <input
              className={inputClass}
              type="password"
              autoComplete="current-password"
              minLength={8}
              maxLength={128}
              value={form.password}
              onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
              required
            />
          </label>

          {error ? <p className="text-sm text-rose-700">{error}</p> : null}

          <button className={buttonPrimaryClass} type="submit" disabled={submitting}>
            {submitting ? "Signing in..." : "Login"}
          </button>
        </form>

        <p className="mt-4 text-sm text-slate-600">
          Need an account?{" "}
          <Link className="font-medium text-slate-900 underline-offset-2 hover:underline" to="/signup">
            Sign up
          </Link>
        </p>
      </section>
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
  const [error, setError] = useState("");

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
    <main className="mx-auto flex min-h-screen w-full max-w-5xl items-center px-4 py-10">
      <section className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Create Account</h1>
        <p className="mt-1 text-sm text-slate-600">Set up your account to start playing.</p>

        <form className="mt-6 grid gap-4" onSubmit={onSubmit}>
          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
            Username
            <input
              className={inputClass}
              type="text"
              autoComplete="username"
              minLength={3}
              maxLength={24}
              value={form.username}
              onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
              required
            />
          </label>

          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
            Password
            <input
              className={inputClass}
              type="password"
              autoComplete="new-password"
              minLength={8}
              maxLength={128}
              value={form.password}
              onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
              required
            />
          </label>

          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
            Confirm Password
            <input
              className={inputClass}
              type="password"
              autoComplete="new-password"
              minLength={8}
              maxLength={128}
              value={form.confirmPassword}
              onChange={(e) => setForm((prev) => ({ ...prev, confirmPassword: e.target.value }))}
              required
            />
          </label>

          {error ? <p className="text-sm text-rose-700">{error}</p> : null}

          <button className={buttonPrimaryClass} type="submit" disabled={submitting}>
            {submitting ? "Creating account..." : "Create account"}
          </button>
        </form>

        <p className="mt-4 text-sm text-slate-600">
          Already have an account?{" "}
          <Link className="font-medium text-slate-900 underline-offset-2 hover:underline" to="/login">
            Login
          </Link>
        </p>
      </section>
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
    <main className="mx-auto min-h-screen w-full max-w-5xl px-4 py-8">
      <header className="rounded-2xl border border-slate-200 bg-white px-5 py-4 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-lg font-semibold tracking-tight text-slate-900">Global Conquest</h1>
          <nav className="flex items-center gap-2">
            <Link
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
              to="/app/lobby"
            >
              Lobby
            </Link>
            <Link
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
              to="/app/profile"
            >
              Profile
            </Link>
            {auth.user?.role === "admin" ? (
              <Link
                className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
                to="/app/admin"
              >
                Admin
              </Link>
            ) : null}
            <button className={buttonGhostClass} type="button" onClick={onLogout}>
              Logout
            </button>
          </nav>
        </div>
      </header>

      <section className="mt-4"> 
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
  const [messages, setMessages] = useState<LobbyMessage[]>([]);
  const [chatBody, setChatBody] = useState("");
  const [chatSending, setChatSending] = useState(false);
  const [chatError, setChatError] = useState("");
  const [typingUsers, setTypingUsers] = useState<string[]>([]);
  const { on, send, status: wsStatus } = useSocket();
  const chatScrollRef = useRef<HTMLDivElement | null>(null);
  const upsertMessage = useCallback((next: LobbyMessage) => {
    setMessages((prev) => {
      if (next.id && prev.some((m) => m.id === next.id)) {
        return prev.map((m) => (m.id === next.id ? next : m));
      }
      return [...prev, next];
    });
  }, []);

  const loadGames = useCallback(
    async (cancelled = false) => {
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
    },
    [auth, navigate]
  );

  const loadMessages = useCallback(
    async (cancelled = false) => {
      setChatError("");
      try {
        const loadedMessages = await listLobbyMessages(100);
        if (cancelled) return;
        setMessages(loadedMessages);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        setChatError(apiErr.message || "Failed to load chat");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;

    const run = async () => {
      setLoading(true);
      await loadGames(cancelled);
      await loadMessages(cancelled);
      if (!cancelled) setLoading(false);
    };

    void run();
    return () => {
      cancelled = true;
    };
  }, [loadGames, loadMessages]);

  useEffect(() => {
    const off = on("lobby_typing_state", (msg) => {
      const payload = msg.payload as { users?: unknown } | undefined;
      const users = Array.isArray(payload?.users)
        ? payload.users.filter((v): v is string => typeof v === "string")
        : [];
      setTypingUsers(users);
    });
    return off;
  }, [on]);

  useEffect(() => {
    const off = on("lobby_chat_message", (msg) => {
      upsertMessage(normalizeLobbyMessage(msg.payload));
    });
    return off;
  }, [on, upsertMessage]);

  useEffect(() => {
    const el = chatScrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages]);

  const isTyping = chatBody.trim() !== "";

  useEffect(() => {
    if (wsStatus !== "connected") return;

    if (!isTyping) {
      send("lobby_typing_stop");
      return;
    }

    send("lobby_typing_start", { username: auth.user?.username ?? "anon" });
    const id = window.setInterval(() => {
      send("lobby_typing_start", { username: auth.user?.username ?? "anon" });
    }, 1500);

    return () => {
      window.clearInterval(id);
      send("lobby_typing_stop");
    };
  }, [isTyping, wsStatus, send, auth.user?.username]);

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

  const onSendChat = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const body = chatBody.trim();
    setChatError("");
    if (!body) return;
    setChatSending(true);
    try {
      const created = await postLobbyMessage(body);
      setChatBody("");
      if (wsStatus !== "connected") {
        upsertMessage(created);
      }
      if (wsStatus === "connected") {
        send("lobby_typing_stop");
      }
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      setChatError(apiErr.message || "Failed to send message");
    } finally {
      setChatSending(false);
    }
  };

  const onChatBodyChange = (value: string) => {
    setChatBody(value);
  };

  const typingText = (() => {
    if (typingUsers.length === 0) return "";
    if (typingUsers.length > 2) return "Many people are typing...";
    if (typingUsers.length === 2) return `${typingUsers[0]} and ${typingUsers[1]} are typing...`;
    return `${typingUsers[0]} is typing...`;
  })();

  const currentUserID = auth.user?.id ?? "";
  const gamesSorted = [...games].sort((a, b) => {
    if (a.status === b.status) return b.createdAt.localeCompare(a.createdAt);
    if (a.status === "lobby") return -1;
    if (b.status === "lobby") return 1;
    return 0;
  });

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
      <div className="grid gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-slate-900">Lobby</h2>
          <p className="text-sm text-slate-600">Welcome back, {auth.user?.username}.</p>
        </div>
        <button
          className={buttonGhostClass}
          type="button"
          onClick={() => void loadGames()}
          disabled={loading || creatingGame || !!joiningGameID}
        >
          Refresh
        </button>
      </div>

      <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Create Game</h3>
        <form className="mt-3 flex flex-wrap items-end gap-3" onSubmit={onCreateGame}>
          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
            Player Count
            <input
              className={`${inputClass} w-32`}
              type="number"
              min={3}
              max={6}
              value={playerCount}
              onChange={(e) => setPlayerCount(Number(e.target.value))}
              required
            />
          </label>
          <button className={buttonPrimaryClass} type="submit" disabled={creatingGame}>
            {creatingGame ? "Creating..." : "Create"}
          </button>
        </form>
        {createError ? <p className="mt-2 text-sm text-rose-700">{createError}</p> : null}
      </section>

      <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Games</h3>
          <span className="text-xs text-slate-500">{gamesSorted.length} total</span>
        </div>

        {loading ? <p className="text-sm text-slate-600">Loading lobby data...</p> : null}
        {error ? <p className="text-sm text-rose-700">{error}</p> : null}
        {joinError ? <p className="mb-2 text-sm text-rose-700">{joinError}</p> : null}

        {!loading && !error && gamesSorted.length === 0 ? (
          <p className="text-sm text-slate-600">No games yet. Create one above.</p>
        ) : null}

        <ul className="grid gap-2">
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
              <li
                key={g.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-3"
              >
                <div className="grid gap-0.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-xs text-slate-700">{g.id}</span>
                    <span className="rounded-full border border-slate-300 bg-white px-2 py-0.5 text-[11px] font-medium text-slate-700">
                      {statusLabel}
                    </span>
                  </div>
                  <p className="text-xs text-slate-600">Owner: {g.ownerUserId || "unknown"}</p>
                  <p className="text-xs text-slate-600">
                    Players: {maxPlayers > 0 ? `${currentPlayers}/${maxPlayers}` : "unknown"}
                  </p>
                </div>

                {canJoin ? (
                  <button
                    className={buttonPrimaryClass}
                    type="button"
                    onClick={() => void onJoinGame(g.id)}
                    disabled={joiningGameID === g.id}
                  >
                    {joiningGameID === g.id ? "Joining..." : "Join Game"}
                  </button>
                ) : (
                  <button className={buttonGhostClass} type="button" disabled>
                    {hasJoined ? "Joined" : isFull ? "Lobby Full" : "Not Joinable"}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      </section>
      </div>

      <aside className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
        <div className="mb-3 flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Lobby Chat</h3>
          <button className={buttonGhostClass} type="button" onClick={() => void loadMessages()} disabled={loading || chatSending}>
            Refresh
          </button>
        </div>

        <div ref={chatScrollRef} className="h-[380px] overflow-y-auto rounded-xl border border-slate-200 bg-slate-50 p-3">
          {messages.length === 0 ? <p className="text-sm text-slate-500">No messages yet.</p> : null}
          <ul className="grid gap-2">
            {messages.map((m) => (
              <li key={m.id || `${m.userId}-${m.createdAt}-${m.body}`} className="rounded-lg bg-white p-2 text-sm">
                <div className="mb-1 flex items-center justify-between gap-2">
                  <span className="font-medium text-slate-900">{m.userName || "unknown"}</span>
                  <span className="text-[11px] text-slate-500">{m.createdAt ? new Date(m.createdAt).toLocaleTimeString() : ""}</span>
                </div>
                <p className="whitespace-pre-wrap text-slate-700">{m.body}</p>
              </li>
            ))}
          </ul>
        </div>

        <form className="mt-3 grid gap-2" onSubmit={onSendChat}>
          <textarea
            className={inputClass}
            rows={3}
            maxLength={1000}
            placeholder="Message lobby..."
            value={chatBody}
            onChange={(e) => onChatBodyChange(e.target.value)}
          />
          {typingText ? <p className="text-xs text-slate-500">{typingText}</p> : null}
          {chatError ? <p className="text-sm text-rose-700">{chatError}</p> : null}
          <button className={buttonPrimaryClass} type="submit" disabled={chatSending || chatBody.trim() === ""}>
            {chatSending ? "Sending..." : "Send"}
          </button>
        </form>
      </aside>
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
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <h2 className="text-2xl font-semibold tracking-tight text-slate-900">Profile</h2>
      {loading ? <p className="mt-3 text-sm text-slate-600">Loading profile...</p> : null}
      {error ? <p className="mt-3 text-sm text-rose-700">{error}</p> : null}
      {!loading && !error ? (
        <dl className="mt-4 grid gap-2 text-sm text-slate-700">
          <div className="grid grid-cols-[100px_1fr] gap-2">
            <dt className="text-slate-500">Username</dt>
            <dd className="font-medium text-slate-900">{profile?.username ?? auth.user?.username ?? "-"}</dd>
          </div>
          <div className="grid grid-cols-[100px_1fr] gap-2">
            <dt className="text-slate-500">User ID</dt>
            <dd className="font-mono text-xs text-slate-700">{profile?.id ?? auth.user?.id ?? "-"}</dd>
          </div>
          <div className="grid grid-cols-[100px_1fr] gap-2">
            <dt className="text-slate-500">Role</dt>
            <dd className="font-medium text-slate-900">{profile?.role ?? auth.user?.role ?? "-"}</dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}

export function AdminPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [users, setUsers] = useState<AdminUserRecord[]>([]);
  const [busyUserID, setBusyUserID] = useState("");

  const load = useCallback(
    async (cancelled = false) => {
      setError("");
      try {
        const out = await listAdminUsers();
        if (cancelled) return;
        setUsers(out);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        if (apiErr.status === 403) {
          await navigate({ to: "/app/lobby" });
          return;
        }
        setError(apiErr.message || "Failed to load admin users");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setLoading(true);
      await load(cancelled);
      if (!cancelled) setLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [load]);

  const patchAccess = async (u: AdminUserRecord, accessStatus: "active" | "blocked") => {
    setBusyUserID(u.id);
    setError("");
    try {
      await updateUserAccess(u.id, accessStatus);
      await load();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      if (apiErr.status === 403) {
        await navigate({ to: "/app/lobby" });
        return;
      }
      setError(apiErr.message || "Failed to update user access");
    } finally {
      setBusyUserID("");
    }
  };

  const onRevokeSessions = async (u: AdminUserRecord) => {
    setBusyUserID(u.id);
    setError("");
    try {
      await revokeUserSessions(u.id);
      await load();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      if (apiErr.status === 403) {
        await navigate({ to: "/app/lobby" });
        return;
      }
      setError(apiErr.message || "Failed to revoke sessions");
    } finally {
      setBusyUserID("");
    }
  };

  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-slate-900">Admin Dashboard</h2>
          <p className="text-sm text-slate-600">Manage user access and active sessions.</p>
        </div>
        <button className={buttonGhostClass} type="button" onClick={() => void load()} disabled={loading || !!busyUserID}>
          Refresh
        </button>
      </div>

      {loading ? <p className="text-sm text-slate-600">Loading users...</p> : null}
      {error ? <p className="mb-3 text-sm text-rose-700">{error}</p> : null}

      <div className="overflow-x-auto rounded-xl border border-slate-200">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50">
            <tr>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">User</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Role</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Access</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Sessions</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {users.map((u) => {
              const isBusy = busyUserID === u.id;
              const isBlocked = u.accessStatus === "blocked";
              const accessLabel = isBlocked ? "Blocked" : "Active";
              return (
                <tr key={u.id}>
                  <td className="px-3 py-2">
                    <p className="font-medium text-slate-900">{u.username}</p>
                    <p className="font-mono text-[11px] text-slate-500">{u.id}</p>
                  </td>
                  <td className="px-3 py-2 text-slate-700">{u.role}</td>
                  <td className="px-3 py-2 text-slate-700">{accessLabel}</td>
                  <td className="px-3 py-2 text-slate-700">{u.activeSessions}</td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-2">
                      {isBlocked ? (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "active")}
                        >
                          Unblock
                        </button>
                      ) : (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "blocked")}
                        >
                          Block
                        </button>
                      )}
                      <button className={buttonPrimaryClass} type="button" disabled={isBusy} onClick={() => void onRevokeSessions(u)}>
                        Revoke Sessions
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
