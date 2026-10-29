/** Saved views (VW-06) and bulk results. Filters follow the server's allow-list; unknown keys are refused. */

export interface ViewFilters {
  column_ids?: string[];
  priorities?: string[];
  assignee_ids?: string[];
  overdue?: boolean;
  q?: string;
}

export interface ViewSortKey {
  field: string;
  dir: "asc" | "desc";
}

export interface SavedView {
  id: string;
  project_id: string | null;
  owner_user_id: string;
  name: string;
  layout: string;
  filters: ViewFilters;
  sort: ViewSortKey[];
  grouping: string;
  shared: boolean;
  version: number;
}

export interface SavedViewInput {
  name: string;
  layout: "list";
  project_id: string;
  filters: ViewFilters;
  sort: ViewSortKey[];
  shared: boolean;
}

export interface BulkItemResult {
  task_id: string;
  ok: boolean;
  version?: number;
  error?: { code: string; message: string };
}

export interface BulkOutput {
  results: BulkItemResult[];
  succeeded: number;
  failed: number;
}

export interface BulkChange {
  column_id?: string;
  priority?: string;
  archive?: boolean;
}
