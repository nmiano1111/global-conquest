import { useMemo, useState } from "react";
import { AuthContext, type AuthContextValue, type AuthUser } from "./context";
import {
  clearSessionInStorage,
  loadSessionFromStorage,
  saveSessionToStorage,
  type SessionState,
} from "./storage";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [session, setSessionState] = useState<SessionState>(() => loadSessionFromStorage());

  const value = useMemo<AuthContextValue>(() => {
    const setSession = (token: string, user: AuthUser) => {
      saveSessionToStorage(token, user);
      setSessionState({ token, user });
    };

    const clearSession = () => {
      clearSessionInStorage();
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
