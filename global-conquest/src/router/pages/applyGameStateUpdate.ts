import type { GameBootstrap } from "../../api/games";
import { MAP_PLAYER_COLORS, type DiceRollResult, type GameEventMessage } from "./gameShared";

type UnknownRecord = Record<string, unknown>;

export function parseTerritories(raw: unknown): Record<string, unknown> {
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
}

export type GameStateUpdateResult = {
  nextGame: GameBootstrap;
  diceResult?: DiceRollResult;
  eventMessage?: GameEventMessage;
  lastActionTerritory: string;
  lastActionFrom: string;
  lastActionTo: string;
  lastActionType: string;
};

/**
 * Turns one game_state_updated-shaped payload into the next board state
 * and the visual cues (dice popup, highlight territories, log line) that
 * go with it. This is the single place that logic lives — GamePage's live
 * socket handler and the replay page both call this with the same payload
 * shape (the live one arriving over the socket, replay's fetched from
 * storage), so replay produces the identical sequence of visible change a
 * live spectator would have seen, action by action.
 */
export function applyGameStateUpdate(prevGame: GameBootstrap, payload: unknown): GameStateUpdateResult {
  const p = (payload && typeof payload === "object" ? payload : {}) as UnknownRecord;
  const action = typeof p.action === "string" ? p.action : "";
  const phase = typeof p.phase === "string" ? p.phase : "";
  const currentPlayer = typeof p.current_player === "number" ? p.current_player : -1;
  const pendingReinforcements = typeof p.pending_reinforcements === "number" ? p.pending_reinforcements : 0;
  const setsTraded = typeof p.sets_traded === "number" ? p.sets_traded : undefined;
  const occupyRaw = p.occupy && typeof p.occupy === "object" ? (p.occupy as UnknownRecord) : null;
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
  const territories = parseTerritories(p.territories);
  const incomingPlayersRaw = Array.isArray(p.players) ? p.players : [];
  const incomingPlayers = incomingPlayersRaw
    .filter((v): v is UnknownRecord => !!v && typeof v === "object")
    .map((pl) => ({
      userId: typeof pl.user_id === "string" ? pl.user_id : "",
      cardCount: typeof pl.card_count === "number" ? pl.card_count : 0,
      setupArmies: typeof pl.setup_armies === "number" ? pl.setup_armies : 0,
      eliminated: pl.eliminated === true,
    }))
    .filter((pl) => pl.userId !== "");

  const metaByID = new Map(prevGame.players.map((pl) => [pl.userId, pl]));
  const nextPlayers = incomingPlayers.map((pl, idx) => {
    const meta = metaByID.get(pl.userId);
    return {
      userId: pl.userId,
      userName: meta?.userName || pl.userId,
      color: meta?.color || MAP_PLAYER_COLORS[idx % MAP_PLAYER_COLORS.length],
      cardCount: pl.cardCount,
      cards: meta?.cards ?? [],
      setupArmies: pl.setupArmies,
      eliminated: pl.eliminated,
      isBot: meta?.isBot ?? false,
    };
  });

  const nextGame: GameBootstrap = {
    ...prevGame,
    phase,
    currentPlayer,
    pendingReinforcements,
    setsTraded: setsTraded ?? prevGame.setsTraded,
    occupy,
    territories,
    players: nextPlayers,
    // Once true, stays true — replay capture never turns off mid-game, so
    // this only ever needs to flip on live as soon as the game's first
    // action commits, without waiting for a fresh bootstrap fetch.
    replayAvailable: prevGame.replayAvailable || p.replay_available === true,
  };

  let diceResult: DiceRollResult | undefined;
  if (action === "attack" && p.result && typeof p.result === "object") {
    const result = p.result as UnknownRecord;
    const attacker = Array.isArray(result.attacker_rolls)
      ? result.attacker_rolls.filter((v): v is number => typeof v === "number")
      : [];
    const defender = Array.isArray(result.defender_rolls)
      ? result.defender_rolls.filter((v): v is number => typeof v === "number")
      : [];
    const attackerLoss = typeof result.attacker_loss === "number" ? result.attacker_loss : 0;
    const defenderLoss = typeof result.defender_loss === "number" ? result.defender_loss : 0;
    diceResult = { attacker, defender, attackerLoss, defenderLoss };
  }

  let eventMessage: GameEventMessage | undefined;
  if (p.event && typeof p.event === "object") {
    const event = p.event as UnknownRecord;
    const gameID = typeof event.game_id === "string" ? event.game_id : typeof p.game_id === "string" ? p.game_id : "";
    const candidate: GameEventMessage = {
      id: typeof event.id === "string" ? event.id : `${gameID}-${Date.now()}`,
      gameID,
      actorUserID: typeof event.actor_user_id === "string" ? event.actor_user_id : "",
      eventType: typeof event.event_type === "string" ? event.event_type : "game_event",
      body: typeof event.body === "string" ? event.body : "",
      createdAt: typeof event.created_at === "string" ? event.created_at : new Date().toISOString(),
    };
    if (candidate.body.trim() !== "") {
      eventMessage = candidate;
    }
  }

  return {
    nextGame,
    diceResult,
    eventMessage,
    lastActionTerritory: typeof p.action_territory === "string" ? p.action_territory : "",
    lastActionFrom: typeof p.action_from === "string" ? p.action_from : "",
    lastActionTo: typeof p.action_to === "string" ? p.action_to : "",
    lastActionType: action,
  };
}
