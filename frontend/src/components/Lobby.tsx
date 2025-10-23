import { useEffect, useState } from "react";
import { Room } from "colyseus.js";
import { useAuth } from "../state/auth";
import { joinOrReconnect } from "../lib/colyseus";

type Presence = { type: "join" | "leave"; userId: string };

export function Lobby() {
  const { user, idToken } = useAuth();
  const [room, setRoom] = useState<Room|null>(null);
  const [peers, setPeers] = useState<string[]>([]);
  const [status, setStatus] = useState("idle");

  useEffect(() => {
    let mounted = true;
    (async () => {
      if (!user || !idToken || room) return;
      setStatus("connecting");
      try {
        const fresh = await user.getIdToken(); // ensure non-stale
        const r = await joinOrReconnect("my_room", { token: fresh });
        if (!mounted) return;
        setRoom(r);
        setStatus("connected");
        r.onMessage("presence", (msg: Presence) => {
          setPeers(prev =>
            msg.type === "join" ? Array.from(new Set([...prev, msg.userId])) : prev.filter(p => p !== msg.userId)
          );
        });
      } catch (e) {
        console.error(e);
        setStatus("error");
      }
    })();
    return () => { mounted = false; };
  }, [user, idToken]);

  if (!user) return <div>Sign in to enter lobby.</div>;
  if (status !== "connected") return <div>{status}…</div>;
  return (
    <div>
      <div>Room: {room?.id}</div>
      <div>Peers ({peers.length}):</div>
      <ul>{peers.map(p => <li key={p}>{p}</li>)}</ul>
    </div>
  );
}
