// src/realtime/SocketProvider.tsx
import type { ReactNode } from "react";
import { useGameSocket } from "./socket";
import { SocketContext } from "./context";

function buildWsUrl(): string {
  const explicit = import.meta.env.VITE_WS_URL as string | undefined;
  if (explicit && explicit.trim() !== "") return explicit;

  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws`;
}

export function SocketProvider({ children }: { children: ReactNode }) {
  const wsUrl = buildWsUrl();
  const socket = useGameSocket(wsUrl);

  return <SocketContext.Provider value={socket}>{children}</SocketContext.Provider>;
}
