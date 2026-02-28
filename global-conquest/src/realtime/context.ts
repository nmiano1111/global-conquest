import { createContext } from "react";
import type { GameSocket } from "./socket";

export const SocketContext = createContext<GameSocket | null>(null);
