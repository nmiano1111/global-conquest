import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getGameBootstrap, type GameBootstrap } from "../../api/games";
import riskBoardImage from "../../assets/images/risk0.png";
import { useAuth } from "../../auth";
import { useSocket } from "../../realtime";
import { buttonGhostClass, buttonPrimaryClass, inputClass } from "./styles";
import {
  MAP_CENTER_X,
  MAP_CENTER_Y,
  MAP_EDGES,
  MAP_OVERLAY_OFFSET_X,
  MAP_OVERLAY_OFFSET_Y,
  MAP_OVERLAY_SCALE,
  MAP_PLAYER_COLORS,
  MAP_TERRITORIES,
  MAP_VIEWBOX_HEIGHT,
  MAP_VIEWBOX_WIDTH,
  type DiceRollResult,
  type GameChatMessage,
} from "./gameShared";

type ActionMode = "reinforce" | "attack" | "fortify" | "occupy";

export function GamePage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const { gameID } = useParams({ from: "/app/game/$gameID" });
  const { on, send, status: wsStatus } = useSocket();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [game, setGame] = useState<GameBootstrap | null>(null);
  const [chatMessages, setChatMessages] = useState<GameChatMessage[]>([]);
  const [chatDraft, setChatDraft] = useState("");
  const [chatError, setChatError] = useState("");
  const [actionError, setActionError] = useState("");
  const [actionMode, setActionMode] = useState<ActionMode>("reinforce");
  const [selectedTerritory, setSelectedTerritory] = useState("");
  const [selectedFrom, setSelectedFrom] = useState("");
  const [selectedTo, setSelectedTo] = useState("");
  const [armiesInput, setArmiesInput] = useState(1);
  const [attackerDice, setAttackerDice] = useState(3);
  const [defenderDice, setDefenderDice] = useState(2);
  const [diceResult, setDiceResult] = useState<DiceRollResult | null>(null);
  const chatScrollRef = useRef<HTMLDivElement | null>(null);

  const loadGame = useCallback(
    async (cancelled = false) => {
      setError("");
      try {
        const out = await getGameBootstrap(gameID);
        if (cancelled) return;
        setGame(out);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        if (apiErr.status === 404) {
          setError("Game not found.");
          return;
        }
        if (apiErr.status === 403) {
          setError("You do not have access to this game. Join it from the lobby first.");
          return;
        }
        setError(apiErr.message || "Failed to load game.");
      }
    },
    [auth, gameID, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setLoading(true);
      await loadGame(cancelled);
      if (!cancelled) setLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [loadGame]);

  useEffect(() => {
    const off = on("game_chat_message", (msg) => {
      const payload = msg.payload as Record<string, unknown> | undefined;
      const payloadGameID =
        typeof payload?.game_id === "string"
          ? payload.game_id
          : typeof payload?.gameID === "string"
            ? payload.gameID
            : msg.game_id;
      if (payloadGameID !== gameID) return;
      const next: GameChatMessage = {
        gameID: payloadGameID,
        userName: typeof payload?.user_name === "string" ? payload.user_name : "anon",
        body: typeof payload?.body === "string" ? payload.body : "",
        createdAt:
          typeof payload?.created_at === "string"
            ? payload.created_at
            : new Date().toISOString(),
      };
      setChatMessages((prev) => [...prev, next]);
    });
    return off;
  }, [gameID, on]);

  useEffect(() => {
    const off = on("game_state_updated", (msg) => {
      const payload = msg.payload as Record<string, unknown> | undefined;
      const payloadGameID = typeof payload?.game_id === "string" ? payload.game_id : msg.game_id;
      if (payloadGameID !== gameID) return;
      const phase = typeof payload?.phase === "string" ? payload.phase : "";
      const currentPlayer = typeof payload?.current_player === "number" ? payload.current_player : -1;
      const pendingReinforcements =
        typeof payload?.pending_reinforcements === "number" ? payload.pending_reinforcements : 0;
      const territories =
        payload?.territories && typeof payload.territories === "object"
          ? (payload.territories as Record<string, unknown>)
          : {};
      const incomingPlayersRaw = Array.isArray(payload?.players) ? payload.players : [];
      const incomingPlayers = incomingPlayersRaw
        .filter((v): v is Record<string, unknown> => !!v && typeof v === "object")
        .map((p) => ({
          userId: typeof p.user_id === "string" ? p.user_id : "",
          cardCount: typeof p.card_count === "number" ? p.card_count : 0,
          eliminated: p.eliminated === true,
        }))
        .filter((p) => p.userId !== "");

      setGame((prev) => {
        if (!prev) return prev;
        const metaByID = new Map(prev.players.map((p) => [p.userId, p]));
        const nextPlayers = incomingPlayers.map((p, idx) => {
          const meta = metaByID.get(p.userId);
          return {
            userId: p.userId,
            userName: meta?.userName || p.userId,
            color: meta?.color || MAP_PLAYER_COLORS[idx % MAP_PLAYER_COLORS.length],
            cardCount: p.cardCount,
            eliminated: p.eliminated,
          };
        });
        return {
          ...prev,
          phase,
          currentPlayer,
          pendingReinforcements,
          territories,
          players: nextPlayers,
        };
      });
    });
    return off;
  }, [gameID, on]);

  useEffect(() => {
    const off = on("error", (msg) => {
      const payload = msg.payload as Record<string, unknown> | undefined;
      const code = typeof payload?.code === "string" ? payload.code : "";
      const message = typeof payload?.message === "string" ? payload.message : "";
      if (code === "invalid_action" || code === "unauthorized" || code === "not_in_room") {
        setActionError(message || "Action failed.");
      }
    });
    return off;
  }, [on]);

  useEffect(() => {
    const off = on("game_chat_history", (msg) => {
      const payload = msg.payload as { messages?: unknown } | undefined;
      if (!Array.isArray(payload?.messages)) return;
      const next = payload.messages
        .filter((m): m is Record<string, unknown> => !!m && typeof m === "object")
        .map((m) => {
          const payloadGameID =
            typeof m.game_id === "string"
              ? m.game_id
              : typeof msg.game_id === "string"
                ? msg.game_id
                : "";
          return {
            gameID: payloadGameID,
            userName: typeof m.user_name === "string" ? m.user_name : "anon",
            body: typeof m.body === "string" ? m.body : "",
            createdAt: typeof m.created_at === "string" ? m.created_at : new Date().toISOString(),
          } satisfies GameChatMessage;
        })
        .filter((m) => m.gameID === gameID);
      setChatMessages(next);
    });
    return off;
  }, [gameID, on]);

  useEffect(() => {
    if (wsStatus !== "connected") return;
    send("game_chat_join", undefined, { game_id: gameID });
    return () => {
      send("game_chat_leave", undefined, { game_id: gameID });
    };
  }, [gameID, send, wsStatus]);

  useEffect(() => {
    const el = chatScrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [chatMessages]);

  const players = useMemo(() => game?.players ?? [], [game?.players]);
  const phase = game?.phase ?? "";
  const territoryState = game?.territories ?? null;
  const pendingReinforcements = game?.pendingReinforcements ?? 0;
  const meIndex = useMemo(() => players.findIndex((p) => p.userId === auth.user?.id), [players, auth.user?.id]);
  const isMyTurn = meIndex >= 0 && game?.currentPlayer === meIndex;
  const canEnterAttack = pendingReinforcements === 0;
  const phaseMode: ActionMode = phase === "attack" ? "attack" : phase === "fortify" ? "fortify" : phase === "occupy" ? "occupy" : "reinforce";
  const playerColors = useMemo(
    () => players.map((p, i) => p.color || MAP_PLAYER_COLORS[i % MAP_PLAYER_COLORS.length]),
    [players]
  );
  const chatColorByUserName = useMemo(() => {
    const out: Record<string, string> = {};
    players.forEach((p, i) => {
      const key = p.userName.trim().toLowerCase();
      if (!key) return;
      out[key] = p.color || MAP_PLAYER_COLORS[i % MAP_PLAYER_COLORS.length];
    });
    return out;
  }, [players]);

  const selectedFromArmies = useMemo(() => {
    if (!selectedFrom) return 0;
    const tRaw = territoryState?.[selectedFrom];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    return typeof t?.armies === "number" ? t.armies : 0;
  }, [selectedFrom, territoryState]);

  const maxArmiesInput = useMemo(() => {
    if (actionMode === "reinforce") {
      return Math.max(1, game?.pendingReinforcements ?? 1);
    }
    if (actionMode === "fortify" || actionMode === "occupy") {
      return Math.max(1, selectedFromArmies - 1);
    }
    return 50;
  }, [actionMode, game?.pendingReinforcements, selectedFromArmies]);

  useEffect(() => {
    setArmiesInput((prev) => Math.max(1, Math.min(prev, maxArmiesInput)));
  }, [maxArmiesInput]);

  useEffect(() => {
    setActionMode(phaseMode);
  }, [phaseMode]);

  const onSendChat = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setChatError("");
    const body = chatDraft.trim();
    if (!body) return;
    if (wsStatus !== "connected") {
      setChatError("Socket disconnected.");
      return;
    }
    send(
      "game_chat_send",
      {
        body,
        username: auth.user?.username ?? "anon",
      },
      { game_id: gameID }
    );
    setChatDraft("");
  };

  const onRollDice = () => {
    const roll = (count: number) =>
      Array.from({ length: count }, () => Math.floor(Math.random() * 6) + 1).sort((a, b) => b - a);
    const attacker = roll(attackerDice);
    const defender = roll(defenderDice);
    const pairs = Math.min(attacker.length, defender.length);
    let attackerLoss = 0;
    let defenderLoss = 0;
    for (let i = 0; i < pairs; i += 1) {
      if (attacker[i] > defender[i]) defenderLoss += 1;
      else attackerLoss += 1;
    }
    setDiceResult({ attacker, defender, attackerLoss, defenderLoss });
  };

  const sendAction = (payload: Record<string, unknown>) => {
    setActionError("");
    if (wsStatus !== "connected") {
      setActionError("Socket disconnected.");
      return;
    }
    send("game_action", payload, { game_id: gameID });
  };

  const resetSelection = () => {
    setSelectedTerritory("");
    setSelectedFrom("");
    setSelectedTo("");
  };

  const onMapTerritoryClick = (name: string) => {
    setActionError("");
    const tRaw = territoryState?.[name];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    const owner = typeof t?.owner === "number" ? t.owner : -1;
    const isMine = owner >= 0 && owner === meIndex;
    const isEnemy = owner >= 0 && owner !== meIndex;

    if (actionMode === "reinforce") {
      if (!isMine) {
        setActionError("Choose one of your territories for reinforcement.");
        return;
      }
      setSelectedTerritory(name);
      return;
    }
    if (actionMode === "occupy") {
      setSelectedTo(name);
      return;
    }
    if (actionMode === "attack") {
      if (!canEnterAttack) {
        setActionError("Place all reinforcements before attacking.");
        return;
      }
      if (!selectedFrom) {
        if (!isMine) {
          setActionError("Select your attacking territory first.");
          return;
        }
        setSelectedFrom(name);
        return;
      }
      if (!isEnemy) {
        setActionError("Select an enemy territory as attack target.");
        return;
      }
      setSelectedTo(name);
      return;
    }
    if (actionMode === "fortify") {
      if (!selectedFrom) {
        if (!isMine) {
          setActionError("Select your source territory first.");
          return;
        }
        setSelectedFrom(name);
        return;
      }
      if (!isMine) {
        setActionError("Select one of your territories as fortify destination.");
        return;
      }
      setSelectedTo(name);
    }
  };

  const commitReinforcement = () => {
    if (!selectedTerritory) {
      setActionError("Click a territory node first.");
      return;
    }
    sendAction({ action: "place_reinforcement", territory: selectedTerritory, armies: armiesInput });
  };

  const commitAttack = () => {
    if (!selectedFrom || !selectedTo) {
      setActionError("Select source and target territories on the map.");
      return;
    }
    sendAction({
      action: "attack",
      from: selectedFrom,
      to: selectedTo,
      attacker_dice: attackerDice,
      defender_dice: defenderDice,
    });
  };

  const commitFortify = () => {
    if (!selectedFrom || !selectedTo) {
      setActionError("Select source and destination territories on the map.");
      return;
    }
    sendAction({ action: "fortify", from: selectedFrom, to: selectedTo, armies: armiesInput });
  };

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(280px,1fr)]">
      <section className="grid gap-4">
        <header className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-slate-200 bg-white px-4 py-3 shadow-sm">
          <div>
            <h2 className="text-xl font-semibold tracking-tight text-slate-900">Game Room</h2>
            <p className="font-mono text-xs text-slate-600">{gameID}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button className={buttonGhostClass} type="button" onClick={() => void loadGame()} disabled={loading}>
              Refresh
            </button>
            <Link className={buttonGhostClass} to="/app/lobby">
              Back to Lobby
            </Link>
          </div>
        </header>

        {loading ? <p className="text-sm text-slate-600">Loading game...</p> : null}
        {error ? <p className="text-sm text-rose-700">{error}</p> : null}

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Map</h3>
            <span className="text-xs text-slate-500">Status: {game?.status || "-"}</span>
          </div>
          <div className="relative aspect-[2048/1367] w-full overflow-hidden rounded-xl border border-slate-200 bg-slate-50">
            <canvas
              className="h-full w-full rounded-lg border border-slate-200"
              width={2048}
              height={1367}
              style={{
                backgroundImage: `url(${riskBoardImage})`,
                backgroundSize: "cover",
                backgroundPosition: "center",
                backgroundRepeat: "no-repeat",
              }}
              aria-label="Game map canvas placeholder"
            />
            <svg
              className="absolute inset-0 h-full w-full"
              viewBox={`0 0 ${MAP_VIEWBOX_WIDTH} ${MAP_VIEWBOX_HEIGHT}`}
              preserveAspectRatio="xMidYMid meet"
            >
              <g
                transform={`translate(${MAP_CENTER_X + MAP_OVERLAY_OFFSET_X} ${MAP_CENTER_Y + MAP_OVERLAY_OFFSET_Y}) scale(${MAP_OVERLAY_SCALE}) translate(${-MAP_CENTER_X} ${-MAP_CENTER_Y})`}
              >
                {MAP_EDGES.map(([a, b]) => {
                  const from = MAP_TERRITORIES[a];
                  const to = MAP_TERRITORIES[b];
                  if (!from || !to) return null;
                  return (
                    <line
                      key={`${a}|${b}`}
                      x1={from.x}
                      y1={from.y}
                      x2={to.x}
                      y2={to.y}
                      stroke="#0f172a"
                      strokeOpacity={0.35}
                      strokeWidth={3}
                    />
                  );
                })}
                {Object.entries(MAP_TERRITORIES).map(([name, pos]) => {
                  const tRaw = territoryState?.[name];
                  const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
                  const owner = typeof t?.owner === "number" ? t.owner : -1;
                  const armies = typeof t?.armies === "number" ? t.armies : 0;
                  const fill = owner >= 0 ? (playerColors[owner] ?? MAP_PLAYER_COLORS[owner % MAP_PLAYER_COLORS.length]) : "#e2e8f0";
                  return (
                    <g key={name}>
                      <circle
                        cx={pos.x}
                        cy={pos.y}
                        r={34}
                        fill={fill}
                        fillOpacity={0.92}
                        stroke={name === selectedTerritory || name === selectedFrom || name === selectedTo ? "#0b1220" : "#0f172a"}
                        strokeWidth={name === selectedTerritory || name === selectedFrom || name === selectedTo ? 5 : 2.7}
                        className="cursor-pointer"
                        onClick={() => onMapTerritoryClick(name)}
                      />
                      <text
                        x={pos.x}
                        y={pos.y + 1}
                        fill="#ffffff"
                        fontSize={20}
                        fontWeight={700}
                        textAnchor="middle"
                        dominantBaseline="middle"
                      >
                        {armies}
                      </text>
                      <text
                        x={pos.x}
                        y={pos.y + 28}
                        fill="#0f172a"
                        fontSize={11}
                        fontWeight={600}
                        textAnchor="middle"
                        dominantBaseline="hanging"
                        style={{ paintOrder: "stroke", stroke: "rgba(255,255,255,0.75)", strokeWidth: 3 }}
                      >
                        {name}
                      </text>
                    </g>
                  );
                })}
              </g>
            </svg>
          </div>
        </section>

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Event Log</h3>
            <span className="text-xs text-slate-500">Phase: {phase || "-"}</span>
          </div>
          <div className="h-[180px] overflow-y-auto rounded-xl border border-slate-200 bg-slate-50 p-3 text-sm text-slate-600">
            <p>Event log will stream game actions here.</p>
          </div>
        </section>

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Actions</h3>
            <span className="text-xs text-slate-500">Broadcast: realtime</span>
          </div>
          <div className="grid gap-3">
            <div className="flex flex-wrap gap-2">
              <button className={actionMode === "reinforce" ? buttonPrimaryClass : buttonGhostClass} type="button" onClick={() => setActionMode("reinforce")}>
                Reinforce
              </button>
              <button
                className={actionMode === "attack" ? buttonPrimaryClass : buttonGhostClass}
                type="button"
                onClick={() => setActionMode("attack")}
                disabled={!canEnterAttack}
              >
                Attack
              </button>
              <button
                className={actionMode === "fortify" ? buttonPrimaryClass : buttonGhostClass}
                type="button"
                onClick={() => setActionMode("fortify")}
                disabled={!canEnterAttack}
              >
                Fortify
              </button>
              <button
                className={actionMode === "occupy" ? buttonPrimaryClass : buttonGhostClass}
                type="button"
                onClick={() => setActionMode("occupy")}
                disabled={!canEnterAttack}
              >
                Occupy
              </button>
              <button className={buttonGhostClass} type="button" onClick={resetSelection}>
                Clear Selection
              </button>
            </div>
            <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-700">
              Armies Remaining To Place: <span className="font-semibold">{pendingReinforcements}</span>
            </div>

            <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
              <label className="grid gap-1 text-xs font-medium text-slate-600">
                Armies
                <input
                  className={inputClass}
                  type="number"
                  min={1}
                  max={maxArmiesInput}
                  value={armiesInput}
                  onChange={(e) => {
                    const n = Number(e.target.value) || 1;
                    setArmiesInput(Math.max(1, Math.min(n, maxArmiesInput)));
                  }}
                />
              </label>
              <button
                className={buttonPrimaryClass}
                type="button"
                onClick={commitReinforcement}
                disabled={!isMyTurn || actionMode !== "reinforce"}
              >
                Place Reinforcement
              </button>
              <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600">
                Territory: {selectedTerritory || "-"} | Max armies: {maxArmiesInput}
              </div>
            </div>

            <div className="grid grid-cols-1 gap-2 md:grid-cols-4">
              <button
                className={buttonPrimaryClass}
                type="button"
                onClick={commitAttack}
                disabled={!isMyTurn || actionMode !== "attack" || !canEnterAttack}
              >
                Attack
              </button>
              <button className={buttonGhostClass} type="button" onClick={() => sendAction({ action: "occupy", armies: armiesInput })} disabled={!isMyTurn || actionMode !== "occupy" || !canEnterAttack}>
                Occupy
              </button>
              <button className={buttonGhostClass} type="button" onClick={() => sendAction({ action: "end_attack" })} disabled={!isMyTurn || !canEnterAttack}>
                End Attack
              </button>
              <button className={buttonGhostClass} type="button" onClick={() => sendAction({ action: "end_turn" })} disabled={!isMyTurn || !canEnterAttack}>
                End Turn
              </button>
            </div>

            <div className="grid grid-cols-1 gap-2 md:grid-cols-4">
              <button
                className={buttonGhostClass}
                type="button"
                onClick={commitFortify}
                disabled={!isMyTurn || actionMode !== "fortify" || !canEnterAttack}
              >
                Fortify
              </button>
              <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600">From: {selectedFrom || "-"}</div>
              <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600">To: {selectedTo || "-"}</div>
              <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600">Turn: {isMyTurn ? "Your turn" : "Waiting"}</div>
            </div>
            {actionError ? <p className="text-sm text-rose-700">{actionError}</p> : null}
          </div>
        </section>
      </section>

      <aside className="grid gap-4">
        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Players</h3>
            <span className="text-xs text-slate-500">
              {players.length}/{players.length}
            </span>
          </div>
          {players.length === 0 ? <p className="text-sm text-slate-500">No player data available yet.</p> : null}
          <ul className="grid gap-2">
            {players.map((p) => (
              <li key={p.userId} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-xs text-slate-700">
                    <span
                      className="mr-1.5 inline-block h-2.5 w-2.5 rounded-full align-middle"
                      style={{ backgroundColor: p.color || "#94a3b8" }}
                    />
                    <span style={{ color: p.color || "#334155" }}>{p.userName || p.userId}</span>
                  </span>
                  {game && players[game.currentPlayer]?.userId === p.userId ? (
                    <span className="rounded-full border border-emerald-300 bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700">
                      Current Turn
                    </span>
                  ) : null}
                </div>
                <p className="mt-1 text-xs text-slate-600">Cards: {p.cardCount}</p>
                <p className="text-xs text-slate-600">Status: {p.eliminated ? "Eliminated" : "Active"}</p>
              </li>
            ))}
          </ul>
        </section>

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Dice Roller</h3>
          </div>
          <div className="grid gap-2">
            <div className="grid grid-cols-2 gap-2">
              <label className="grid gap-1 text-xs font-medium text-slate-600">
                Attacker Dice
                <select
                  className={inputClass}
                  value={attackerDice}
                  onChange={(e) => setAttackerDice(Number(e.target.value))}
                >
                  <option value={1}>1</option>
                  <option value={2}>2</option>
                  <option value={3}>3</option>
                </select>
              </label>
              <label className="grid gap-1 text-xs font-medium text-slate-600">
                Defender Dice
                <select
                  className={inputClass}
                  value={defenderDice}
                  onChange={(e) => setDefenderDice(Number(e.target.value))}
                >
                  <option value={1}>1</option>
                  <option value={2}>2</option>
                </select>
              </label>
            </div>
            <button className={buttonPrimaryClass} type="button" onClick={onRollDice}>
              Roll Dice
            </button>
          </div>

          {diceResult ? (
            <div className="mt-3 rounded-xl border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700">
              <p>
                Attacker: <span className="font-semibold">{diceResult.attacker.join(", ")}</span>
              </p>
              <p>
                Defender: <span className="font-semibold">{diceResult.defender.join(", ")}</span>
              </p>
              <p className="mt-1 text-xs text-slate-600">
                Losses: Attacker {diceResult.attackerLoss}, Defender {diceResult.defenderLoss}
              </p>
            </div>
          ) : null}
        </section>

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Game Chat</h3>
            <span className="text-xs text-slate-500">Socket: {wsStatus}</span>
          </div>
          <div ref={chatScrollRef} className="h-[260px] overflow-y-auto rounded-xl border border-slate-200 bg-slate-50 p-3 text-sm text-slate-600">
            {chatMessages.length === 0 ? <p>No messages yet.</p> : null}
            <ul className="grid gap-2">
              {chatMessages.map((m, idx) => {
                const nameKey = m.userName.trim().toLowerCase();
                const chatColor = chatColorByUserName[nameKey] ?? "#334155";
                return (
                <li key={`${m.userName}-${m.createdAt}-${idx}`} className="rounded-lg bg-white p-2">
                  <div className="mb-1 flex items-center justify-between gap-2">
                    <span className="font-medium" style={{ color: chatColor }}>
                      {m.userName}
                    </span>
                    <span className="text-[11px] text-slate-500">{new Date(m.createdAt).toLocaleTimeString()}</span>
                  </div>
                  <p className="whitespace-pre-wrap" style={{ color: chatColor }}>
                    {m.body}
                  </p>
                </li>
                );
              })}
            </ul>
          </div>
          <form className="mt-3 grid gap-2" onSubmit={onSendChat}>
            <textarea
              className={inputClass}
              rows={3}
              value={chatDraft}
              onChange={(e) => setChatDraft(e.target.value)}
              placeholder="Type a game message..."
            />
            {chatError ? <p className="text-sm text-rose-700">{chatError}</p> : null}
            <button className={buttonPrimaryClass} type="submit" disabled={chatDraft.trim() === "" || wsStatus !== "connected"}>
              Send
            </button>
          </form>
        </section>
      </aside>
    </div>
  );
}
