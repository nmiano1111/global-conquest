import { useMemo, useState } from "react";
import { AuthContext, type AuthContextValue, type AuthUser } from "./context";

type SessionState = {
  token: string | null;
  user: AuthUser | null;
};

const TOKEN_KEY = "gc.auth.token";
const USER_KEY = "gc.auth.user";

function loadSession(): SessionState {
  const token = localStorage.getItem(TOKEN_KEY);
  const rawUser = localStorage.getItem(USER_KEY);
  if (!token || !rawUser) return { token: null, user: null };

  try {
    const parsed = JSON.parse(rawUser) as AuthUser;
    return { token, user: parsed };
  } catch {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    return { token: null, user: null };
  }
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [session, setSessionState] = useState<SessionState>(() => loadSession());

  const value = useMemo<AuthContextValue>(() => {
    const setSession = (token: string, user: AuthUser) => {
      localStorage.setItem(TOKEN_KEY, token);
      localStorage.setItem(USER_KEY, JSON.stringify(user));
      setSessionState({ token, user });
    };

    const clearSession = () => {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
      setSessionState({ token: null, user: null });
    };

    return {
      token: session.token,
      user: session.user,
      isAuthenticated: Boolean(session.token),
      setSession,
      clearSession,
    };
  }, [session.token, session.user]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
