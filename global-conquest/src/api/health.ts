// src/api/health.ts
import { request } from "./client";

export type HealthResponse = { message: "pong" };

export function getHealth() {
  return request<HealthResponse>({ method: "GET", url: "/ping" });
}
