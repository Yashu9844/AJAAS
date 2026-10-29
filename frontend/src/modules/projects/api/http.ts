import type { ApiResponse, PageMeta, Query } from "../types/common";
import { ApiError, networkError, parseErrorBody, sessionLostError } from "./errors";
import { buildQuery } from "./query";

/** Everything the HTTP layer needs from the session; injected so the layer can be checked without a browser. */
export interface HttpDeps {
  baseUrl(): string | null;
  accessToken(): string | null;
  /** Refreshes the session once (callers share one in-flight refresh); true when new tokens are in place. */
  refresh(): Promise<boolean>;
  onSessionLost(): void;
  fetchFn?: typeof fetch;
  sleep?: (ms: number) => Promise<void>;
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  query?: Query;
  body?: unknown;
  headers?: Record<string, string>;
  /** Public endpoints (login, refresh) send no bearer token and never trigger a refresh. */
  noAuth?: boolean;
  signal?: AbortSignal;
}

const DEFAULT_RETRY_SECONDS = 2;
const MAX_RETRY_SECONDS = 30;

/** Seconds to wait from the Retry-After header (the API exposes it to browsers); a short default applies only when it is absent or unreadable. */
export function retryAfterSeconds(header: string | null): number {
  const n = header === null ? NaN : Number.parseInt(header, 10);
  if (!Number.isFinite(n) || n < 0) return DEFAULT_RETRY_SECONDS;
  return Math.min(n, MAX_RETRY_SECONDS);
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** Unwraps {data, meta}; a body without a data key is returned as is. */
export function unwrapEnvelope<T>(text: string): ApiResponse<T> {
  if (!text) return { data: null as T, meta: {} };
  const body: unknown = JSON.parse(text);
  if (isRecord(body) && "data" in body) {
    const meta: PageMeta = isRecord(body.meta) ? (body.meta as PageMeta) : {};
    return { data: body.data as T, meta };
  }
  return { data: body as T, meta: {} };
}

export interface Http {
  request<T>(path: string, opts?: RequestOptions): Promise<ApiResponse<T>>;
}

export function createHttp(deps: HttpDeps): Http {
  const doFetch = deps.fetchFn ?? ((...a: Parameters<typeof fetch>) => fetch(...a));
  const sleep = deps.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));

  async function send(path: string, opts: RequestOptions): Promise<Response> {
    const base = deps.baseUrl();
    if (base === null) throw sessionLostError();
    const headers: Record<string, string> = { Accept: "application/json", ...opts.headers };
    if (opts.body !== undefined) headers["Content-Type"] = "application/json";
    const token = deps.accessToken();
    if (!opts.noAuth && token) headers.Authorization = `Bearer ${token}`;
    try {
      return await doFetch(`${base}/api/v1${path}${buildQuery(opts.query)}`, {
        method: opts.method ?? "GET",
        headers,
        body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
        signal: opts.signal,
      });
    } catch (e) {
      if (e instanceof DOMException && e.name === "AbortError") throw e;
      throw networkError();
    }
  }

  async function sendWithRecovery(path: string, opts: RequestOptions): Promise<Response> {
    let res = await send(path, opts);
    if (res.status === 429) {
      await sleep(retryAfterSeconds(res.headers.get("Retry-After")) * 1000);
      res = await send(path, opts);
    }
    if (res.status === 401 && !opts.noAuth) {
      const ok = await deps.refresh();
      if (!ok) {
        deps.onSessionLost();
        throw sessionLostError();
      }
      res = await send(path, opts);
      if (res.status === 401) {
        deps.onSessionLost();
        throw sessionLostError();
      }
    }
    return res;
  }

  async function request<T>(path: string, opts: RequestOptions = {}): Promise<ApiResponse<T>> {
    const res = await sendWithRecovery(path, opts);
    const text = await res.text();
    if (!res.ok) {
      const retry = res.status === 429 ? retryAfterSeconds(res.headers.get("Retry-After")) : undefined;
      throw parseErrorBody(res.status, text, retry);
    }
    try {
      return unwrapEnvelope<T>(text);
    } catch {
      throw new ApiError(res.status, "BAD_RESPONSE", "The server answered with something unexpected.");
    }
  }

  return { request };
}
