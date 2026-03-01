// src/realtime/SocketProvider.tsx
import type { ReactNode } from "react";
import { useGameSocket } from "./socket";
import { SocketContext } from "./context";
import { useAuth } from "../auth";

function buildWsUrl(): string {
  const explicit = import.meta.env.VITE_WS_URL as string | undefined;
  if (explicit && explicit.trim() !== "") return explicit;

  if (import.meta.env.DEV) {
    return "ws://127.0.0.1:8080/ws";
  }

  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws`;
}

export function SocketProvider({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const base = buildWsUrl();
  const wsUrl = auth.token
    ? `${base}${base.includes("?") ? "&" : "?"}token=${encodeURIComponent(auth.token)}`
    : base;
  const socket = useGameSocket(wsUrl);

  return <SocketContext.Provider value={socket}>{children}</SocketContext.Provider>;
}
