import { Client, Room } from "colyseus.js";
const WS = import.meta.env.VITE_WS_BASE as string;
let client: Client | null = null;
const key = (name: string) => `colyseus.session.${name}`;

export function getClient() { return client ??= new Client(WS); }

export async function joinOrReconnect(roomName: string, options: any): Promise<Room> {
  const c = getClient();
  const saved = localStorage.getItem(key(roomName));
  if (saved) {
    try {
      const { roomId, sessionId } = JSON.parse(saved);
      return await c.reconnect(roomId, sessionId);
    } catch { /* fresh join */ }
  }
  const room = await c.joinOrCreate(roomName, options);
  localStorage.setItem(key(roomName), JSON.stringify({ roomId: room.id, sessionId: room.sessionId }));
  room.onLeave(() => localStorage.removeItem(key(roomName)));
  return room;
}
