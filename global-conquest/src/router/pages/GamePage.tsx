import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getGameBootstrap, type GameBootstrap, type Card } from "../../api/games";
import { useAuth } from "../../auth";
import { GameMap } from "../../map/GameMap";
import { useSocket } from "../../realtime";
import { buttonGhostClass, buttonPrimaryClass, inputClass } from "./styles";
import {
  MAP_EDGES,
  MAP_PLAYER_COLORS,
  MAP_TERRITORIES,
  type DiceRollResult,
  type GameChatMessage,
  type GameEventMessage,
} from "./gameShared";

export function GamePage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const { gameID } = useParams({ from: "/app/game/$gameID" });
  const { on, send, status: wsStatus } = useSocket();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [game, setGame] = useState<GameBootstrap | null>(null);
  const [chatMessages, setChatMessages] = useState<GameChatMessage[]>([]);
  const [eventMessages, setEventMessages] = useState<GameEventMessage[]>([]);
  const [chatDraft, setChatDraft] = useState("");
  const [chatError, setChatError] = useState("");
  const [actionError, setActionError] = useState("");
  const [selectedTerritory, setSelectedTerritory] = useState("");
  const [selectedFrom, setSelectedFrom] = useState("");
  const [selectedTo, setSelectedTo] = useState("");
  const [armiesInput, setArmiesInput] = useState(1);
  const [attackerDice, setAttackerDice] = useState(3);
  const [diceResult, setDiceResult] = useState<DiceRollResult | null>(null);
  const [myCards, setMyCards] = useState<Card[]>([]);
  const [selectedCardIndices, setSelectedCardIndices] = useState<number[]>([]);
  const chatScrollRef = useRef<HTMLDivElement | null>(null);
  const eventScrollRef = useRef<HTMLDivElement | null>(null);

  const parseTerritories = (raw: unknown): Record<string, unknown> => {
    if (raw && typeof raw === "object") return raw as Record<string, unknown>;
    if (typeof raw === "string") {
      try {
        const parsed = JSON.parse(raw);
        if (parsed && typeof parsed === "object") return parsed as Record<string, unknown>;
      } catch {
        return {};
      }
    }
    return {};
  };

  const loadGame = useCallback(
    async (cancelled = false) => {
      setError("");
      try {
        const out = await getGameBootstrap(gameID);
        if (cancelled) return;
        setGame(out);
        setEventMessages(out.events ?? []);
        const me = out.players.find((p) => p.userId === auth.user?.id);
        if (me) setMyCards(me.cards ?? []);
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
      const action = typeof payload?.action === "string" ? payload.action : "";
      const phase = typeof payload?.phase === "string" ? payload.phase : "";
      const currentPlayer = typeof payload?.current_player === "number" ? payload.current_player : -1;
      const pendingReinforcements =
        typeof payload?.pending_reinforcements === "number" ? payload.pending_reinforcements : 0;
      const setsTraded = typeof payload?.sets_traded === "number" ? payload.sets_traded : undefined;
      const occupyRaw = payload?.occupy && typeof payload.occupy === "object" ? (payload.occupy as Record<string, unknown>) : null;
      const occupy =
        occupyRaw &&
        typeof occupyRaw.from === "string" &&
        typeof occupyRaw.to === "string" &&
        typeof occupyRaw.min_move === "number" &&
        typeof occupyRaw.max_move === "number"
          ? {
              from: occupyRaw.from,
              to: occupyRaw.to,
              minMove: occupyRaw.min_move,
              maxMove: occupyRaw.max_move,
            }
          : null;
      const territories = parseTerritories(payload?.territories);
      const incomingPlayersRaw = Array.isArray(payload?.players) ? payload.players : [];
      const incomingPlayers = incomingPlayersRaw
        .filter((v): v is Record<string, unknown> => !!v && typeof v === "object")
        .map((p) => ({
          userId: typeof p.user_id === "string" ? p.user_id : "",
          cardCount: typeof p.card_count === "number" ? p.card_count : 0,
          setupArmies: typeof p.setup_armies === "number" ? p.setup_armies : 0,
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
            cards: meta?.cards ?? [],
            setupArmies: p.setupArmies,
            eliminated: p.eliminated,
          };
        });
        return {
          ...prev,
          phase,
          currentPlayer,
          pendingReinforcements,
          setsTraded: setsTraded ?? prev.setsTraded,
          occupy,
          territories,
          players: nextPlayers,
        };
      });

      if (action === "attack" && payload?.result && typeof payload.result === "object") {
        const result = payload.result as Record<string, unknown>;
        const attacker = Array.isArray(result.attacker_rolls)
          ? result.attacker_rolls.filter((v): v is number => typeof v === "number")
          : [];
        const defender = Array.isArray(result.defender_rolls)
          ? result.defender_rolls.filter((v): v is number => typeof v === "number")
          : [];
        const attackerLoss = typeof result.attacker_loss === "number" ? result.attacker_loss : 0;
        const defenderLoss = typeof result.defender_loss === "number" ? result.defender_loss : 0;
        setDiceResult({ attacker, defender, attackerLoss, defenderLoss });
      }
      if (payload?.event && typeof payload.event === "object") {
        const event = payload.event as Record<string, unknown>;
        const nextEvent: GameEventMessage = {
          id: typeof event.id === "string" ? event.id : `${payloadGameID}-${Date.now()}`,
          gameID:
            typeof event.game_id === "string"
              ? event.game_id
              : typeof payloadGameID === "string"
                ? payloadGameID
                : gameID,
          actorUserID: typeof event.actor_user_id === "string" ? event.actor_user_id : "",
          eventType: typeof event.event_type === "string" ? event.event_type : "game_event",
          body: typeof event.body === "string" ? event.body : "",
          createdAt: typeof event.created_at === "string" ? event.created_at : new Date().toISOString(),
        };
        if (nextEvent.body.trim() !== "") {
          setEventMessages((prev) => {
            if (prev.some((ev) => ev.id !== "" && ev.id === nextEvent.id)) return prev;
            return [...prev, nextEvent];
          });
        }
      }
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
    const off = on("your_cards", (msg) => {
      const payload = msg.payload as Record<string, unknown> | undefined;
      const cardsRaw = Array.isArray(payload?.cards) ? payload.cards : [];
      const cards: Card[] = cardsRaw
        .filter((c): c is Record<string, unknown> => !!c && typeof c === "object")
        .map((c) => ({
          territory: typeof c.territory === "string" ? c.territory : "",
          symbol: typeof c.symbol === "string" ? c.symbol : "",
        }));
      setMyCards(cards);
      setSelectedCardIndices([]);
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
  useEffect(() => {
    const el = eventScrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [eventMessages]);

  const players = useMemo(() => game?.players ?? [], [game?.players]);
  const phase = game?.phase ?? "";
  const setsTraded = game?.setsTraded ?? 0;
  const nextTradeBonus = useMemo(() => {
    const n = setsTraded + 1;
    if (n <= 5) return 2 * n + 2;
    if (n === 6) return 15;
    return 15 + (n - 6) * 5;
  }, [setsTraded]);
  const territoryState = game?.territories ?? null;
  const pendingReinforcements = game?.pendingReinforcements ?? 0;
  const occupyRequirement = game?.occupy ?? null;
  const meIndex = useMemo(() => players.findIndex((p) => p.userId === auth.user?.id), [players, auth.user?.id]);
  const isMyTurn = meIndex >= 0 && game?.currentPlayer === meIndex;
  const canEnterAttack = pendingReinforcements === 0;
  const phaseMode = phase === "attack" || phase === "fortify" || phase === "occupy" || phase === "reinforce" || phase === "setup_reinforce" ? phase : "reinforce";
  const mySetupArmies = useMemo(() => players[meIndex]?.setupArmies ?? 0, [players, meIndex]);
  const activeFrom = phaseMode === "occupy" && occupyRequirement ? occupyRequirement.from : selectedFrom;
  const activeTo = phaseMode === "occupy" && occupyRequirement ? occupyRequirement.to : selectedTo;
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
  const eventColorByActorID = useMemo(() => {
    const out: Record<string, string> = {};
    players.forEach((p, i) => {
      if (!p.userId) return;
      out[p.userId] = p.color || MAP_PLAYER_COLORS[i % MAP_PLAYER_COLORS.length];
    });
    return out;
  }, [players]);
  const playerNamesByKey = useMemo(() => {
    const out: Record<string, string> = {};
    players.forEach((p) => {
      const userName = p.userName.trim();
      const userID = p.userId.trim();
      if (userName) out[userName.toLowerCase()] = userName;
      if (userID) out[userID.toLowerCase()] = userID;
    });
    return out;
  }, [players]);
  const territoryNameSet = useMemo(() => new Set(Object.keys(MAP_TERRITORIES)), []);
  const eventHighlightRegex = useMemo(() => {
    const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const terms = [
      ...Object.values(playerNamesByKey),
      ...Array.from(territoryNameSet),
    ]
      .filter((value) => value.trim() !== "")
      .sort((a, b) => b.length - a.length);
    if (terms.length === 0) return null;
    return new RegExp(`(${terms.map((term) => escapeRegExp(term)).join("|")})`, "g");
  }, [playerNamesByKey, territoryNameSet]);

  const renderEventBody = (body: string) => {
    if (!eventHighlightRegex) return body;
    const parts = body.split(eventHighlightRegex);
    return parts.map((part, idx) => {
      if (!part) return null;
      const playerKey = part.toLowerCase();
      if (playerNamesByKey[playerKey]) {
        return (
          <span key={`${part}-${idx}`} className="font-extrabold">
            {part}
          </span>
        );
      }
      if (territoryNameSet.has(part)) {
        return (
          <span key={`${part}-${idx}`} className="font-extrabold">
            {part}
          </span>
        );
      }
      return <span key={`${part}-${idx}`}>{part}</span>;
    });
  };

  const selectedFromArmies = useMemo(() => {
    if (!activeFrom) return 0;
    const tRaw = territoryState?.[activeFrom];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    return typeof t?.armies === "number" ? t.armies : 0;
  }, [activeFrom, territoryState]);
  const selectedToArmies = useMemo(() => {
    if (!activeTo) return 0;
    const tRaw = territoryState?.[activeTo];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    return typeof t?.armies === "number" ? t.armies : 0;
  }, [activeTo, territoryState]);
  const selectedFromOwner = useMemo(() => {
    if (!selectedFrom) return -1;
    const tRaw = territoryState?.[selectedFrom];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    return typeof t?.owner === "number" ? t.owner : -1;
  }, [selectedFrom, territoryState]);
  const selectedToOwner = useMemo(() => {
    if (!selectedTo) return -1;
    const tRaw = territoryState?.[selectedTo];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    return typeof t?.owner === "number" ? t.owner : -1;
  }, [selectedTo, territoryState]);
  const areAdjacent = useMemo(
    () => MAP_EDGES.some(([a, b]) => (a === selectedFrom && b === selectedTo) || (a === selectedTo && b === selectedFrom)),
    [selectedFrom, selectedTo]
  );
  const maxAttackDiceAllowed = useMemo(() => Math.max(1, Math.min(3, selectedFromArmies - 1)), [selectedFromArmies]);
  const maxDefendDiceAllowed = useMemo(() => Math.max(1, Math.min(2, selectedToArmies)), [selectedToArmies]);
  const clampedAttackerDice = Math.max(1, Math.min(attackerDice, maxAttackDiceAllowed));
  const canAttackSelection = useMemo(() => {
    if (!selectedFrom || !selectedTo) return false;
    if (!areAdjacent) return false;
    if (selectedFromOwner !== meIndex) return false;
    if (selectedToOwner < 0 || selectedToOwner === meIndex) return false;
    if (selectedFromArmies <= 1) return false;
    if (selectedToArmies <= 0) return false;
    return true;
  }, [areAdjacent, meIndex, selectedFrom, selectedFromArmies, selectedFromOwner, selectedTo, selectedToArmies, selectedToOwner]);

  const maxArmiesInput = useMemo(() => {
    if (phaseMode === "reinforce") {
      return Math.max(1, game?.pendingReinforcements ?? 1);
    }
    if (phaseMode === "occupy" && occupyRequirement) {
      return Math.max(occupyRequirement.minMove, occupyRequirement.maxMove);
    }
    if (phaseMode === "fortify") {
      return Math.max(1, selectedFromArmies - 1);
    }
    return 50;
  }, [phaseMode, game?.pendingReinforcements, selectedFromArmies, occupyRequirement]);

  const minArmiesInput = useMemo(() => {
    if (phaseMode === "occupy" && occupyRequirement) {
      return Math.max(1, occupyRequirement.minMove);
    }
    return 1;
  }, [phaseMode, occupyRequirement]);
  const clampedArmiesInput = Math.max(minArmiesInput, Math.min(armiesInput, maxArmiesInput));

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
    if (phaseMode !== "attack") {
      setActionError("You can only roll during the attack phase.");
      return;
    }
    if (!isMyTurn) {
      setActionError("It's not your turn.");
      return;
    }
    if (!selectedFrom || !selectedTo) {
      setActionError("Select attacking and defending territories first.");
      return;
    }
    if (!canAttackSelection) {
      setActionError("Selected territories do not support a legal attack.");
      return;
    }
    sendAction({
      action: "attack",
      from: selectedFrom,
      to: selectedTo,
      attacker_dice: clampedAttackerDice,
      defender_dice: maxDefendDiceAllowed,
    });
  };

  const sendAction = (payload: Record<string, unknown>) => {
    setActionError("");
    if (wsStatus !== "connected") {
      setActionError("Socket disconnected.");
      return;
    }
    send("game_action", payload, { game_id: gameID });
  };

  const onMapTerritoryClick = (name: string) => {
    setActionError("");
    const tRaw = territoryState?.[name];
    const t = tRaw && typeof tRaw === "object" ? (tRaw as Record<string, unknown>) : null;
    const owner = typeof t?.owner === "number" ? t.owner : -1;
    const isMine = owner >= 0 && owner === meIndex;
    const isEnemy = owner >= 0 && owner !== meIndex;

    if (phaseMode === "setup_reinforce") {
      if (!isMine) {
        setActionError("You can only place armies on your own territories.");
        return;
      }
      if (mySetupArmies <= 0) {
        setActionError("You have no armies left to place.");
        return;
      }
      setSelectedTerritory(name);
      sendAction({ action: "place_initial_army", territory: name });
      return;
    }
    if (phaseMode === "reinforce") {
      if (!isMine) {
        setActionError("Choose one of your territories for reinforcement.");
        return;
      }
      setSelectedTerritory(name);
      return;
    }
    if (phaseMode === "occupy") {
      if (occupyRequirement) {
        setSelectedFrom(occupyRequirement.from);
        setSelectedTo(occupyRequirement.to);
      }
      return;
    }
    if (phaseMode === "attack") {
      if (!canEnterAttack) {
        setActionError("Place all reinforcements before attacking.");
        return;
      }
      if (isMine) {
        // Clear stale reinforce highlight whenever a new attacker is picked.
        setSelectedTerritory("");
        setSelectedFrom(name);
        setSelectedTo("");
        return;
      }
      if (!isEnemy) {
        setActionError("Select an enemy territory as attack target.");
        return;
      }
      if (!selectedFrom) {
        setActionError("Select your attacking territory first.");
        return;
      }
      setSelectedTo(name);
      return;
    }
    if (phaseMode === "fortify") {
      // Clear stale reinforce highlight on any fortify interaction.
      setSelectedTerritory("");
      if (!isMine) {
        setActionError("Select one of your territories.");
        return;
      }
      // If no source yet, OR both source+dest already chosen → start fresh.
      // This lets the player change their source at any point.
      if (!selectedFrom || selectedTo) {
        setSelectedFrom(name);
        setSelectedTo("");
        return;
      }
      // Source set, destination not yet — must be adjacent.
      const adjacent = MAP_EDGES.some(([a, b]) => (a === selectedFrom && b === name) || (b === selectedFrom && a === name));
      if (!adjacent) {
        setActionError(`${name} is not adjacent to ${selectedFrom}. Armies can only move to a neighboring territory.`);
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
    sendAction({ action: "place_reinforcement", territory: selectedTerritory, armies: clampedArmiesInput });
  };

  const commitFortify = () => {
    if (!selectedFrom || !selectedTo) {
      setActionError("Select source and destination territories on the map.");
      return;
    }
    sendAction({ action: "fortify", from: selectedFrom, to: selectedTo, armies: clampedArmiesInput });
  };

  const commitOccupy = () => {
    if (phaseMode !== "occupy" || !occupyRequirement) {
      setActionError("No troop movement is currently required.");
      return;
    }
    if (clampedArmiesInput < occupyRequirement.minMove || clampedArmiesInput > occupyRequirement.maxMove) {
      setActionError(`Move must be between ${occupyRequirement.minMove} and ${occupyRequirement.maxMove} armies.`);
      return;
    }
    sendAction({ action: "occupy", armies: clampedArmiesInput });
  };

  const toggleCardSelection = (idx: number) => {
    setSelectedCardIndices((prev) => {
      if (prev.includes(idx)) return prev.filter((i) => i !== idx);
      if (prev.length >= 3) return prev;
      return [...prev, idx];
    });
  };

  const commitTradeCards = () => {
    if (selectedCardIndices.length !== 3) {
      setActionError("Select exactly 3 cards to trade.");
      return;
    }
    const [i0, i1, i2] = selectedCardIndices;
    sendAction({ action: "trade_cards", card_indices: [i0, i1, i2] });
    setSelectedCardIndices([]);
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
          {phaseMode === "setup_reinforce" ? (
            <div className="mb-3 rounded-xl border border-indigo-200 bg-indigo-50 p-3 text-xs text-indigo-800">
              <p className="font-semibold">Initial Troop Placement</p>
              <p className="mt-0.5">
                {mySetupArmies > 0
                  ? `Click one of your territories to place an army. You have ${mySetupArmies} left.`
                  : "You've placed all your armies. Waiting for other players to finish."}
              </p>
            </div>
          ) : null}
          <div className="mb-3 grid gap-2 rounded-xl border border-slate-200 bg-slate-50 p-3 text-xs text-slate-700 md:grid-cols-[1fr_auto] md:items-center">
            <div>
              <p>
                Phase: <span className="font-semibold">{phase || "-"}</span> | Armies To Place:{" "}
                <span className="font-semibold">{pendingReinforcements}</span>
              </p>
              <p>
                Selected: territory <span className="font-semibold">{selectedTerritory || "-"}</span>, from{" "}
                <span className="font-semibold">{activeFrom || "-"}</span>, to{" "}
                <span className="font-semibold">{activeTo || "-"}</span>
              </p>
              {phaseMode === "occupy" && occupyRequirement ? (
                <p className="text-amber-700">
                  Move required: {occupyRequirement.from} {"->"} {occupyRequirement.to} ({occupyRequirement.minMove}-{occupyRequirement.maxMove})
                </p>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {phaseMode !== "attack" && phaseMode !== "setup_reinforce" ? (
                <input
                  className={`${inputClass} w-24`}
                  type="number"
                  min={minArmiesInput}
                  max={maxArmiesInput}
                  value={clampedArmiesInput}
                  onChange={(e) => {
                    const n = Number(e.target.value) || 1;
                    setArmiesInput(Math.max(minArmiesInput, Math.min(n, maxArmiesInput)));
                  }}
                />
              ) : null}
              {phaseMode === "reinforce" ? (
                <button className={buttonPrimaryClass} type="button" onClick={commitReinforcement} disabled={!isMyTurn}>
                  Place
                </button>
              ) : null}
              {phaseMode === "attack" ? (
                <>
                  <button className={buttonGhostClass} type="button" onClick={() => { sendAction({ action: "end_attack" }); setSelectedFrom(""); setSelectedTo(""); setSelectedTerritory(""); }} disabled={!isMyTurn}>
                    End Attack
                  </button>
                </>
              ) : null}
              {phaseMode === "occupy" ? (
                <button className={buttonPrimaryClass} type="button" onClick={commitOccupy} disabled={!isMyTurn}>
                  Move Troops
                </button>
              ) : null}
              {phaseMode === "fortify" ? (
                <>
                  <button className={buttonGhostClass} type="button" onClick={commitFortify} disabled={!isMyTurn}>
                    Fortify
                  </button>
                  <button className={buttonPrimaryClass} type="button" onClick={() => { sendAction({ action: "end_turn" }); setSelectedFrom(""); setSelectedTo(""); setSelectedTerritory(""); }} disabled={!isMyTurn}>
                    End Turn
                  </button>
                </>
              ) : null}
            </div>
          </div>
          <GameMap
            game={game}
            selectedTerritory={selectedTerritory}
            activeFrom={activeFrom}
            activeTo={activeTo}
            playerColors={playerColors}
            onTerritoryClick={onMapTerritoryClick}
          />
        </section>

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Event Log</h3>
            <span className="text-xs text-slate-500">Phase: {phase || "-"}</span>
          </div>
          <div ref={eventScrollRef} className="h-[180px] overflow-y-auto rounded-xl border border-slate-200 bg-slate-50 p-3 text-sm text-slate-600">
            {eventMessages.length === 0 ? <p>No events yet.</p> : null}
            <ul className="grid gap-2">
              {eventMessages.map((ev, idx) => {
                const eventColor = eventColorByActorID[ev.actorUserID] ?? "#334155";
                return (
                <li key={`${ev.id}-${idx}`} className="rounded-lg bg-white px-2 py-1.5">
                  <div className="mb-1 flex items-center justify-between gap-2 text-xs" style={{ color: eventColor }}>
                    <span className="font-semibold">{ev.eventType.replaceAll("_", " ")}</span>
                    <span>{new Date(ev.createdAt).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}</span>
                  </div>
                  <p className="text-sm font-medium" style={{ color: eventColor }}>
                    {renderEventBody(ev.body)}
                  </p>
                </li>
                );
              })}
            </ul>
          </div>
        </section>

        {actionError ? <p className="text-sm text-rose-700">{actionError}</p> : null}
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
                      <span className="font-bold" style={{ color: p.color || "#334155" }}>
                        {p.userName || p.userId}
                      </span>
                    </span>
                  {game && players[game.currentPlayer]?.userId === p.userId ? (
                    <span className="rounded-full border border-emerald-300 bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700">
                      Current Turn
                    </span>
                  ) : null}
                </div>
                {phaseMode === "setup_reinforce" ? (
                  <p className="mt-1 text-xs text-slate-600">Armies to place: <span className="font-semibold">{p.setupArmies}</span></p>
                ) : (
                  <p className="mt-1 text-xs text-slate-600">Cards: {p.cardCount}</p>
                )}
                <p className="text-xs text-slate-600">Status: {p.eliminated ? "Eliminated" : "Active"}</p>
              </li>
            ))}
          </ul>
        </section>

        {meIndex >= 0 && game?.status === "in_progress" ? (
          <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <div className="mb-3 flex items-center justify-between gap-2">
              <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">My Cards</h3>
              <span className="text-xs text-slate-500">
                {myCards.length} card{myCards.length !== 1 ? "s" : ""} · next trade: <span className="font-semibold text-indigo-700">+{nextTradeBonus}</span>
              </span>
            </div>
            {myCards.length === 0 ? (
              <p className="text-xs text-slate-500">No cards yet. Earn one by conquering a territory.</p>
            ) : (
              <ul className="grid gap-1.5">
                {myCards.map((card, idx) => {
                  const isSelected = selectedCardIndices.includes(idx);
                  const symbolIcon = card.symbol === "infantry" ? "🪖" : card.symbol === "cavalry" ? "🐴" : card.symbol === "artillery" ? "💣" : "⭐";
                  return (
                    <li key={idx}>
                      <button
                        type="button"
                        onClick={() => toggleCardSelection(idx)}
                        className={`w-full rounded-lg border px-3 py-2 text-left text-xs transition-colors ${
                          isSelected
                            ? "border-indigo-400 bg-indigo-50 text-indigo-800"
                            : "border-slate-200 bg-slate-50 text-slate-700 hover:border-slate-300 hover:bg-slate-100"
                        }`}
                      >
                        <span className="mr-1.5">{symbolIcon}</span>
                        <span className="font-semibold capitalize">{card.symbol}</span>
                        {card.territory ? (
                          <span className="ml-1.5 text-slate-500">— {card.territory}</span>
                        ) : null}
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
            {myCards.length >= 5 && isMyTurn && phaseMode === "reinforce" ? (
              <p className="mt-2 rounded-lg border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs font-medium text-amber-800">
                You must trade cards before placing reinforcements (5+ cards held).
              </p>
            ) : null}
            {isMyTurn && phaseMode === "reinforce" && myCards.length >= 3 ? (
              <div className="mt-3 grid gap-2">
                <p className="text-xs text-slate-500">
                  {selectedCardIndices.length}/3 selected
                </p>
                <button
                  type="button"
                  className={buttonPrimaryClass}
                  onClick={commitTradeCards}
                  disabled={selectedCardIndices.length !== 3}
                >
                  Trade Selected Cards
                </button>
              </div>
            ) : null}
          </section>
        ) : null}

        <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-slate-700">Dice Roller</h3>
          </div>
          <div className="grid gap-2">
            <p className="text-xs text-slate-600">
              Attack: <span className="font-semibold">{selectedFrom || "-"}</span> {"->"}{" "}
              <span className="font-semibold">{selectedTo || "-"}</span>
            </p>
            <div className="grid grid-cols-2 gap-2">
              <label className="grid gap-1 text-xs font-medium text-slate-600">
                Attacker Dice
                <select
                  className={inputClass}
                  value={clampedAttackerDice}
                  onChange={(e) => setAttackerDice(Math.max(1, Number(e.target.value) || 1))}
                >
                  <option value={1}>1</option>
                  {maxAttackDiceAllowed >= 2 ? <option value={2}>2</option> : null}
                  {maxAttackDiceAllowed >= 3 ? <option value={3}>3</option> : null}
                </select>
              </label>
              <div className="grid gap-1 text-xs font-medium text-slate-600">
                Defender Dice
                <div className={`${inputClass} bg-slate-100`}>{maxDefendDiceAllowed}</div>
              </div>
            </div>
            <button
              className={buttonPrimaryClass}
              type="button"
              onClick={onRollDice}
              disabled={!isMyTurn || phaseMode !== "attack" || !canAttackSelection}
            >
              Roll Dice
            </button>
            {!canAttackSelection && phaseMode === "attack" ? (
              <p className="text-xs text-slate-500">Select adjacent attacker/defender territories to roll.</p>
            ) : null}
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
                    <span className="text-[11px] text-slate-500">{new Date(m.createdAt).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}</span>
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
