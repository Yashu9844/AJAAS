import type { ApiResponse, Page, Query } from "../types/common";
import { createHttp, type RequestOptions } from "./http";
import { currentAccessToken, currentBaseUrl, markSessionLost, refreshTokens } from "./session-store";

/** The one shared HTTP client of the product UI. */
const http = createHttp({
  baseUrl: currentBaseUrl,
  accessToken: currentAccessToken,
  refresh: refreshTokens,
  onSessionLost: markSessionLost,
});

export type CallOptions = Omit<RequestOptions, "noAuth">;

/** Calls the API and returns the unwrapped data. */
export async function getData<T>(path: string, opts?: CallOptions): Promise<T> {
  return (await http.request<T>(path, opts)).data;
}

/** Calls the API and returns data plus pagination meta. */
export function call<T>(path: string, opts?: CallOptions): Promise<ApiResponse<T>> {
  return http.request<T>(path, opts);
}

/** GET a cursor-paged list. */
export async function getPage<T>(path: string, query?: Query, signal?: AbortSignal): Promise<Page<T>> {
  const res = await http.request<T[] | null>(path, { query, signal });
  return { items: res.data ?? [], next: res.meta.next_cursor ?? null };
}

export function post<T>(path: string, body?: unknown, opts?: CallOptions): Promise<T> {
  return getData<T>(path, { ...opts, method: "POST", body: body ?? {} });
}

export function patch<T>(path: string, body: unknown, opts?: CallOptions): Promise<T> {
  return getData<T>(path, { ...opts, method: "PATCH", body });
}

export function put<T>(path: string, body?: unknown): Promise<T> {
  return getData<T>(path, { method: "PUT", body: body ?? {} });
}

export function del<T = null>(path: string, query?: Query): Promise<T> {
  return getData<T>(path, { method: "DELETE", query });
}

/** A fresh key makes a create safe to replay (Idempotency-Key). */
export function idempotencyHeader(): Record<string, string> {
  return { "Idempotency-Key": crypto.randomUUID() };
}
