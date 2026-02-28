import type { AuthUser } from "./context";

export const TOKEN_KEY = "gc.auth.token";
export const USER_KEY = "gc.auth.user";

export type SessionState = {
  token: string | null;
  user: AuthUser | null;
};

export function loadSessionFromStorage(): SessionState {
  const token = localStorage.getItem(TOKEN_KEY);
  const rawUser = localStorage.getItem(USER_KEY);
  if (!token || !rawUser) return { token: null, user: null };

  try {
    const parsed = JSON.parse(rawUser) as AuthUser;
    return { token, user: parsed };
  } catch {
    clearSessionInStorage();
    return { token: null, user: null };
  }
}

export function saveSessionToStorage(token: string, user: AuthUser): void {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearSessionInStorage(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

export function getStoredToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}
