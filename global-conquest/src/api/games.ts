import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type GameRecord = {
  id: string;
  ownerUserId: string;
  status: string;
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
    return { id: "", ownerUserId: "", status: "", createdAt: "", updatedAt: "" };
  }

  return {
    id: readString(record.id ?? record.ID),
    ownerUserId: readString(record.owner_user_id ?? record.OwnerUserID),
    status: readString(record.status ?? record.Status),
    createdAt: readString(record.created_at ?? record.CreatedAt),
    updatedAt: readString(record.updated_at ?? record.UpdatedAt),
  };
}

export async function listGames(): Promise<GameRecord[]> {
  const res = await request<unknown>({ method: "GET", url: "/games/" });
  if (!Array.isArray(res)) return [];
  return res.map((item) => normalizeGame(item));
}
