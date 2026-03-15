import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type GameRecord = {
  id: string;
  ownerUserId: string;
  status: string;
  state: unknown;
  playerCount: number | null;
  playerIds: string[];
  createdAt: string;
  updatedAt: string;
};

export type CreateGameRequest = {
  playerCount: number;
};

export type Card = {
  territory: string;
  symbol: string;
};

export type GameBootstrapPlayer = {
  userId: string;
  userName: string;
  color: string;
  cardCount: number;
  cards: Card[];
  eliminated: boolean;
};

export type GameOccupyRequirement = {
  from: string;
  to: string;
  minMove: number;
  maxMove: number;
};

export type GameEventEntry = {
  id: string;
  gameID: string;
  actorUserID: string;
  eventType: string;
  body: string;
  createdAt: string;
};

export type GameBootstrap = {
  id: string;
  ownerUserId: string;
  status: string;
  phase: string;
  currentPlayer: number;
  pendingReinforcements: number;
  setsTraded: number;
  occupy: GameOccupyRequirement | null;
  players: GameBootstrapPlayer[];
  territories: Record<string, unknown>;
  events: GameEventEntry[];
  createdAt: string;
  updatedAt: string;
};

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== "object") return null;
  return value as UnknownRecord;
}

function readString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function normalizeGame(value: unknown): GameRecord {
  const record = asRecord(value);
  if (!record) {
    return {
      id: "",
      ownerUserId: "",
      status: "",
      state: null,
      playerCount: null,
      playerIds: [],
      createdAt: "",
      updatedAt: "",
    };
  }

  const rawState = record.state;
  let playerCount: number | null = null;
  let playerIds: string[] = [];
  if (rawState && typeof rawState === "object") {
    const stateRecord = rawState as UnknownRecord;
    const count = stateRecord.player_count;
    if (typeof count === "number") {
      playerCount = count;
    }
    if (Array.isArray(stateRecord.player_ids)) {
      playerIds = stateRecord.player_ids.filter((v): v is string => typeof v === "string");
    }
    // In-progress games store Risk engine state, which includes `players` but not lobby metadata.
    if (playerCount === null && Array.isArray(stateRecord.players)) {
      const players = stateRecord.players.filter((v): v is UnknownRecord => !!v && typeof v === "object");
      playerCount = players.length;
      playerIds = players
        .map((p) => p.id)
        .filter((id): id is string => typeof id === "string");
    }
  }

  return {
    id: readString(record.id ?? record.ID),
    ownerUserId: readString(record.owner_user_id ?? record.OwnerUserID),
    status: readString(record.status ?? record.Status),
    state: rawState,
    playerCount,
    playerIds,
    createdAt: readString(record.created_at ?? record.CreatedAt),
    updatedAt: readString(record.updated_at ?? record.UpdatedAt),
  };
}

function readNumber(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function normalizeGameBootstrap(value: unknown): GameBootstrap {
  const record = asRecord(value);
  if (!record) {
    return {
      id: "",
      ownerUserId: "",
      status: "",
      phase: "",
      currentPlayer: -1,
      pendingReinforcements: 0,
      setsTraded: 0,
      occupy: null,
      players: [],
      territories: {},
      events: [],
      createdAt: "",
      updatedAt: "",
    };
  }
  const playersRaw = Array.isArray(record.players) ? record.players : [];
  const players = playersRaw
    .filter((v): v is UnknownRecord => !!v && typeof v === "object")
    .map((p) => {
      const cardsRaw = Array.isArray(p.cards) ? p.cards : [];
      const cards: Card[] = cardsRaw
        .filter((c): c is UnknownRecord => !!c && typeof c === "object")
        .map((c) => ({
          territory: readString(c.territory ?? c.Territory),
          symbol: readString(c.symbol ?? c.Symbol),
        }));
      return {
        userId: readString(p.user_id ?? p.userId),
        userName: readString(p.user_name ?? p.userName),
        color: readString(p.color ?? p.Color),
        cardCount: readNumber(p.card_count ?? p.cardCount),
        cards,
        eliminated: p.eliminated === true,
      };
    });
  const territories =
    record.territories && typeof record.territories === "object"
      ? (record.territories as Record<string, unknown>)
      : {};
  const occupyRaw = record.occupy && typeof record.occupy === "object" ? (record.occupy as UnknownRecord) : null;
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
  const eventsRaw = Array.isArray(record.events) ? record.events : [];
  const events = eventsRaw
    .filter((v): v is UnknownRecord => !!v && typeof v === "object")
    .map((e) => ({
      id: readString(e.id ?? e.ID),
      gameID: readString(e.game_id ?? e.gameID),
      actorUserID: readString(e.actor_user_id ?? e.actorUserID),
      eventType: readString(e.event_type ?? e.eventType),
      body: readString(e.body ?? e.Body),
      createdAt: readString(e.created_at ?? e.createdAt),
    }));
  return {
    id: readString(record.id ?? record.ID),
    ownerUserId: readString(record.owner_user_id ?? record.OwnerUserID),
    status: readString(record.status ?? record.Status),
    phase: readString(record.phase ?? record.Phase),
    currentPlayer: readNumber(record.current_player ?? record.currentPlayer, -1),
    pendingReinforcements: readNumber(record.pending_reinforcements ?? record.pendingReinforcements, 0),
    setsTraded: readNumber(record.sets_traded ?? record.setsTraded, 0),
    occupy,
    players,
    territories,
    events,
    createdAt: readString(record.created_at ?? record.CreatedAt),
    updatedAt: readString(record.updated_at ?? record.UpdatedAt),
  };
}

export async function listGames(): Promise<GameRecord[]> {
  const res = await request<unknown>({ method: "GET", url: "/games/" });
  if (!Array.isArray(res)) return [];
  return res.map((item) => normalizeGame(item));
}

export async function createGame(input: CreateGameRequest): Promise<GameRecord> {
  const res = await request<unknown>({
    method: "POST",
    url: "/games/",
    data: {
      player_count: input.playerCount,
    },
  });
  return normalizeGame(res);
}

export async function joinGame(gameID: string): Promise<GameRecord> {
  const res = await request<unknown>({
    method: "POST",
    url: `/games/${encodeURIComponent(gameID)}/join`,
  });
  return normalizeGame(res);
}

export async function getGame(gameID: string): Promise<GameRecord> {
  const res = await request<unknown>({
    method: "GET",
    url: `/games/${encodeURIComponent(gameID)}`,
  });
  return normalizeGame(res);
}

export async function getGameBootstrap(gameID: string): Promise<GameBootstrap> {
  const res = await request<unknown>({
    method: "GET",
    url: `/games/${encodeURIComponent(gameID)}/bootstrap`,
  });
  return normalizeGameBootstrap(res);
}
