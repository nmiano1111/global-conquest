// src/api/client.ts
import axios from "axios";
import type { AxiosError, AxiosInstance, AxiosRequestConfig } from "axios";

export type ApiError = {
  status: number | null;
  message: string;
  details?: unknown;
};

function env(name: string, fallback = ""): string {
  return (import.meta as any).env?.[name] ?? fallback;
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

// ---- Request interceptor: attach auth token later ----
// api.interceptors.request.use(async (config) => {
//   const token = await getAccessTokenSomehow(); // Auth0 hook/utility
//   if (token) config.headers.Authorization = `Bearer ${token}`;
//   return config;
// });

function toApiError(err: unknown): ApiError {
  if (!axios.isAxiosError(err)) {
    return { status: null, message: err instanceof Error ? err.message : String(err) };
  }

  const axErr = err as AxiosError<any>;
  const status = axErr.response?.status ?? null;

  // Try to preserve server-provided error payloads
  const details = axErr.response?.data;

  // Prefer a useful message
  const message =
    (typeof details?.message === "string" && details.message) ||
    axErr.message ||
    "Request failed";

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
