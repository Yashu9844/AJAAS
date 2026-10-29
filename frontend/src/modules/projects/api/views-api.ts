import type { Page } from "../types/common";
import type { BulkChange, BulkOutput, SavedView, SavedViewInput } from "../types/views";
import { del, getPage, post } from "./client";
import { seg } from "./query";

export function listViews(projectId: string): Promise<Page<SavedView>> {
  return getPage<SavedView>("/saved-views", { project_id: projectId, limit: 50 });
}

export function createView(input: SavedViewInput): Promise<SavedView> {
  return post<SavedView>("/saved-views", input);
}

export function deleteView(id: string): Promise<unknown> {
  return del(`/saved-views/${seg(id)}`);
}

/** Updates up to 100 tasks; the answer lists each task outcome (success or error). */
export function bulkUpdate(projectId: string, taskIds: string[], change: BulkChange): Promise<BulkOutput> {
  return post<BulkOutput>(`/projects/${seg(projectId)}/tasks/bulk`, { task_ids: taskIds, ...change });
}
