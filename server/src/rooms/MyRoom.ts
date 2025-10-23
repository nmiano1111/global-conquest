import { Room, Client } from "@colyseus/core";
import { MyRoomState } from "./schema/MyRoomState";
import admin from "firebase-admin";

if (!admin.apps.length) {
  admin.initializeApp({
    credential: admin.credential.cert({
      projectId: process.env.FIREBASE_PROJECT_ID!,
      clientEmail: process.env.FIREBASE_CLIENT_EMAIL!,
      privateKey: process.env.FIREBASE_PRIVATE_KEY!.replace(/\\n/g, "\n"),
    }),
  });
}

export class MyRoom extends Room<MyRoomState> {
  maxClients = 4;
  state = new MyRoomState();

  async onAuth(client: Client, options: any) {
    console.log(options)
    const token = options?.token;
    if (!token) return false;

    try {
      const decoded = await admin.auth().verifyIdToken(token);
      (client as any).userId = decoded.uid;
      (client as any).email = decoded.email;
      return true;
    } catch (err) {
      console.error("Invalid Firebase token", err);
      return false;
    }
  }

  onCreate (options: any) {
    this.onMessage("type", (client, message) => {
      //
      // handle "type" message
      //
    });
  }

  onJoin (client: Client, options: any) {
    console.log(client.sessionId, "joined!");
  }

  onLeave (client: Client, consented: boolean) {
    console.log(client.sessionId, "left!");
  }

  onDispose() {
    console.log("room", this.roomId, "disposing...");
  }

}
