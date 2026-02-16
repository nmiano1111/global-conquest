// src/realtime/socket.ts

import { useEffect, useMemo, useRef, useState } from "react";
import type { Listener, SocketStatus, WsEnvelope } from "./types";
import { safeParseEnvelope } from "./types";

type OnUnsubscribe = () => void;

export type SendOptions = {
  id?: string;
  game_id?: string;
};

export type GameSocket = {
  status: SocketStatus;

  // Low-level: send a full envelope
  sendEnvelope: (env: WsEnvelope) => void;

  // High-level convenience: send by type + payload
  send: (type: string, payload?: unknown, opts?: SendOptions) => string; // returns id

  on: (type: string, fn: Listener) => OnUnsubscribe;

  connect: () => void;
  disconnect: () => void;
};

function computeBackoffMs(attempt: number): number {
  const base = 250 * Math.pow(2, attempt);
  return Math.min(base, 8000);
}

function genId(): string {
  // Browser-native UUID is fine for request ids.
  return crypto.randomUUID();
}

export function useGameSocket(wsUrl: string): GameSocket {
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<SocketStatus>("disconnected");

  const listenersRef = useRef(new Map<string, Set<Listener>>());
  const sendQueueRef = useRef<string[]>([]);
  const reconnectAttemptRef = useRef(0);
  const closedByUserRef = useRef(false);

  const emit = (msg: WsEnvelope) => {
    listenersRef.current.get(msg.type)?.forEach((fn) => fn(msg));
    listenersRef.current.get("*")?.forEach((fn) => fn(msg));
  };

  const on = (type: string, fn: Listener): OnUnsubscribe => {
    let set = listenersRef.current.get(type);
    if (!set) {
      set = new Set();
      listenersRef.current.set(type, set);
    }
    set.add(fn);
    return () => set!.delete(fn);
  };

  const flushQueue = () => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    const q = sendQueueRef.current;
    sendQueueRef.current = [];

    for (const raw of q) ws.send(raw);
  };

  const sendEnvelope = (env: WsEnvelope) => {
    const raw = JSON.stringify(env);
    const ws = wsRef.current;

    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(raw);
    } else {
      sendQueueRef.current.push(raw);
    }
  };

  const send = (type: string, payload?: unknown, opts?: SendOptions): string => {
    const id = opts?.id ?? genId();
    const env: WsEnvelope = {
      type,
      id,
      game_id: opts?.game_id,
      payload: payload ?? undefined,
    };
    sendEnvelope(env);
    return id;
  };

  const connect = () => {
    closedByUserRef.current = false;

    const existing = wsRef.current;
    if (existing && (existing.readyState === WebSocket.CONNECTING || existing.readyState === WebSocket.OPEN)) {
      return;
    }

    setStatus("connecting");

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptRef.current = 0;
      setStatus("connected");
      flushQueue();
      // No client.hello needed; your server sends "hello" on Register.
    };

    ws.onmessage = (ev) => {
      const parsed = safeParseEnvelope(ev.data);
      if (parsed) {
        emit(parsed);
      } else {
        emit({ type: "error.parse", payload: { raw: String(ev.data) } });
      }
    };

    ws.onerror = () => {
      // Let onclose handle reconnect.
    };

    ws.onclose = () => {
      wsRef.current = null;
      setStatus("disconnected");

      if (closedByUserRef.current) return;

      const attempt = reconnectAttemptRef.current++;
      const delay = computeBackoffMs(attempt);

      window.setTimeout(() => {
        if (!closedByUserRef.current) connect();
      }, delay);
    };
  };

  const disconnect = () => {
    closedByUserRef.current = true;
    const ws = wsRef.current;
    wsRef.current = null;

    if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
      ws.close(1000, "client disconnect");
    }

    setStatus("disconnected");
  };

  useEffect(() => {
    connect();
    return () => disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wsUrl]);

  return useMemo(
    () => ({ status, sendEnvelope, send, on, connect, disconnect }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [status, wsUrl]
  );
}