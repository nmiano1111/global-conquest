import config from "@colyseus/tools";
import { monitor } from "@colyseus/monitor";
import { playground } from "@colyseus/playground";


import express from "express";
import cors from "cors";
import bodyParser from "body-parser";
import jwt from "jsonwebtoken";
import crypto from "crypto";

const JWT_SECRET = process.env.JWT_SECRET || "dev-change-me";

type User = { id: string; email: string; salt: string; hash: string };
const usersByEmail = new Map<string, User>();

function hash(password: string, salt: string) {
  return crypto.pbkdf2Sync(password, salt, 100_000, 32, "sha256").toString("hex");
}
function makeUser(email: string, password: string): User {
  const salt = crypto.randomBytes(16).toString("hex");
  return { id: crypto.randomUUID(), email, salt, hash: hash(password, salt) };
}

/**
 * Import your Room files
 */
import { MyRoom } from "./rooms/MyRoom";

export default config({

    initializeGameServer: (gameServer) => {
        /**
         * Define your room handlers:
         */
        gameServer.define('my_room', MyRoom);

    },

    initializeExpress: (app) => {
        // Dev CORS for your React dev server (adjust port if needed)
        app.use(cors({ origin: ["http://localhost:5173"], credentials: true }));
        app.use(bodyParser.json());

        app.get("/health", (_req, res) => res.json({ ok: true }));

        app.post("/signup", (req, res) => {
        const { email, password } = req.body ?? {};
        if (!email || !password) return res.status(400).json({ error: "missing_fields" });
        if (usersByEmail.has(email)) return res.status(409).json({ error: "email_exists" });
        const u = makeUser(email, password);
        usersByEmail.set(email, u);
        const token = jwt.sign({ sub: u.id, email: u.email }, JWT_SECRET, { expiresIn: "15m" });
        res.json({ token, userId: u.id, email: u.email });
        });

        app.post("/login", (req, res) => {
        const { email, password } = req.body ?? {};
        const u = usersByEmail.get(email);
        if (!u || hash(password, u.salt) !== u.hash) return res.status(401).json({ error: "invalid_credentials" });
        const token = jwt.sign({ sub: u.id, email: u.email }, JWT_SECRET, { expiresIn: "15m" });
        res.json({ token, userId: u.id, email: u.email });
        });

        /**
         * Bind your custom express routes here:
         * Read more: https://expressjs.com/en/starter/basic-routing.html
         */
        app.get("/hello_world", (req, res) => {
            res.send("It's time to kick ass and chew bubblegum!");
        });

        /**
         * Use @colyseus/playground
         * (It is not recommended to expose this route in a production environment)
         */
        if (process.env.NODE_ENV !== "production") {
            app.use("/", playground());
        }

        /**
         * Use @colyseus/monitor
         * It is recommended to protect this route with a password
         * Read more: https://docs.colyseus.io/tools/monitor/#restrict-access-to-the-panel-using-a-password
         */
        app.use("/monitor", monitor());
    },


    beforeListen: () => {
        /**
         * Before before gameServer.listen() is called.
         */
    }
});
