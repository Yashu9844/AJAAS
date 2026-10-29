/** Shared API vocabulary used by every M2 type file. Mirrors backend/api/swagger.yaml. */

export type Priority = "none" | "low" | "medium" | "high" | "urgent";
export type ColumnCategory = "backlog" | "todo" | "in_progress" | "done" | "canceled";
export type ProjectMode = "kanban" | "scrum";
export type ProjectVisibility = "private" | "tenant";
export type ProjectStatus = "active" | "archived";
export type ProjectRoleKey = "owner" | "manager" | "member" | "viewer";

export const PRIORITIES: readonly Priority[] = ["none", "low", "medium", "high", "urgent"];

/** Pagination block of a list response. */
export interface PageMeta {
  limit?: number;
  next_cursor?: string | null;
}

/** One page of a cursor-paged list. */
export interface Page<T> {
  items: T[];
  next: string | null;
}

/** A successful response after the envelope has been unwrapped. */
export interface ApiResponse<T> {
  data: T;
  meta: PageMeta;
}

export type Query = Record<string, string | number | boolean | null | undefined>;
