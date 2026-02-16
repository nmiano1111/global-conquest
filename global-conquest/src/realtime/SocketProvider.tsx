// src/realtime/SocketProvider.tsx

import React, { createContext, useContext } from "react";
import type { GameSocket } from "./socket";
import { useGameSocket } from "./socket";

const SocketContext = createContext<GameSocket | null>(null);

function buildWsUrl(): string {
  const explicit = import.meta.env.VITE_WS_URL as string | undefined;
  if (explicit && explicit.trim() !== "") return explicit;

  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws`;
}

export function SocketProvider({ children }: { children: React.ReactNode }) {
  const wsUrl = buildWsUrl();
  const socket = useGameSocket(wsUrl);

  return <SocketContext.Provider value={socket}>{children}</SocketContext.Provider>;
}

export function useSocket(): GameSocket {
  const ctx = useContext(SocketContext);
  if (!ctx) throw new Error("useSocket must be used inside <SocketProvider>");
  return ctx;
}