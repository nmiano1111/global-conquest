// src/realtime/socket.ts

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  // Fallback for non-secure contexts (plain HTTP)
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
  });
}

export function useGameSocket(wsUrl: string): GameSocket {
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<SocketStatus>("disconnected");

  const listenersRef = useRef(new Map<string, Set<Listener>>());
  const sendQueueRef = useRef<string[]>([]);
  const reconnectAttemptRef = useRef(0);
  const closedByUserRef = useRef(false);
  const connectIdRef = useRef(0);

  const emit = useCallback((msg: WsEnvelope) => {
    listenersRef.current.get(msg.type)?.forEach((fn) => fn(msg));
    listenersRef.current.get("*")?.forEach((fn) => fn(msg));
  }, []);

  const on = useCallback((type: string, fn: Listener): OnUnsubscribe => {
    let set = listenersRef.current.get(type);
    if (!set) {
      set = new Set();
      listenersRef.current.set(type, set);
    }
    set.add(fn);
    return () => set!.delete(fn);
  }, []);

  const flushQueue = useCallback(() => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    const q = sendQueueRef.current;
    sendQueueRef.current = [];

    for (const raw of q) ws.send(raw);
  }, []);

  const sendEnvelope = useCallback((env: WsEnvelope) => {
    const raw = JSON.stringify(env);
    const ws = wsRef.current;

    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(raw);
    } else {
      sendQueueRef.current.push(raw);
    }
  }, []);

  const send = useCallback((type: string, payload?: unknown, opts?: SendOptions): string => {
    const id = opts?.id ?? genId();
    const env: WsEnvelope = {
      type,
      id,
      game_id: opts?.game_id,
      payload: payload ?? undefined,
    };
    sendEnvelope(env);
    return id;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const connect = useCallback(() => {
    closedByUserRef.current = false;

    const existing = wsRef.current;
    if (existing && (existing.readyState === WebSocket.CONNECTING || existing.readyState === WebSocket.OPEN)) {
      return;
    }

    setStatus("connecting");

    // Every connect() gets its own generation id. A socket's event handlers
    // only act if they're still the most recent connect() call — this stops
    // a late/async close event from a since-abandoned socket (e.g. the brief
    // anonymous connection created mid account-switch) from rescheduling a
    // reconnect that clobbers wsRef with a stale, wrongly-authenticated socket.
    const connectId = ++connectIdRef.current;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      if (connectIdRef.current !== connectId) return;
      reconnectAttemptRef.current = 0;
      setStatus("connected");
      flushQueue();
      // No client.hello needed; your server sends "hello" on Register.
    };

    ws.onmessage = (ev) => {
      if (connectIdRef.current !== connectId) return;
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
      if (connectIdRef.current !== connectId) return;

      wsRef.current = null;
      setStatus("disconnected");

      if (closedByUserRef.current) return;

      const attempt = reconnectAttemptRef.current++;
      const delay = computeBackoffMs(attempt);

      window.setTimeout(() => {
        if (connectIdRef.current === connectId && !closedByUserRef.current) connect();
      }, delay);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wsUrl]);

  const disconnect = useCallback(() => {
    closedByUserRef.current = true;
    const ws = wsRef.current;
    wsRef.current = null;

    if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
      ws.close(1000, "client disconnect");
    }

    setStatus("disconnected");
  }, []);

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