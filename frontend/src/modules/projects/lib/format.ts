import type { Person } from "../types/project";

/** 90 -> "1h 30m". */
export function formatMinutes(total: number): string {
  if (!Number.isFinite(total) || total <= 0) return "0m";
  const h = Math.floor(total / 60);
  const m = Math.round(total % 60);
  if (h === 0) return `${m}m`;
  return m === 0 ? `${h}h` : `${h}h ${m}m`;
}

/** "1h 30m", "90", "2h", "45m" -> minutes, or null when it cannot be read. */
export function parseMinutes(text: string): number | null {
  const t = text.trim().toLowerCase();
  if (/^\d+$/.test(t)) return Number(t);
  const m = /^(?:(\d+)\s*h)?\s*(?:(\d+)\s*m)?$/.exec(t);
  if (!m || (m[1] === undefined && m[2] === undefined)) return null;
  return Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0);
}

export function shortId(id: string): string {
  return id.slice(0, 8);
}

/** Label for the administrator's user picker only: name plus e-mail so same-named people can be told apart. */
export function directoryLabel(p: Person): string {
  const full = `${p.first_name} ${p.last_name}`.trim();
  return full ? `${full} (${p.email})` : p.email;
}

export function initials(name: string): string {
  const parts = name.split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  return (parts[0][0] + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toUpperCase();
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

/** "in_progress" -> "In progress". */
export function humanize(value: string): string {
  const s = value.replace(/[_-]+/g, " ").trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
}
