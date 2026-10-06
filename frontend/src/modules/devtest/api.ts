import type { CallSpec, Exchange } from "./types";

export interface Endpoint {
  host: string;
  port: string;
  platformKey?: string; // sent as X-Platform-Key on /tenants routes
}

let seq = 0;

export function maskToken(t: string): string {
  return t.length > 12 ? `${t.slice(0, 8)}…••••` : "••••";
}

export function buildUrl(spec: CallSpec, ep: Endpoint): string {
  const origin =
    spec.scope === "tenant"
      ? `http://${spec.slug ?? ""}.${ep.host}:${ep.port}`
      : `http://${ep.host}:${ep.port}`;
  const q = spec.query
    ? Object.entries(spec.query)
        .filter(([, v]) => v !== "")
        .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
        .join("&")
    : "";
  return `${origin}/api/v1${spec.path}${q ? `?${q}` : ""}`;
}

/** Performs a REAL fetch against the backend and records the full exchange. Never throws. */
export async function callApi(spec: CallSpec, ep: Endpoint): Promise<Exchange> {
  const url = buildUrl(spec, ep);
  const headers: Record<string, string> = { Accept: "application/json" };
  let payload: string | undefined;
  if (spec.rawBody !== undefined) {
    payload = spec.rawBody;
    headers["Content-Type"] = "application/json";
  } else if (spec.body !== undefined) {
    payload = JSON.stringify(spec.body);
    headers["Content-Type"] = "application/json";
  }
  if (spec.token) headers["Authorization"] = `Bearer ${spec.token}`;
  if (spec.scope === "global" && spec.path.startsWith("/tenants") && ep.platformKey && !spec.noPlatformKey) headers["X-Platform-Key"] = ep.platformKey;

  if (spec.headers) Object.assign(headers, spec.headers);

  const shown = { ...headers };
  if (spec.token) shown["Authorization"] = `Bearer ${maskToken(spec.token)}`;

  const ex: Exchange = {
    id: `x${Date.now().toString(36)}${seq++}`,
    ts: Date.now(),
    opKey: spec.opKey,
    method: spec.method,
    url,
    reqHeaders: shown,
    reqBody: spec.rawBody !== undefined ? spec.rawBody : spec.body,
    status: 0,
    statusText: "",
    ms: 0,
    resBody: null,
  };
  const t0 = performance.now();
  try {
    const res = await fetch(url, { method: spec.method, headers, body: payload });
    ex.status = res.status;
    ex.statusText = res.statusText;
    const text = await res.text();
    ex.ms = Math.round(performance.now() - t0);
    try {
      ex.resBody = text ? JSON.parse(text) : null;
    } catch {
      ex.resBody = text;
    }
  } catch (e) {
    ex.ms = Math.round(performance.now() - t0);
    ex.error = e instanceof Error ? e.message : String(e);
  }
  return ex;
}

export function getPath(obj: unknown, path: string): unknown {
  let cur: unknown = obj;
  for (const part of path.split(".")) {
    if (cur && typeof cur === "object" && part in (cur as Record<string, unknown>)) {
      cur = (cur as Record<string, unknown>)[part];
    } else return undefined;
  }
  return cur;
}

export function errorOf(ex: Exchange): string {
  if (ex.error) return `Network error: ${ex.error}`;
  const b = ex.resBody as { error?: unknown } | null;
  if (b && typeof b === "object" && "error" in b) {
    const e = b.error;
    if (typeof e === "string") return e;
    if (e && typeof e === "object") {
      const o = e as { code?: string; message?: string };
      return `${o.code ?? ""} ${o.message ?? ""}`.trim();
    }
  }
  return "";
}

export interface JwtClaims {
  exp?: number;
  iat?: number;
  tid?: string;
  sid?: string;
  sub?: string;
  roles?: string[];
}

export function decodeJwt(token: string): JwtClaims | null {
  try {
    const p = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(atob(p)) as JwtClaims;
  } catch {
    return null;
  }
}
