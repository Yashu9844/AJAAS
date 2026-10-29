import type { GanttData } from "../types/planning";
import { isoToDateKey } from "./dates";

/** Pure maths for the Gantt chart: a day-based horizontal scale and bar positions. All dates are local YYYY-MM-DD keys. */

const DAY_MS = 86_400_000;

export interface GanttScale {
  fromKey: string;
  days: number;
  pxPerDay: number;
}

export interface BarBox {
  kind: "bar" | "marker";
  left: number;
  width: number;
}

function dayNumber(key: string): number {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
  if (!m) return NaN;
  return Math.round(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])) / DAY_MS);
}

export function makeScale(fromKey: string, toKey: string, pxPerDay: number): GanttScale {
  const days = Math.max(1, dayNumber(toKey) - dayNumber(fromKey) + 1);
  return { fromKey, days: Number.isFinite(days) ? days : 1, pxPerDay };
}

export function scaleWidth(scale: GanttScale): number {
  return scale.days * scale.pxPerDay;
}

/** Pixel offset of the start of a day (can be negative or beyond the width when the day is outside the window). */
export function dayOffset(scale: GanttScale, key: string): number {
  return (dayNumber(key) - dayNumber(scale.fromKey)) * scale.pxPerDay;
}

/** Offset of the today line, or null when today is outside the window. */
export function todayOffset(scale: GanttScale, todayKey: string): number | null {
  const x = dayOffset(scale, todayKey) + scale.pxPerDay / 2;
  return x >= 0 && x <= scaleWidth(scale) ? x : null;
}

/**
 * Position of a task. With both start and due it is a bar from the start day to the end of the due day; with one date it
 * is a one-day marker; without dates it has no box (null). A due date before the start is shown as a one-day marker.
 */
export function barBox(scale: GanttScale, startOn: string | null, dueAt: string | null): BarBox | null {
  const start = startOn ?? "";
  const due = isoToDateKey(dueAt);
  if (!start && !due) return null;
  const first = start || due;
  const last = due || start;
  const days = dayNumber(last) - dayNumber(first) + 1;
  const left = dayOffset(scale, first);
  if (days <= 1) return { kind: "marker", left, width: scale.pxPerDay };
  return { kind: "bar", left, width: days * scale.pxPerDay };
}

/** Week starts inside the window, for the header ticks. */
export function weekTicks(scale: GanttScale): { key: string; left: number }[] {
  const ticks: { key: string; left: number }[] = [];
  const base = dayNumber(scale.fromKey);
  for (let i = 0; i < scale.days; i++) {
    const d = new Date((base + i) * DAY_MS);
    if (d.getUTCDay() === 1 || i === 0) {
      ticks.push({ key: `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}-${String(d.getUTCDate()).padStart(2, "0")}`, left: i * scale.pxPerDay });
    }
  }
  return ticks;
}

/** Window of `days` days that starts `before` days before the given key. */
export function windowAround(todayKey: string, before: number, days: number): { from: string; to: string } {
  const base = dayNumber(todayKey);
  const fmt = (n: number) => {
    const d = new Date(n * DAY_MS);
    return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}-${String(d.getUTCDate()).padStart(2, "0")}`;
  };
  return { from: fmt(base - before), to: fmt(base - before + days - 1) };
}

/** Moves a window by whole days. */
export function shiftWindow(w: { from: string; to: string }, deltaDays: number): { from: string; to: string } {
  const length = dayNumber(w.to) - dayNumber(w.from) + 1;
  return windowAround(w.from, -deltaDays, length);
}

export interface ArrowLine {
  fromId: string;
  toId: string;
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** A straight line from the end of the blocking task to the start of the blocked one; null when either has no box. */
export function arrowLine(fromId: string, toId: string, boxes: ReadonlyMap<string, { box: BarBox; row: number }>, rowHeight: number): ArrowLine | null {
  const a = boxes.get(fromId);
  const b = boxes.get(toId);
  if (!a || !b) return null;
  return {
    fromId,
    toId,
    x1: a.box.left + a.box.width,
    y1: a.row * rowHeight + rowHeight / 2,
    x2: b.box.left,
    y2: b.row * rowHeight + rowHeight / 2,
  };
}

/** Merges a further page: new tasks and dependencies are added, milestones are replaced (they repeat on every page). */
export function mergeGanttPage(prev: GanttData, page: GanttData): GanttData {
  const seen = new Set(prev.tasks.map((t) => t.id));
  const depKey = (d: { source_task_id: string; target_task_id: string }) => `${d.source_task_id}>${d.target_task_id}`;
  const seenDeps = new Set(prev.dependencies.map(depKey));
  return {
    tasks: [...prev.tasks, ...page.tasks.filter((t) => !seen.has(t.id))],
    milestones: page.milestones.length ? page.milestones : prev.milestones,
    dependencies: [...prev.dependencies, ...page.dependencies.filter((d) => !seenDeps.has(depKey(d)))],
    next_cursor: page.next_cursor,
  };
}
