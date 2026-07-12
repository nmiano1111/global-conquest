// src/api/client.ts
import axios from "axios";
import type { AxiosError, AxiosInstance, AxiosRequestConfig } from "axios";
import { getStoredToken } from "../auth/storage";

export type ApiError = {
  status: number | null;
  message: string;
  details?: unknown;
};

function env(name: string, fallback = ""): string {
  const value = import.meta.env[name];
  return typeof value === "string" ? value : fallback;
}

const baseURL =
  env("VITE_API_BASE_URL") ||
  (import.meta.env.DEV ? "/api" : ""); // dev via Vite proxy, prod same-origin by default

export const api: AxiosInstance = axios.create({
  baseURL,
  timeout: 10_000,
  headers: {
    "Content-Type": "application/json",
  },
  // withCredentials: true, // only if you use cookies later
});

api.interceptors.request.use((config) => {
  const token = getStoredToken();
  if (!token) return config;

  config.headers = config.headers ?? {};
  if (!("Authorization" in config.headers)) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

function toApiError(err: unknown): ApiError {
  if (!axios.isAxiosError(err)) {
    return { status: null, message: err instanceof Error ? err.message : String(err) };
  }

  const axErr = err as AxiosError<unknown>;
  const status = axErr.response?.status ?? null;

  // Try to preserve server-provided error payloads
  const details = axErr.response?.data;

  // Prefer a useful message. The backend's error responses use
  // `{"error": "..."}` (see httpapi/handler.go); some also use `message`,
  // so check both rather than falling back to Axios's generic
  // "Request failed with status code N" text.
  let message = axErr.message || "Request failed";
  if (details && typeof details === "object") {
    const detailRecord = details as { message?: unknown; error?: unknown };
    const detailMessage =
      typeof detailRecord.message === "string" && detailRecord.message.trim() !== ""
        ? detailRecord.message
        : typeof detailRecord.error === "string" && detailRecord.error.trim() !== ""
          ? detailRecord.error
          : undefined;
    if (detailMessage) {
      message = detailMessage;
    }
  }

  return { status, message, details };
}

export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  try {
    const res = await api.request<T>(config);
    return res.data;
  } catch (e) {
    throw toApiError(e);
  }
}
