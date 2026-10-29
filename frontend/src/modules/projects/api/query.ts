import type { Query } from "../types/common";

/** Encodes a query object, dropping empty values. */
export function buildQuery(q?: Query): string {
  if (!q) return "";
  const parts: string[] = [];
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === "" || v === false) continue;
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`);
  }
  return parts.length ? `?${parts.join("&")}` : "";
}

/** Encodes one path segment so an id or key can never alter the route. */
export function seg(value: string): string {
  return encodeURIComponent(value);
}
