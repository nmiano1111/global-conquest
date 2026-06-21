import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { listLobbyMessages, normalizeLobbyMessage, postLobbyMessage, type LobbyMessage } from "../../api/chat";
import { createGame, joinGame, listGames, type GameRecord } from "../../api/games";
import { useAuth } from "../../auth";
import { useSocket } from "../../realtime";
import { buttonGhostClass, buttonPrimaryClass, inputClass } from "./styles";

function statusBadgeClass(label: string): string {
  if (label === "Open") return "border border-gc-success/40 bg-gc-success/10 text-gc-success";
  if (label === "Joined") return "border border-gc-accent/40 bg-gc-accent/10 text-gc-accent";
  if (label === "Full") return "border border-gc-danger/40 bg-gc-danger/10 text-gc-danger";
  if (label === "In Progress") return "border border-sky-700/40 bg-sky-900/20 text-sky-400";
  return "border border-gc-border bg-gc-surface-2 text-gc-muted";
}

export function LobbyPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [games, setGames] = useState<GameRecord[]>([]);
  const [playerCount, setPlayerCount] = useState(3);
  const [setupMode, setSetupMode] = useState<"random" | "manual">("random");
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
      await createGame({ playerCount, setupMode: auth.user?.role === "admin" ? setupMode : undefined });
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

  const typingText = (() => {
    if (typingUsers.length === 0) return "";
    if (typingUsers.length > 2) return "Many people are typing…";
    if (typingUsers.length === 2) return `${typingUsers[0]} and ${typingUsers[1]} are typing…`;
    return `${typingUsers[0]} is typing…`;
  })();

  const currentUserID = auth.user?.id ?? "";
  const gamesSorted = [...games].sort((a, b) => {
    if (a.status === b.status) return b.createdAt.localeCompare(a.createdAt);
    if (a.status === "lobby") return -1;
    if (b.status === "lobby") return 1;
    return 0;
  });

  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
      {/* ── Left column ── */}
      <div className="grid gap-5">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold text-gc-text">Lobby</h2>
            <p className="mt-0.5 text-sm text-gc-muted">Welcome back, {auth.user?.username}.</p>
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

        {/* Create game */}
        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <h3 className="text-sm font-semibold text-gc-text">New Game</h3>
          <form className="mt-3 flex flex-wrap items-end gap-3" onSubmit={onCreateGame}>
            <label className="grid gap-1.5 text-xs font-medium text-gc-muted">
              Players
              <input
                className={`${inputClass} w-28`}
                type="number"
                min={3}
                max={6}
                value={playerCount}
                onChange={(e) => setPlayerCount(Number(e.target.value))}
                required
              />
            </label>
            {auth.user?.role === "admin" && (
              <label className="grid gap-1.5 text-xs font-medium text-gc-muted">
                Setup Mode
                <select
                  className={`${inputClass} w-36`}
                  value={setupMode}
                  onChange={(e) => setSetupMode(e.target.value as "random" | "manual")}
                >
                  <option value="random">Random</option>
                  <option value="manual">Manual</option>
                </select>
              </label>
            )}
            <button className={buttonPrimaryClass} type="submit" disabled={creatingGame}>
              {creatingGame ? "Creating…" : "Create Game"}
            </button>
          </form>
          {createError ? (
            <p className="mt-2 text-sm text-gc-danger">{createError}</p>
          ) : null}
        </section>

        {/* Games list */}
        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-gc-text">Games</h3>
            <span className="text-xs text-gc-muted">{gamesSorted.length} total</span>
          </div>

          {loading ? (
            <p className="text-sm text-gc-muted">Loading…</p>
          ) : null}
          {error ? (
            <p className="text-sm text-gc-danger">{error}</p>
          ) : null}
          {joinError ? (
            <p className="mb-2 text-sm text-gc-danger">{joinError}</p>
          ) : null}

          {!loading && !error && gamesSorted.length === 0 ? (
            <p className="py-4 text-center text-sm text-gc-muted">No games yet. Create one above.</p>
          ) : null}

          <ul className="grid gap-2">
            {gamesSorted.map((g) => {
              const currentPlayers = g.playerIds.length;
              const maxPlayers = g.playerCount ?? 0;
              const hasJoined = currentUserID !== "" && g.playerIds.includes(currentUserID);
              const canOpenGame = hasJoined;
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
                  className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-gc-border bg-gc-surface-2 px-3 py-3 transition-colors hover:border-gc-border/80"
                >
                  <div className="grid gap-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-xs text-gc-muted truncate">{g.id}</span>
                      <span
                        className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${statusBadgeClass(statusLabel)}`}
                      >
                        {statusLabel}
                      </span>
                    </div>
                    <p className="text-xs text-gc-muted">
                      <span className="text-gc-text/70">Players</span>{" "}
                      {maxPlayers > 0 ? `${currentPlayers} / ${maxPlayers}` : "—"}
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2 shrink-0">
                    {canOpenGame ? (
                      <Link className={buttonGhostClass} to="/app/game/$gameID" params={{ gameID: g.id }}>
                        Open
                      </Link>
                    ) : (
                      <button className={buttonGhostClass} type="button" disabled>
                        Open
                      </button>
                    )}
                    {canJoin ? (
                      <button
                        className={buttonPrimaryClass}
                        type="button"
                        onClick={() => void onJoinGame(g.id)}
                        disabled={joiningGameID === g.id}
                      >
                        {joiningGameID === g.id ? "Joining…" : "Join"}
                      </button>
                    ) : (
                      <button className={buttonGhostClass} type="button" disabled>
                        {hasJoined ? "Joined" : isFull ? "Full" : "Closed"}
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        </section>
      </div>

      {/* ── Right column: chat ── */}
      <aside className="rounded-xl border border-gc-border bg-gc-surface p-4">
        <div className="mb-3 flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold text-gc-text">Lobby Chat</h3>
          <div className="flex items-center gap-2">
            <span
              className={`h-1.5 w-1.5 rounded-full ${wsStatus === "connected" ? "bg-gc-success" : "bg-gc-danger"}`}
              title={wsStatus}
            />
            <button
              className={buttonGhostClass}
              type="button"
              onClick={() => void loadMessages()}
              disabled={loading || chatSending}
            >
              Refresh
            </button>
          </div>
        </div>

        <div
          ref={chatScrollRef}
          className="h-[380px] overflow-y-auto rounded-lg border border-gc-border bg-gc-surface-2 p-3"
        >
          {messages.length === 0 ? (
            <p className="py-6 text-center text-sm text-gc-muted">No messages yet.</p>
          ) : null}
          <ul className="grid gap-2">
            {messages.map((m) => (
              <li
                key={m.id || `${m.userId}-${m.createdAt}-${m.body}`}
                className="rounded-lg bg-gc-surface px-3 py-2 text-sm"
              >
                <div className="mb-1 flex items-center justify-between gap-2">
                  <span className="font-medium text-gc-text">{m.userName || "unknown"}</span>
                  <span className="text-[11px] text-gc-muted">
                    {m.createdAt
                      ? new Date(m.createdAt).toLocaleString(undefined, {
                          month: "short",
                          day: "numeric",
                          hour: "numeric",
                          minute: "2-digit",
                        })
                      : ""}
                  </span>
                </div>
                <p className="whitespace-pre-wrap text-gc-text/80">{m.body}</p>
              </li>
            ))}
          </ul>
        </div>

        <form className="mt-3 grid gap-2" onSubmit={onSendChat}>
          <textarea
            className={inputClass}
            rows={3}
            maxLength={1000}
            placeholder="Message lobby…"
            value={chatBody}
            onChange={(e) => setChatBody(e.target.value)}
          />
          {typingText ? <p className="text-xs text-gc-muted italic">{typingText}</p> : null}
          {chatError ? <p className="text-sm text-gc-danger">{chatError}</p> : null}
          <button
            className={buttonPrimaryClass}
            type="submit"
            disabled={chatSending || chatBody.trim() === ""}
          >
            {chatSending ? "Sending…" : "Send"}
          </button>
        </form>
      </aside>
    </div>
  );
}
