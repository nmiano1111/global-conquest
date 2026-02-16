// src/realtime/types.ts

export type WsEnvelope<TPayload = unknown> = {
  type: string; // Go: wsmsg.Type
  id?: string; // client-generated message id
  correlation_id?: string; // server correlates to client id
  game_id?: string;
  payload?: TPayload; // decoded JSON object (frontend convenience)
};

export type SocketStatus = "disconnected" | "connecting" | "connected";

export type Listener<T = any> = (msg: WsEnvelope<T>) => void;

export function safeParseEnvelope(raw: unknown): WsEnvelope | null {
  if (typeof raw !== "string") return null;
  try {
    const v = JSON.parse(raw);
    if (!v || typeof v !== "object") return null;
    if (typeof (v as any).type !== "string") return null;
    return v as WsEnvelope;
  } catch {
    return null;
  }
}