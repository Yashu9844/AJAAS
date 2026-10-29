import type { TaskListFilters } from "../types/task";
import type { SavedView, ViewFilters, ViewSortKey } from "../types/views";

export const EMPTY_FILTERS: TaskListFilters = { q: "", column_id: "", priority: "", assignee: "", overdue: false, blocked: false, sort: "rank", dir: "asc" };

/**
 * Maps the list filters to the server's saved-view document. The server accepts only its allow-listed keys, so the
 * "blocked" filter (which a saved view cannot express) is left out.
 */
export function filtersToView(f: TaskListFilters): { filters: ViewFilters; sort: ViewSortKey[] } {
  const filters: ViewFilters = {};
  if (f.q.trim()) filters.q = f.q.trim();
  if (f.column_id) filters.column_ids = [f.column_id];
  if (f.priority) filters.priorities = [f.priority];
  if (f.assignee) filters.assignee_ids = [f.assignee];
  if (f.overdue) filters.overdue = true;
  return { filters, sort: [{ field: f.sort, dir: f.dir }] };
}

/** Maps a saved view back to list filters (only the first value of a list is used: the filter bar holds one). */
export function viewToFilters(view: Pick<SavedView, "filters" | "sort">): TaskListFilters {
  const f = view.filters ?? {};
  const first = view.sort?.[0];
  return {
    ...EMPTY_FILTERS,
    q: f.q ?? "",
    column_id: f.column_ids?.[0] ?? "",
    priority: f.priorities?.[0] ?? "",
    assignee: f.assignee_ids?.[0] ?? "",
    overdue: f.overdue === true,
    sort: first?.field ?? "rank",
    dir: first?.dir === "desc" ? "desc" : "asc",
  };
}

/** True when the view holds something the filter bar cannot show (so the person is told). */
export function viewHasUnshownParts(view: Pick<SavedView, "filters">): boolean {
  const f = view.filters ?? {};
  return (f.column_ids?.length ?? 0) > 1 || (f.priorities?.length ?? 0) > 1 || (f.assignee_ids?.length ?? 0) > 1;
}
