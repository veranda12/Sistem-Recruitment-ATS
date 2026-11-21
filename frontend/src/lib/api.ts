import type { components } from "./api-types";

export type Schemas = components["schemas"];
export type Session = Schemas["Session"];
export type ErrorBody = Schemas["ErrorBody"];

/** Error API dengan code mesin stabil, pesan Indonesia, fieldErrors dan requestId. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fieldErrors: Record<string, string>;
  readonly requestId: string;
  readonly details: unknown;
  readonly retryAfter: number | null;
  constructor(status: number, body: Partial<ErrorBody> | null, retryAfter: number | null) {
    const e = body?.error;
    super(e?.message ?? (status === 0 ? "Tidak dapat terhubung ke server. Periksa koneksi Anda." : "Terjadi kesalahan. Coba lagi."));
    this.status = status;
    this.code = e?.code ?? (status === 0 ? "network" : "unknown");
    this.fieldErrors = (e?.fieldErrors as Record<string, string> | undefined) ?? {};
    this.requestId = body?.requestId ?? "";
    this.details = e?.details;
    this.retryAfter = retryAfter;
  }
}

let csrfToken = "";
export function setCsrf(token: string) {
  csrfToken = token;
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export async function api<T>(method: Method, path: string, body?: unknown, headers: Record<string, string> = {}, signal?: AbortSignal): Promise<T> {
  const h: Record<string, string> = { Accept: "application/json", ...headers };
  if (body !== undefined) h["Content-Type"] = "application/json";
  if (method !== "GET" && csrfToken) h["X-CSRF-Token"] = csrfToken;
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, {
      method,
      headers: h,
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: "same-origin",
      signal,
    });
  } catch (err) {
    if ((err as Error).name === "AbortError") throw err;
    throw new ApiError(0, null, null);
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let parsed: unknown = null;
  try {
    parsed = text ? JSON.parse(text) : null;
  } catch {
    parsed = null;
  }
  if (!res.ok) {
    const ra = res.headers.get("Retry-After");
    throw new ApiError(res.status, parsed as ErrorBody | null, ra ? Number(ra) : null);
  }
  return parsed as T;
}

export const get = <T,>(path: string, signal?: AbortSignal) => api<T>("GET", path, undefined, {}, signal);
export const post = <T,>(path: string, body?: unknown, headers?: Record<string, string>) => api<T>("POST", path, body ?? {}, headers);
export const put = <T,>(path: string, body: unknown) => api<T>("PUT", path, body);
export const patch = <T,>(path: string, body: unknown) => api<T>("PATCH", path, body);

export function newIdempotencyKey(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

export const del = <T,>(path: string) => api<T>("DELETE", path);

/** Unggah multipart (FormData). Content-Type diatur browser beserta boundary. */
export async function apiForm<T>(path: string, form: FormData): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, { method: "POST", body: form, credentials: "same-origin", headers: csrfToken ? { "X-CSRF-Token": csrfToken } : {} });
  } catch {
    throw new ApiError(0, null, null);
  }
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(res.status, body as ErrorBody | null, null);
  return body as T;
}
