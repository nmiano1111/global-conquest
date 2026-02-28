import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type UserRecord = {
  id: string;
  username: string;
  role: string;
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

function normalizeUser(value: unknown): UserRecord {
  const record = asRecord(value);
  if (!record) {
    return { id: "", username: "", role: "player", createdAt: "", updatedAt: "" };
  }

  return {
    id: readString(record.id ?? record.ID),
    username: readString(record.username ?? record.UserName),
    role: readString(record.role ?? record.Role, "player"),
    createdAt: readString(record.created_at ?? record.CreatedAt),
    updatedAt: readString(record.updated_at ?? record.UpdatedAt),
  };
}

export async function listUsers(): Promise<UserRecord[]> {
  const res = await request<unknown>({ method: "GET", url: "/users/" });
  if (!Array.isArray(res)) return [];
  return res.map((item) => normalizeUser(item));
}

export async function getUserByUsername(username: string): Promise<UserRecord> {
  const res = await request<unknown>({
    method: "GET",
    url: `/users/${encodeURIComponent(username)}`,
  });
  return normalizeUser(res);
}
