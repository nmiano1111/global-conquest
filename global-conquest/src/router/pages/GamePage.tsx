import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getGameBootstrap, type GameBootstrap, type Card } from "../../api/games";
import { useAuth } from "../../auth";
import { GameMap, type GameMapHandle } from "../../map/GameMap";
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
import { MobileGameView } from "./MobileGameView";
import { FullscreenGameMap } from "./FullscreenGameMap";
import { reconcileSelection } from "./selectionReconciliation";

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
  // Passive highlight for whatever the most recently committed action
  // touched (bot or human) — separate from selectedTerritory/selectedFrom/
  // selectedTo above, which also drive form/attack-panel logic. This is
  // purely visual and gets superseded by the next action, or cleared the
  // moment the local user clicks the map themselves.
  const [lastActionTerritory, setLastActionTerritory] = useState("");
  const [lastActionFrom, setLastActionFrom] = useState("");
  const [lastActionTo, setLastActionTo] = useState("");
  // Which action type produced the lastAction* territories above — used to
  // scope the "recent combat" map highlight to attacks specifically (a
  // fortify or reinforcement shouldn't get the combat glow).
  const [lastActionType, setLastActionType] = useState("");
  const [mapFullscreenOpen, setMapFullscreenOpen] = useState(false);
  // A single GameMap (and its underlying Pixi Application) is shared
  // between the embedded and fullscreen views — reparented imperatively
  // between whichever of two slot elements is currently active, rather
  // than being created/destroyed on every fullscreen toggle. This is
  // required, not just an optimization: PixiJS's canvas-text texture pool
  // is a module-level singleton (TexturePool), and destroying any one
  // Application's renderer wipes it for every other still-live
  // Application on the page — so a second, independently created/
  // destroyed Application (the old approach) reliably crashes the first
  // one's eventual cleanup with "Cannot read properties of undefined
  // (reading 'push')" inside Pixi's CanvasTextPipe.destroy(). Sharing one
  // instance also means the camera never resets across the toggle, not
  // just across game-state updates.
  //
  // GameMap is portaled into mapHolderEl, a plain DOM node created exactly
  // once (never swapped) — React's portal reconciliation keys off that
  // container's identity, and a *changing* container (e.g. portaling
  // straight into whichever slot is "active") makes React unmount and
  // remount the portaled tree even though the target was never literally
  // null in between. Keeping the portal target permanently stable and
  // instead moving *that* node between slots with plain DOM appendChild
  // (which React's reconciler never inspects) is what actually avoids the
  // remount.
  const mapRef = useRef<GameMapHandle>(null);
  const [mapHolderEl] = useState(() => {
    const el = document.createElement("div");
    el.className = "contents"; // generates no box of its own; GameMap's own absolute-inset-0 div sizes against whichever slot this is currently appended to
    return el;
  });
  const [embeddedMapSlotEl, setEmbeddedMapSlotEl] = useState<HTMLDivElement | null>(null);
  const [fullscreenMapSlotEl, setFullscreenMapSlotEl] = useState<HTMLDivElement | null>(null);
  const activeMapSlotEl = mapFullscreenOpen && fullscreenMapSlotEl ? fullscreenMapSlotEl : embeddedMapSlotEl;

  // Moves the (stable, never-recreated) map holder into whichever slot is
  // currently active. useLayoutEffect (not useEffect) so this happens
  // before the browser paints, avoiding a flash in the previous slot.
  useLayoutEffect(() => {
    if (activeMapSlotEl && mapHolderEl.parentElement !== activeMapSlotEl) {
      activeMapSlotEl.appendChild(mapHolderEl);
    }
  }, [activeMapSlotEl, mapHolderEl]);

  // On entering fullscreen, explicitly fill the (now larger) fullscreen
  // slot edge-to-edge rather than waiting on the map's own ResizeObserver,
  // which only clamps the existing camera (correct for an ordinary
  // resize) rather than re-fitting it — fullscreen specifically wants an
  // auto-fill zoom on open, not just "whatever the embedded view's camera
  // happened to be, now shown bigger." A frame's delay lets the fullscreen
  // shell's layout (100dvh, safe-area insets) settle first.
  useEffect(() => {
    if (!mapFullscreenOpen || !fullscreenMapSlotEl) return;
    const raf = requestAnimationFrame(() => {
      const { offsetWidth, offsetHeight } = fullscreenMapSlotEl;
      if (offsetWidth && offsetHeight) {
        mapRef.current?.enterFullscreenFit(offsetWidth, offsetHeight);
      }
    });
    return () => cancelAnimationFrame(raf);
  }, [mapFullscreenOpen, fullscreenMapSlotEl]);
  // Other connected players' live territory presses, relayed over the
  // socket (see territory_select/territory_selected) — keyed by user id so
  // multiple players' selections can be shown at once. Purely a passive
  // display signal: never persisted, never authoritative.
  type RemoteSelection = { territory: string; from: string; to: string };
  const [remoteSelections, setRemoteSelections] = useState<Record<string, RemoteSelection>>({});
  const [armiesInput, setArmiesInput] = useState(1);
  const [attackerDice, setAttackerDice] = useState(3);
  const [diceResult, setDiceResult] = useState<DiceRollResult | null>(null);
  const [myCards, setMyCards] = useState<Card[]>([]);
  const [selectedCardIndices, setSelectedCardIndices] = useState<number[]>([]);
  const chatScrollRef = useRef<HTMLDivElement | null>(null);
  const eventScrollRef = useRef<HTMLDivElement | null>(null);
  const [mobileUI, setMobileUI] = useState<boolean>(() => {
    const stored = localStorage.getItem("gc.mobile.ui");
    if (stored !== null) return stored === "true";
    return typeof window !== "undefined" && window.innerWidth < 768;
  });
  const toggleMobileUI = () => {
    setMobileUI((prev) => {
      localStorage.setItem("gc.mobile.ui", String(!prev));
      return !prev;
    });
  };

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

      if (occupy) {
        setArmiesInput(occupy.maxMove);
      }

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
            // isBot never changes after bootstrap and every broadcast only
            // carries gameplay fields, so it's simply carried over here.
            isBot: meta?.isBot ?? false,
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
      // Reflect whichever territories the just-committed action touched
      // (bot or human) as a passive highlight. Always set (defaulting to
      // "" when absent, e.g. end_turn/trade_cards) so a stale highlight
      // from an earlier action doesn't linger past an action with nothing
      // to show.
      setLastActionTerritory(typeof payload?.action_territory === "string" ? payload.action_territory : "");
      setLastActionFrom(typeof payload?.action_from === "string" ? payload.action_from : "");
      setLastActionTo(typeof payload?.action_to === "string" ? payload.action_to : "");
      setLastActionType(action);
    });
    return off;
  }, [gameID, on]);

  useEffect(() => {
    const off = on("error", (msg) => {
      const payload = msg.payload as Record<string, unknown> | undefined;
      const code = typeof payload?.code === "string" ? payload.code : "";
      const message = typeof payload?.message === "string" ? payload.message : "";
      if (code === "unauthorized") {
        // The socket's session token was rejected server-side (expired/revoked).
        // Unlike REST calls, WebSocket actions never hit a 401 that would
        // otherwise trigger this same redirect, so do it explicitly here.
        auth.clearSession();
        void navigate({ to: "/login" });
        return;
      }
      if (code === "invalid_action" || code === "not_in_room") {
        setActionError(message || "Action failed.");
      }
    });
    return off;
  }, [on, auth, navigate]);

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
    const off = on("territory_selected", (msg) => {
      const payload = msg.payload as Record<string, unknown> | undefined;
      const userID = typeof payload?.user_id === "string" ? payload.user_id : "";
      if (!userID || userID === auth.user?.id) return; // ignore our own echo
      setRemoteSelections((prev) => ({
        ...prev,
        [userID]: {
          territory: typeof payload?.territory === "string" ? payload.territory : "",
          from: typeof payload?.from === "string" ? payload.from : "",
          to: typeof payload?.to === "string" ? payload.to : "",
        },
      }));
    });
    return off;
  }, [on, auth.user?.id]);

  // Broadcast the local user's own selection to everyone else in the game
  // whenever it changes, including clearing it (an all-empty send is how
  // others learn we deselected). This is a pure relay — the server never
  // interprets it, it just fans it back out to the room.
  useEffect(() => {
    if (wsStatus !== "connected") return;
    send(
      "territory_select",
      { territory: selectedTerritory, from: selectedFrom, to: selectedTo },
      { game_id: gameID }
    );
  }, [selectedTerritory, selectedFrom, selectedTo, wsStatus, send, gameID]);

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

  useLayoutEffect(() => {
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
  const isGameOver = phase === "game_over";
  const mySetupArmies = useMemo(() => players[meIndex]?.setupArmies ?? 0, [players, meIndex]);
  const activeFrom = phaseMode === "occupy" && occupyRequirement ? occupyRequirement.from : selectedFrom;
  const activeTo = phaseMode === "occupy" && occupyRequirement ? occupyRequirement.to : selectedTo;
  // Everything to passively highlight that isn't already covered by the
  // local user's own selection: the last committed action's territories
  // (bot or human) plus every other connected player's live press.
  const highlightedTerritories = useMemo(() => {
    const set = new Set<string>();
    if (lastActionTerritory) set.add(lastActionTerritory);
    if (lastActionFrom) set.add(lastActionFrom);
    if (lastActionTo) set.add(lastActionTo);
    for (const sel of Object.values(remoteSelections)) {
      if (sel.territory) set.add(sel.territory);
      if (sel.from) set.add(sel.from);
      if (sel.to) set.add(sel.to);
    }
    return set;
  }, [lastActionTerritory, lastActionFrom, lastActionTo, remoteSelections]);
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
          <span key={`${part}-${idx}`} className="font-extrabold" style={{ color: chatColorByUserName[playerKey] }}>
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

  // Visual-only affordance: enemy territories directly adjacent to the
  // selected attacker, computed purely from already-public data (map
  // adjacency + territory ownership/armies) — the same predicate already
  // used above for canAttackSelection, just applied one territory at a
  // time. This never gates the actual attack command; the backend still
  // authoritatively validates it on submit.
  const legalAttackTargets = useMemo(() => {
    const set = new Set<string>();
    if (phaseMode !== "attack" || !isMyTurn || !selectedFrom || selectedFromArmies <= 1) return set;
    for (const [a, b] of MAP_EDGES) {
      let neighbor: string | null = null;
      if (a === selectedFrom) neighbor = b;
      else if (b === selectedFrom) neighbor = a;
      if (!neighbor) continue;
      const raw = territoryState?.[neighbor];
      const t = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : null;
      const owner = typeof t?.owner === "number" ? t.owner : -1;
      if (owner >= 0 && owner !== meIndex) set.add(neighbor);
    }
    return set;
  }, [phaseMode, isMyTurn, selectedFrom, selectedFromArmies, territoryState, meIndex]);

  // Territories the most recently resolved attack touched — a transient,
  // purely visual "something just happened here" cue superseded by the
  // next action.
  const recentCombatTerritories = useMemo(() => {
    const set = new Set<string>();
    if (lastActionType !== "attack") return set;
    if (lastActionFrom) set.add(lastActionFrom);
    if (lastActionTo) set.add(lastActionTo);
    return set;
  }, [lastActionType, lastActionFrom, lastActionTo]);

  // The territory just conquered and awaiting an occupation move — derived
  // directly from the authoritative occupy requirement, never guessed at
  // by diffing ownership client-side.
  const recentCaptureTerritories = useMemo(() => {
    const set = new Set<string>();
    if (phaseMode === "occupy" && occupyRequirement) set.add(occupyRequirement.to);
    return set;
  }, [phaseMode, occupyRequirement]);

  // Selection reconciliation: authoritative state can change after every
  // command (ours, another player's, or a bot's). Local selection must
  // never point at a territory role it no longer legally occupies — see
  // reconcileSelection for the exact rules. Camera position is untouched
  // by any of this; it lives entirely inside MapScene.
  //
  // Applied during render (React's documented pattern for "adjust state
  // when a prop changes") rather than in an effect, guarded by state (not
  // a ref — ref reads/writes aren't safe during render) so it only runs
  // once per distinct `game` object — this avoids an extra render→effect→
  // re-render cascade for something that fires on every authoritative update.
  const [previousCurrentPlayer, setPreviousCurrentPlayer] = useState<number | undefined>(undefined);
  const [lastReconciledGame, setLastReconciledGame] = useState<GameBootstrap | null>(null);
  if (game && lastReconciledGame !== game) {
    setLastReconciledGame(game);
    setPreviousCurrentPlayer(game.currentPlayer);
    const reconciled = reconcileSelection({
      currentPlayer: game.currentPlayer,
      previousCurrentPlayer,
      territoryState,
      meIndex,
      phaseMode,
      selectedTerritory,
      selectedFrom,
      selectedTo,
    });
    if (reconciled.selectedTerritory !== selectedTerritory) setSelectedTerritory(reconciled.selectedTerritory);
    if (reconciled.selectedFrom !== selectedFrom) setSelectedFrom(reconciled.selectedFrom);
    if (reconciled.selectedTo !== selectedTo) setSelectedTo(reconciled.selectedTo);
  }

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
    // A fresh click always takes over the display — don't leave a stale
    // bot/action highlight competing with the user's own live selection.
    setLastActionTerritory("");
    setLastActionFrom("");
    setLastActionTo("");
    setLastActionType("");
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
      if (!isMyTurn) return;
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
      if (!isMyTurn) return;
      if (!canEnterAttack) {
        setActionError("Place all reinforcements before attacking.");
        return;
      }
      if (isMine) {
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
      if (!isMyTurn) return;
      setSelectedTerritory("");
      if (!isMine) {
        setActionError("Select one of your territories.");
        return;
      }
      if (!selectedFrom || selectedTo) {
        setSelectedFrom(name);
        setSelectedTo("");
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

  // The one shared GameMap, portaled into the permanently-stable
  // mapHolderEl (see its declaration above for why the target must never
  // change) and always rendered — the useLayoutEffect above is what
  // actually moves it between the embedded and fullscreen slots.
  const mapPortal = createPortal(
    <GameMap
      ref={mapRef}
      game={game}
      selectedTerritory={selectedTerritory}
      activeFrom={activeFrom}
      activeTo={activeTo}
      highlightedTerritories={highlightedTerritories}
      legalTargets={legalAttackTargets}
      recentCombat={recentCombatTerritories}
      recentCapture={recentCaptureTerritories}
      playerColors={playerColors}
      onTerritoryClick={onMapTerritoryClick}
      className="absolute inset-0"
    />,
    mapHolderEl,
  );

  const fullscreenMap = mapFullscreenOpen ? (
    <FullscreenGameMap
      game={game}
      onClose={() => setMapFullscreenOpen(false)}
      mapRef={mapRef}
      mapSlotRef={setFullscreenMapSlotEl}
      phase={phase}
      phaseMode={phaseMode}
      isMyTurn={isMyTurn}
      isGameOver={isGameOver}
      wsStatus={wsStatus}
      players={players}
      playerColors={playerColors}
      territoryState={territoryState}
      pendingReinforcements={pendingReinforcements}
      mySetupArmies={mySetupArmies}
      occupyRequirement={occupyRequirement}
      diceResult={diceResult}
      selectedTerritory={selectedTerritory}
      selectedFrom={selectedFrom}
      selectedTo={selectedTo}
      activeFrom={activeFrom}
      activeTo={activeTo}
      clampedArmiesInput={clampedArmiesInput}
      minArmiesInput={minArmiesInput}
      maxArmiesInput={maxArmiesInput}
      clampedAttackerDice={clampedAttackerDice}
      maxAttackDiceAllowed={maxAttackDiceAllowed}
      maxDefendDiceAllowed={maxDefendDiceAllowed}
      canAttackSelection={canAttackSelection}
      commitReinforcement={commitReinforcement}
      commitFortify={commitFortify}
      commitOccupy={commitOccupy}
      onRollDice={onRollDice}
      setArmiesInput={setArmiesInput}
      setAttackerDice={setAttackerDice}
      sendAction={sendAction}
      setSelectedFrom={setSelectedFrom}
      setSelectedTo={setSelectedTo}
      setSelectedTerritory={setSelectedTerritory}
    />
  ) : null;

  if (mobileUI) {
    return (
      <>
      <MobileGameView
        game={game}
        loading={loading}
        error={error}
        actionError={actionError}
        chatMessages={chatMessages}
        eventMessages={eventMessages}
        chatDraft={chatDraft}
        chatError={chatError}
        wsStatus={wsStatus}
        gameID={gameID}
        phase={phase}
        phaseMode={phaseMode}
        meIndex={meIndex}
        isMyTurn={isMyTurn}
        canEnterAttack={canEnterAttack}
        players={players}
        playerColors={playerColors}
        territoryState={territoryState}
        myCards={myCards}
        selectedCardIndices={selectedCardIndices}
        mySetupArmies={mySetupArmies}
        nextTradeBonus={nextTradeBonus}
        pendingReinforcements={pendingReinforcements}
        occupyRequirement={occupyRequirement}
        diceResult={diceResult}
        selectedTerritory={selectedTerritory}
        activeFrom={activeFrom}
        activeTo={activeTo}
        armiesInput={armiesInput}
        clampedArmiesInput={clampedArmiesInput}
        clampedAttackerDice={clampedAttackerDice}
        minArmiesInput={minArmiesInput}
        maxArmiesInput={maxArmiesInput}
        maxAttackDiceAllowed={maxAttackDiceAllowed}
        maxDefendDiceAllowed={maxDefendDiceAllowed}
        canAttackSelection={canAttackSelection}
        renderEventBody={renderEventBody}
        commitReinforcement={commitReinforcement}
        commitFortify={commitFortify}
        commitOccupy={commitOccupy}
        commitTradeCards={commitTradeCards}
        toggleCardSelection={toggleCardSelection}
        onRollDice={onRollDice}
        setAttackerDice={setAttackerDice}
        setArmiesInput={setArmiesInput}
        setChatDraft={setChatDraft}
        onSendChat={onSendChat}
        sendAction={sendAction}
        setSelectedFrom={setSelectedFrom}
        setSelectedTo={setSelectedTo}
        setSelectedTerritory={setSelectedTerritory}
        onRefresh={() => void loadGame()}
        onToggleDesktop={toggleMobileUI}
        onOpenFullscreen={() => setMapFullscreenOpen(true)}
        mapRef={mapRef}
        mapSlotRef={setEmbeddedMapSlotEl}
      />
      {mapPortal}
      {fullscreenMap}
      </>
    );
  }

  return (
    <>
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(280px,1fr)]">
      {/* ── Left: map + event log ── */}
      <section className="grid gap-4">
        {/* Game header */}
        <header className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gc-border bg-gc-surface px-4 py-3">
          <div>
            <h2 className="text-base font-semibold text-gc-text">{game?.name || "Game Room"}</h2>
            <p className="font-mono text-xs text-gc-muted">{gameID}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button className={buttonGhostClass} type="button" onClick={() => void loadGame()} disabled={loading}>
              Refresh
            </button>
            <button className={buttonGhostClass} type="button" onClick={toggleMobileUI}>
              Mobile View
            </button>
            <Link className={buttonGhostClass} to="/app/lobby">
              ← Lobby
            </Link>
          </div>
        </header>

        {loading ? <p className="text-sm text-gc-muted">Loading game…</p> : null}
        {error ? (
          <p className="rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {error}
          </p>
        ) : null}

        {/* Map panel */}
        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-gc-text">Map</h3>
            <div className="flex items-center gap-2">
              <span className="text-xs text-gc-muted capitalize">{game?.status || "—"}</span>
              <button
                className={buttonGhostClass}
                type="button"
                onClick={() => setMapFullscreenOpen(true)}
                aria-label="Expand map to fullscreen"
                title="Expand map to fullscreen"
              >
                ⛶ Fullscreen
              </button>
            </div>
          </div>

          {/* Setup phase banner */}
          {phaseMode === "setup_reinforce" ? (
            <div className="mb-3 rounded-lg border border-sky-700/60 bg-sky-900/30 p-3 text-xs text-sky-300">
              <p className="font-semibold">Initial Troop Placement</p>
              <p className="mt-0.5 text-sky-400">
                {mySetupArmies > 0
                  ? `Click one of your territories to place an army. You have ${mySetupArmies} left.`
                  : "You've placed all your armies. Waiting for other players to finish."}
              </p>
            </div>
          ) : null}

          {/* Phase controls */}
          {!isGameOver ? (
            <div className="mb-3 grid gap-2 rounded-lg border border-gc-border bg-gc-surface-2 p-3 text-xs text-gc-text md:grid-cols-[1fr_auto] md:items-center">
              <div className="grid gap-1">
                <p>
                  Phase:{" "}
                  <span className="font-semibold capitalize text-gc-accent">{phase || "—"}</span>
                  {isMyTurn && pendingReinforcements > 0 ? (
                    <>
                      {" · "}Armies to place:{" "}
                      <span className="font-semibold">{pendingReinforcements}</span>
                    </>
                  ) : null}
                </p>
                {isMyTurn ? (
                  <p className="text-gc-muted">
                    Territory{" "}
                    <span className="font-medium text-gc-text">{selectedTerritory || "—"}</span>
                    {" · "}From{" "}
                    <span className="font-medium text-gc-text">{activeFrom || "—"}</span>
                    {" · "}To{" "}
                    <span className="font-medium text-gc-text">{activeTo || "—"}</span>
                  </p>
                ) : null}
                {phaseMode === "occupy" && occupyRequirement ? (
                  <p className="text-gc-warning">
                    Move required: {occupyRequirement.from} → {occupyRequirement.to} ({occupyRequirement.minMove}–{occupyRequirement.maxMove})
                  </p>
                ) : null}
              </div>
              {isMyTurn ? (
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
                  {phaseMode === "reinforce" && (game?.pendingReinforcements ?? 0) > 0 ? (
                    <button className={buttonPrimaryClass} type="button" onClick={commitReinforcement}>
                      Place
                    </button>
                  ) : null}
                  {phaseMode === "attack" ? (
                    <button
                      className={buttonGhostClass}
                      type="button"
                      onClick={() => {
                        sendAction({ action: "end_attack" });
                        setSelectedFrom("");
                        setSelectedTo("");
                        setSelectedTerritory("");
                      }}
                    >
                      End Attack
                    </button>
                  ) : null}
                  {phaseMode === "occupy" ? (
                    <button className={buttonPrimaryClass} type="button" onClick={commitOccupy}>
                      Move Troops
                    </button>
                  ) : null}
                  {phaseMode === "fortify" ? (
                    <>
                      <button className={buttonGhostClass} type="button" onClick={commitFortify}>
                        Fortify
                      </button>
                      <button
                        className={buttonPrimaryClass}
                        type="button"
                        onClick={() => {
                          sendAction({ action: "end_turn" });
                          setSelectedFrom("");
                          setSelectedTo("");
                          setSelectedTerritory("");
                        }}
                      >
                        End Turn
                      </button>
                    </>
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : null}

          {/* The actual <GameMap> is rendered once above (sharedGameMap) and
              portaled in here — see the comment by embeddedMapSlotEl. */}
          <div
            ref={setEmbeddedMapSlotEl}
            className="relative aspect-[2048/1367] w-full overflow-hidden rounded-xl border border-slate-200 bg-slate-900"
          />
        </section>

        {/* Event log */}
        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-gc-text">Event Log</h3>
            <span className="text-xs text-gc-muted capitalize">{phase || "—"}</span>
          </div>
          <div
            ref={eventScrollRef}
            className="h-[180px] overflow-y-auto rounded-lg border border-gc-border bg-gc-surface-2 p-3 text-sm text-gc-muted"
          >
            {eventMessages.length === 0 ? <p>No events yet.</p> : null}
            <ul className="grid gap-2">
              {eventMessages.map((ev, idx) => {
                const eventColor = eventColorByActorID[ev.actorUserID] ?? "#687a91";
                return (
                  <li key={`${ev.id}-${idx}`} className="rounded-lg bg-gc-surface px-2 py-1.5">
                    <div className="mb-1 flex items-center justify-between gap-2 text-xs" style={{ color: eventColor }}>
                      <span className="font-semibold capitalize">{ev.eventType.replaceAll("_", " ")}</span>
                      <span className="text-gc-muted">
                        {new Date(ev.createdAt).toLocaleString(undefined, {
                          month: "short",
                          day: "numeric",
                          hour: "numeric",
                          minute: "2-digit",
                        })}
                      </span>
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

        {actionError ? (
          <p className="rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {actionError}
          </p>
        ) : null}
      </section>

      {/* ── Right: players, cards, dice, chat ── */}
      <aside className="grid gap-4">
        {/* Players */}
        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-gc-text">Players</h3>
            <span className="text-xs text-gc-muted">
              {players.length}
              {game && game.playerCount > 0 ? ` / ${game.playerCount}` : ""}
            </span>
          </div>
          {players.length === 0 ? (
            <p className="text-sm text-gc-muted">No player data yet.</p>
          ) : null}
          <ul className="grid gap-2">
            {players.map((p) => (
              <li
                key={p.userId}
                className="rounded-lg border border-gc-border bg-gc-surface-2 px-3 py-2"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="flex items-center gap-2 text-sm font-medium">
                    <span
                      className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
                      style={{ backgroundColor: p.color || "#94a3b8" }}
                    />
                    <span style={{ color: p.color || "#dde4f0" }}>{p.userName || p.userId}</span>
                    {p.isBot ? (
                      <span
                        title="Computer-controlled"
                        className="rounded-full border border-gc-border bg-gc-surface px-1.5 py-0.5 text-[10px] font-medium text-gc-muted"
                      >
                        🤖 Computer
                      </span>
                    ) : null}
                  </span>
                  {game && players[game.currentPlayer]?.userId === p.userId ? (
                    <span className="rounded-full border border-gc-success/40 bg-gc-success/10 px-2 py-0.5 text-[11px] font-medium text-gc-success">
                      Playing
                    </span>
                  ) : null}
                </div>
                {phaseMode === "setup_reinforce" ? (
                  <p className="mt-1 text-xs text-gc-muted">
                    Armies to place: <span className="font-semibold text-gc-text">{p.setupArmies}</span>
                  </p>
                ) : (
                  <p className="mt-1 text-xs text-gc-muted">
                    Cards: <span className="text-gc-text">{p.cardCount}</span>
                    {p.eliminated ? (
                      <span className="ml-2 text-gc-danger">· Eliminated</span>
                    ) : null}
                  </p>
                )}
              </li>
            ))}
            {game && game.playerCount > players.length
              ? Array.from({ length: game.playerCount - players.length }).map((_, i) => (
                  <li
                    key={`open-slot-${i}`}
                    className="rounded-lg border border-dashed border-gc-border bg-transparent px-3 py-2 text-sm text-gc-muted"
                  >
                    Open human slot
                  </li>
                ))
              : null}
          </ul>
        </section>

        {/* Cards */}
        {meIndex >= 0 && game?.status === "in_progress" ? (
          <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
            <div className="mb-3 flex items-center justify-between gap-2">
              <h3 className="text-sm font-semibold text-gc-text">My Cards</h3>
              <span className="text-xs text-gc-muted">
                {myCards.length} card{myCards.length !== 1 ? "s" : ""} · next:{" "}
                <span className="font-semibold text-gc-accent">+{nextTradeBonus}</span>
              </span>
            </div>
            {myCards.length === 0 ? (
              <p className="text-xs text-gc-muted">No cards yet. Conquer a territory to earn one.</p>
            ) : (
              <ul className="grid gap-1.5">
                {myCards.map((card, idx) => {
                  const isSelected = selectedCardIndices.includes(idx);
                  const symbolIcon =
                    card.symbol === "infantry"
                      ? "🪖"
                      : card.symbol === "cavalry"
                        ? "🐴"
                        : card.symbol === "artillery"
                          ? "💣"
                          : "⭐";
                  return (
                    <li key={idx}>
                      <button
                        type="button"
                        onClick={() => toggleCardSelection(idx)}
                        className={`w-full rounded-lg border px-3 py-2 text-left text-xs transition-colors ${
                          isSelected
                            ? "border-gc-accent/60 bg-gc-accent/10 text-gc-accent"
                            : "border-gc-border bg-gc-surface-2 text-gc-text hover:border-gc-border/80 hover:bg-gc-surface"
                        }`}
                      >
                        <span className="mr-1.5">{symbolIcon}</span>
                        <span className="font-semibold capitalize">{card.symbol}</span>
                        {card.territory ? (
                          <span className="ml-1.5 text-gc-muted">— {card.territory}</span>
                        ) : null}
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
            {myCards.length >= 5 && isMyTurn && phaseMode === "reinforce" ? (
              <p className="mt-2 rounded-lg border border-gc-warning/40 bg-gc-warning/10 px-2 py-1.5 text-xs font-medium text-gc-warning">
                {(game?.pendingReinforcements ?? 0) === 0
                  ? "You hold 5+ cards. Trade a valid set before continuing."
                  : "Trade cards before placing reinforcements (5+ cards held)."}
              </p>
            ) : null}
            {isMyTurn && phaseMode === "reinforce" && myCards.length >= 3 ? (
              <div className="mt-3 grid gap-2">
                <p className="text-xs text-gc-muted">{selectedCardIndices.length}/3 selected</p>
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

        {/* Dice roller — only shown during your attack turn */}
        {isMyTurn && phaseMode === "attack" ? <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <h3 className="mb-3 text-sm font-semibold text-gc-text">Dice Roller</h3>
          <div className="grid gap-2">
            <p className="text-xs text-gc-muted">
              <span className="font-medium text-gc-text">{selectedFrom || "—"}</span>
              {" → "}
              <span className="font-medium text-gc-text">{selectedTo || "—"}</span>
            </p>
            <div className="grid grid-cols-2 gap-2">
              <label className="grid gap-1 text-xs font-medium text-gc-muted">
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
              <div className="grid gap-1 text-xs font-medium text-gc-muted">
                Defender Dice
                <div className={`${inputClass} bg-gc-surface-2`}>{maxDefendDiceAllowed}</div>
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
              <p className="text-xs text-gc-muted">Select adjacent attacker and defender territories to roll.</p>
            ) : null}
          </div>

          {diceResult ? (
            <div className="mt-3 rounded-lg border border-gc-border bg-gc-surface-2 p-3 text-sm">
              <div className="grid grid-cols-2 gap-2 text-center">
                <div>
                  <p className="text-xs text-gc-muted">Attacker</p>
                  <p className="text-lg font-bold text-gc-text">{diceResult.attacker.join(" · ")}</p>
                  <p className="text-xs text-gc-danger">−{diceResult.attackerLoss}</p>
                </div>
                <div>
                  <p className="text-xs text-gc-muted">Defender</p>
                  <p className="text-lg font-bold text-gc-text">{diceResult.defender.join(" · ")}</p>
                  <p className="text-xs text-gc-success">−{diceResult.defenderLoss}</p>
                </div>
              </div>
            </div>
          ) : null}
        </section> : null}

        {/* Game chat */}
        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-gc-text">Game Chat</h3>
            <div className="flex items-center gap-1.5">
              <span
                className={`h-1.5 w-1.5 rounded-full ${wsStatus === "connected" ? "bg-gc-success" : "bg-gc-danger"}`}
                title={wsStatus}
              />
              <span className="text-xs text-gc-muted">{wsStatus}</span>
            </div>
          </div>
          <div
            ref={chatScrollRef}
            className="h-[260px] overflow-y-auto rounded-lg border border-gc-border bg-gc-surface-2 p-3 text-sm text-gc-muted"
          >
            {chatMessages.length === 0 ? <p>No messages yet.</p> : null}
            <ul className="grid gap-2">
              {chatMessages.map((m, idx) => {
                const nameKey = m.userName.trim().toLowerCase();
                const chatColor = chatColorByUserName[nameKey] ?? "#687a91";
                return (
                  <li
                    key={`${m.userName}-${m.createdAt}-${idx}`}
                    className="rounded-lg bg-gc-surface px-2 py-2"
                  >
                    <div className="mb-1 flex items-center justify-between gap-2">
                      <span className="text-xs font-semibold" style={{ color: chatColor }}>
                        {m.userName}
                      </span>
                      <span className="text-[11px] text-gc-muted">
                        {new Date(m.createdAt).toLocaleString(undefined, {
                          month: "short",
                          day: "numeric",
                          hour: "numeric",
                          minute: "2-digit",
                        })}
                      </span>
                    </div>
                    <p className="whitespace-pre-wrap text-xs" style={{ color: chatColor }}>
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
              placeholder="Type a message…"
            />
            {chatError ? <p className="text-sm text-gc-danger">{chatError}</p> : null}
            <button
              className={buttonPrimaryClass}
              type="submit"
              disabled={chatDraft.trim() === "" || wsStatus !== "connected"}
            >
              Send
            </button>
          </form>
        </section>
      </aside>
    </div>
    {fullscreenMap}
    </>
  );
}
