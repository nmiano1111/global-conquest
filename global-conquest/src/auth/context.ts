import { createContext } from "react";

export type AuthUser = {
  id: string;
  username: string;
  role: string;
};

export type AuthContextValue = {
  token: string | null;
  user: AuthUser | null;
  isAuthenticated: boolean;
  setSession: (token: string, user: AuthUser) => void;
  clearSession: () => void;
};

export const AuthContext = createContext<AuthContextValue | null>(null);
