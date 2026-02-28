import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type SessionUser = {
  id: string;
  username: string;
  role: string;
};

export type LoginResponse = {
  token: string;
  expires_at: string;
  user: SessionUser;
};

export type SignupRequest = {
  username: string;
  password: string;
};

export type LoginRequest = SignupRequest;

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== "object") return null;
  return value as UnknownRecord;
}

function readString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function normalizeUser(value: unknown): SessionUser {
  const record = asRecord(value);
  if (!record) return { id: "", username: "", role: "player" };

  return {
    id: readString(record.id ?? record.ID),
    username: readString(record.username ?? record.UserName),
    role: readString(record.role ?? record.Role, "player"),
  };
}

export async function signup(input: SignupRequest): Promise<SessionUser> {
  const res = await request<unknown>({
    method: "POST",
    url: "/users/",
    data: input,
  });
  return normalizeUser(res);
}

export async function login(input: LoginRequest): Promise<LoginResponse> {
  const res = await request<unknown>({
    method: "POST",
    url: "/auth/login",
    data: input,
  });

  const record = asRecord(res);
  if (!record) {
    return { token: "", expires_at: "", user: { id: "", username: "", role: "player" } };
  }

  return {
    token: readString(record.token),
    expires_at: readString(record.expires_at),
    user: normalizeUser(record.user),
  };
}
