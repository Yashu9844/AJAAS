import type { CalendarData, GanttData, MyWork, SearchHit, Sprint } from "../types/planning";
import type { Page } from "../types/common";
import type { TaskCard } from "../types/task";
import { getData, getPage, patch, post } from "./client";
import { seg } from "./query";

export function getMyWork(): Promise<MyWork> {
  return getData<MyWork>("/my-work", { query: { per_group: 50 } });
}

export function searchTasks(q: string, signal?: AbortSignal): Promise<SearchHit[]> {
  return getData<SearchHit[] | null>("/search", { query: { q, limit: 10 }, signal }).then((r) => r ?? []);
}

export function getCalendar(projectId: string, from: string, to: string): Promise<CalendarData> {
  return getData<CalendarData>(`/projects/${seg(projectId)}/calendar`, { query: { from, to } });
}

export function listSprints(projectId: string): Promise<Sprint[]> {
  return getData<Sprint[]>(`/projects/${seg(projectId)}/sprints`);
}

export function createSprint(projectId: string, name: string, startOn: string, endOn: string): Promise<Sprint> {
  return post<Sprint>(`/projects/${seg(projectId)}/sprints`, { name, start_on: startOn || undefined, end_on: endOn || undefined });
}

export function startSprint(sprintId: string): Promise<Sprint> {
  return post<Sprint>(`/sprints/${seg(sprintId)}/start`);
}

/** Completes the active sprint; unfinished tasks go to "backlog" or to the id of the next planned sprint. */
export function completeSprint(sprintId: string, moveUnfinishedTo: string): Promise<Sprint> {
  return post<Sprint>(`/sprints/${seg(sprintId)}/complete`, { move_unfinished_to: moveUnfinishedTo });
}

export function renameSprint(sprint: Sprint, name: string): Promise<Sprint> {
  return patch<Sprint>(`/sprints/${seg(sprint.id)}`, { name, version: sprint.version });
}

export interface PlanResult {
  task_id?: string;
  result?: string;
  error?: string;
}

/** Moves tasks between the backlog (sprintId null) and a sprint; the response lists each task outcome. */
export function planTasks(projectId: string, taskIds: string[], sprintId: string | null): Promise<PlanResult[]> {
  return post<PlanResult[]>(`/projects/${seg(projectId)}/plan`, { task_ids: taskIds, sprint_id: sprintId });
}

export function getBacklog(projectId: string): Promise<Page<TaskCard>> {
  return getPage<TaskCard>(`/projects/${seg(projectId)}/backlog`, { limit: 50 });
}

/** One page of the Gantt window; the cursor for the next page is part of the data. */
export function getGantt(projectId: string, from: string, to: string, cursor?: string): Promise<GanttData> {
  return getData<GanttData>(`/projects/${seg(projectId)}/gantt`, { query: { from, to, cursor, limit: 100 } });
}
