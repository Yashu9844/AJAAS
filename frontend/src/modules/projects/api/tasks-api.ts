import type { Page, Query } from "../types/common";
import type {
  Board,
  CustomFieldDefinition,
  CustomFieldValue,
  MoveInput,
  Relation,
  RelationType,
  Subtask,
  Task,
  TaskCreateInput,
  TaskListFilters,
  TaskUpdate,
} from "../types/task";
import { del, getData, getPage, idempotencyHeader, patch, post, put } from "./client";
import { seg } from "./query";

const TICKET_RE = /^[A-Za-z][A-Za-z0-9]{1,9}-[0-9]+$/;

export function isTicketKey(s: string): boolean {
  return TICKET_RE.test(s);
}

/** Loads a task by ticket key (KEY-12) or id. */
export function getTask(idOrKey: string): Promise<Task> {
  return getData<Task>(isTicketKey(idOrKey) ? `/tickets/${seg(idOrKey)}` : `/tasks/${seg(idOrKey)}`);
}

export function createTask(projectId: string, input: TaskCreateInput): Promise<Task> {
  return post<Task>(`/projects/${seg(projectId)}/tasks`, input, { headers: idempotencyHeader() });
}

/** Edits a task; the expected version travels in the body and a stale one answers 409 VERSION_CONFLICT. */
export function updateTask(id: string, version: number, changes: TaskUpdate): Promise<Task> {
  return patch<Task>(`/tasks/${seg(id)}`, { ...changes, version });
}

export function moveTask(id: string, input: MoveInput): Promise<Task> {
  return post<Task>(`/tasks/${seg(id)}/move`, input);
}

export function archiveTask(id: string): Promise<Task> {
  return post<Task>(`/tasks/${seg(id)}/archive`);
}

export function getBoard(projectId: string, columnCursors: Record<string, string> = {}): Promise<Board> {
  const query: Query = { per_column: 30 };
  for (const [col, cursor] of Object.entries(columnCursors)) query[`cursor_${col}`] = cursor;
  return getData<Board>(`/projects/${seg(projectId)}/board`, { query });
}

export function listTasks(projectId: string, f: TaskListFilters, cursor?: string, signal?: AbortSignal): Promise<Page<Task>> {
  return getPage<Task>(
    `/projects/${seg(projectId)}/tasks`,
    { q: f.q, column_id: f.column_id, priority: f.priority, assignee: f.assignee, overdue: f.overdue, blocked: f.blocked, sort: f.sort, dir: f.dir, top_level: true, cursor, limit: 30 },
    signal,
  );
}

export function assignUsers(id: string, userIds: string[]): Promise<unknown> {
  return post(`/tasks/${seg(id)}/assignees`, { user_ids: userIds });
}

export function unassignUser(id: string, userId: string): Promise<unknown> {
  return del(`/tasks/${seg(id)}/assignees/${seg(userId)}`);
}

export function addTaskLabel(id: string, labelId: string): Promise<unknown> {
  return post(`/tasks/${seg(id)}/labels`, { label_ids: [labelId] });
}

export function removeTaskLabel(id: string, labelId: string): Promise<unknown> {
  return del(`/tasks/${seg(id)}/labels/${seg(labelId)}`);
}

export function listSubtasks(id: string): Promise<Subtask[]> {
  return getData<Subtask[]>(`/tasks/${seg(id)}/subtasks`);
}

export function listRelations(id: string): Promise<Relation[]> {
  return getData<Relation[]>(`/tasks/${seg(id)}/relations`);
}

export function addRelation(id: string, targetTaskId: string, type: RelationType): Promise<unknown> {
  return post(`/tasks/${seg(id)}/relations`, { target_task_id: targetTaskId, type });
}

export function removeRelation(id: string, relationId: string): Promise<unknown> {
  return del(`/tasks/${seg(id)}/relations/${seg(relationId)}`);
}

export function listCustomFieldDefinitions(projectId: string): Promise<CustomFieldDefinition[]> {
  return getData<CustomFieldDefinition[]>(`/projects/${seg(projectId)}/custom-fields`);
}

export function listCustomFieldValues(id: string): Promise<CustomFieldValue[]> {
  return getData<CustomFieldValue[]>(`/tasks/${seg(id)}/custom-fields`);
}

export function setCustomFieldValues(id: string, version: number, values: Record<string, unknown>): Promise<unknown> {
  return put(`/tasks/${seg(id)}/custom-fields`, { values, version });
}

export function duplicateTask(id: string, copyLabels: boolean, copyAssignees: boolean): Promise<Task> {
  return post<Task>(`/tasks/${seg(id)}/duplicate`, { copy_labels: copyLabels, copy_assignees: copyAssignees });
}

export function restoreTask(id: string): Promise<Task> {
  return post<Task>(`/tasks/${seg(id)}/restore`);
}

/** Moves a task to another project: it gets a new ticket key there; the old key keeps resolving. */
export function moveTaskToProject(id: string, projectId: string, version: number): Promise<Task> {
  return post<Task>(`/tasks/${seg(id)}/move-project`, { project_id: projectId, version });
}
