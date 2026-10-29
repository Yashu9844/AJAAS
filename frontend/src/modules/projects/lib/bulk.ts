import type { BulkItemResult, BulkOutput } from "../types/views";

export const BULK_LIMIT = 100;

/** Adds or removes one id from a selection (returns a new set). */
export function toggleSelection(selected: ReadonlySet<string>, id: string): Set<string> {
  const next = new Set(selected);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
}

/** "Select page": everything visible is selected unless it already is, then everything visible is cleared. Capped at the bulk limit. */
export function togglePage(selected: ReadonlySet<string>, visibleIds: readonly string[]): Set<string> {
  const allSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id));
  const next = new Set(selected);
  if (allSelected) visibleIds.forEach((id) => next.delete(id));
  else visibleIds.forEach((id) => next.size < BULK_LIMIT && next.add(id));
  return next;
}

/** Keeps only ids that are still shown (after a reload or a filter change). */
export function pruneSelection(selected: ReadonlySet<string>, visibleIds: readonly string[]): Set<string> {
  const visible = new Set(visibleIds);
  return new Set([...selected].filter((id) => visible.has(id)));
}

export interface BulkSummaryLine {
  taskId: string;
  label: string;
  ok: boolean;
  message: string;
}

export interface BulkSummary {
  succeeded: number;
  failed: number;
  lines: BulkSummaryLine[];
}

/** Turns the server's per-task answer into lines a person can read; failures come first. `labelOf` gives the ticket key. */
export function summarizeBulk(output: Pick<BulkOutput, "results">, labelOf: (taskId: string) => string): BulkSummary {
  const line = (r: BulkItemResult): BulkSummaryLine => ({
    taskId: r.task_id,
    label: labelOf(r.task_id),
    ok: r.ok,
    message: r.ok ? "Updated" : (r.error?.message ?? r.error?.code ?? "Failed"),
  });
  const lines = output.results.map(line).sort((a, b) => Number(a.ok) - Number(b.ok));
  const succeeded = lines.filter((l) => l.ok).length;
  return { succeeded, failed: lines.length - succeeded, lines };
}
