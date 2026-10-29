/** Date helpers. Task dates are ISO strings from the API; all grouping uses the viewer's local calendar day. */

const pad = (n: number) => String(n).padStart(2, "0");

export function toDateKey(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Local YYYY-MM-DD of an ISO timestamp, or "" when missing or invalid. */
export function isoToDateKey(iso: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : toDateKey(d);
}

/** Turns a date input value into the due timestamp stored for the task (17:00 local on that day). */
export function dateKeyToDueAt(key: string): string | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
  if (!m) return null;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), 17, 0, 0).toISOString();
}

/** A task is overdue when it has a due date in the past and is not in a done or canceled column. */
export function isOverdue(dueAt: string | null, category: string, nowMs: number): boolean {
  if (!dueAt || category === "done" || category === "canceled") return false;
  const t = new Date(dueAt).getTime();
  return !Number.isNaN(t) && t < nowMs;
}

export function formatDay(iso: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export function formatDateTime(iso: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export interface CalendarCell {
  key: string;
  day: number;
  inMonth: boolean;
}

/** The six-week grid (Monday first) that shows `month` (0-11) of `year`: 42 cells. */
export function monthGrid(year: number, month: number): CalendarCell[] {
  const first = new Date(year, month, 1);
  const offset = (first.getDay() + 6) % 7;
  const cells: CalendarCell[] = [];
  for (let i = 0; i < 42; i++) {
    const d = new Date(year, month, 1 - offset + i);
    cells.push({ key: toDateKey(d), day: d.getDate(), inMonth: d.getMonth() === month });
  }
  return cells;
}

/** The API window for a month grid: first and last visible day (42 days, within the 62-day limit). */
export function gridWindow(cells: CalendarCell[]): { from: string; to: string } {
  return { from: cells[0].key, to: cells[cells.length - 1].key };
}

export function shiftMonth(year: number, month: number, delta: number): { year: number; month: number } {
  const d = new Date(year, month + delta, 1);
  return { year: d.getFullYear(), month: d.getMonth() };
}
