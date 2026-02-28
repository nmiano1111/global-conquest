import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type LobbyMessage = {
  id: string;
  room: string;
  userId: string;
  userName: string;
  body: string;
  createdAt: string;
};

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== "object") return null;
  return value as UnknownRecord;
}

function readString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

export function normalizeLobbyMessage(value: unknown): LobbyMessage {
  const record = asRecord(value);
  if (!record) {
    return { id: "", room: "lobby", userId: "", userName: "", body: "", createdAt: "" };
  }

  return {
    id: readString(record.id ?? record.ID),
    room: readString(record.room ?? record.Room, "lobby"),
    userId: readString(record.user_id ?? record.UserID),
    userName: readString(record.user_name ?? record.UserName),
    body: readString(record.body ?? record.Body),
    createdAt: readString(record.created_at ?? record.CreatedAt),
  };
}

export async function listLobbyMessages(limit = 100): Promise<LobbyMessage[]> {
  const res = await request<unknown>({
    method: "GET",
    url: "/chat/lobby/messages",
    params: { limit },
  });
  if (!Array.isArray(res)) return [];
  return res.map((v) => normalizeLobbyMessage(v));
}

export async function postLobbyMessage(body: string): Promise<LobbyMessage> {
  const res = await request<unknown>({
    method: "POST",
    url: "/chat/lobby/messages",
    data: { body },
  });
  return normalizeLobbyMessage(res);
}
