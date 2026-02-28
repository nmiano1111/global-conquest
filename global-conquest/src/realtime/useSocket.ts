import { useContext } from "react";
import { SocketContext } from "./context";
import type { GameSocket } from "./socket";

export function useSocket(): GameSocket {
  const ctx = useContext(SocketContext);
  if (!ctx) throw new Error("useSocket must be used inside <SocketProvider>");
  return ctx;
}
