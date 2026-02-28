import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type UserRecord = {
  id: string;
  username: string;
  role: string;
  accessStatus: "active" | "blocked";
  createdAt: string;
  updatedAt: string;
};

export type AdminUserRecord = UserRecord & {
  activeSessions: number;
};

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== "object") return null;
  return value as UnknownRecord;
}

function readString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function readNumber(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function normalizeUser(value: unknown): UserRecord {
  const record = asRecord(value);
  if (!record) {
    return {
      id: "",
      username: "",
      role: "player",
      accessStatus: "active",
      createdAt: "",
      updatedAt: "",
    };
  }

  const access = readString(record.access_status ?? record.accessStatus ?? record.AccessStatus, "active");
  return {
    id: readString(record.id ?? record.ID),
    username: readString(record.username ?? record.UserName),
    role: readString(record.role ?? record.Role, "player"),
    accessStatus: access === "blocked" ? "blocked" : "active",
    createdAt: readString(record.created_at ?? record.CreatedAt),
    updatedAt: readString(record.updated_at ?? record.UpdatedAt),
  };
}

function normalizeAdminUser(value: unknown): AdminUserRecord {
  const user = normalizeUser(value);
  const record = asRecord(value);
  return {
    ...user,
    activeSessions: readNumber(record?.active_sessions ?? record?.activeSessions ?? record?.ActiveSessions, 0),
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

export async function listAdminUsers(): Promise<AdminUserRecord[]> {
  const res = await request<unknown>({ method: "GET", url: "/admin/users" });
  if (!Array.isArray(res)) return [];
  return res.map((item) => normalizeAdminUser(item));
}

export async function updateUserAccess(userID: string, accessStatus: "active" | "blocked"): Promise<UserRecord> {
  const res = await request<unknown>({
    method: "PUT",
    url: `/admin/users/${encodeURIComponent(userID)}/access`,
    data: { access_status: accessStatus },
  });
  return normalizeUser(res);
}

export async function revokeUserSessions(userID: string): Promise<number> {
  const res = await request<unknown>({
    method: "POST",
    url: `/admin/users/${encodeURIComponent(userID)}/revoke-sessions`,
  });
  const record = asRecord(res);
  return readNumber(record?.revoked, 0);
}
