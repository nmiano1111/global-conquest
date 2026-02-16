// src/App.tsx

import { useEffect, useMemo, useRef, useState } from "react";
import { getHealth } from "./api/health";
import "./App.css";

import { useSocket } from "./realtime";

function App() {
  const [status, setStatus] = useState<string>("loading...");
  const [clientInfo, setClientInfo] = useState<{ client_id?: string; name?: string } | null>(null);

  const { status: wsStatus, on, send } = useSocket();

  // Track pending ping ids so we can confirm correlation works.
  const pendingPingsRef = useRef(new Set<string>());

  useEffect(() => {
    getHealth()
      .then((r) => setStatus(r.message))
      .catch((e) => setStatus(`error: ${e.status ?? "?"} ${e.message}`));
  }, []);

  useEffect(() => {
    const offHello = on("hello", (msg) => {
      // Your server sends payload: { client_id, name }
      const p = msg.payload as any;
      setClientInfo({ client_id: p?.client_id, name: p?.name });
    });

    const offPong = on("pong", (msg) => {
      const corr = msg.correlation_id ?? "";
      if (corr && pendingPingsRef.current.has(corr)) {
        pendingPingsRef.current.delete(corr);
        // eslint-disable-next-line no-console
        console.log("pong (correlated):", corr);
      } else {
        // eslint-disable-next-line no-console
        console.log("pong (uncorrelated):", msg);
      }
    });

    const offError = on("error", (msg) => {
      // eslint-disable-next-line no-console
      console.error("server error:", msg.payload);
    });

    return () => {
      offHello();
      offPong();
      offError();
    };
  }, [on]);

  const canPing = wsStatus === "connected";

  const onPing = () => {
    const id = send("ping"); // sends {type:"ping", id:"uuid"}
    pendingPingsRef.current.add(id);
  };

  return (
    <div style={{ padding: 16 }}>
      <div>HTTP health: {status}</div>
      <div>WS status: {wsStatus}</div>

      <div style={{ marginTop: 12 }}>
        <div>
          WS client: {clientInfo?.client_id ? `${clientInfo.client_id} (${clientInfo.name ?? "anon"})` : "(no hello yet)"}
        </div>
      </div>

      <button style={{ marginTop: 12 }} onClick={onPing} disabled={!canPing}>
        Ping
      </button>
    </div>
  );
}

export default App;