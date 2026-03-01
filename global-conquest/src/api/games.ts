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
