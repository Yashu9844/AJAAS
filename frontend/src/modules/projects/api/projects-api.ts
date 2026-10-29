import type { Page, ProjectRoleKey } from "../types/common";
import type { Column, ColumnInput, CreateProjectInput, Label, Member, Person, Project } from "../types/project";
import { del, getData, getPage, idempotencyHeader, patch, post, put } from "./client";
import { seg } from "./query";

export interface ProjectListParams {
  q?: string;
  status?: "active" | "archived";
  favorite?: boolean;
  mine?: boolean;
  cursor?: string;
}

export function listProjects(p: ProjectListParams, signal?: AbortSignal): Promise<Page<Project>> {
  return getPage<Project>("/projects", { ...p, limit: 20 }, signal);
}

export function getProject(id: string): Promise<Project> {
  return getData<Project>(`/projects/${seg(id)}`);
}

export function createProject(input: CreateProjectInput): Promise<Project> {
  return post<Project>("/projects", input, { headers: idempotencyHeader() });
}

export function archiveProject(id: string): Promise<Project> {
  return post<Project>(`/projects/${seg(id)}/archive`);
}

export function restoreProject(id: string): Promise<Project> {
  return post<Project>(`/projects/${seg(id)}/restore`);
}

export function setFavorite(id: string, favorite: boolean): Promise<unknown> {
  return favorite ? put(`/projects/${seg(id)}/favorite`) : del(`/projects/${seg(id)}/favorite`);
}

export function listColumns(projectId: string): Promise<Column[]> {
  return getData<Column[]>(`/projects/${seg(projectId)}/columns`);
}

export function addColumn(projectId: string, input: ColumnInput): Promise<Column> {
  return post<Column>(`/projects/${seg(projectId)}/columns`, input);
}

export function updateColumn(projectId: string, columnId: string, input: ColumnInput): Promise<Column> {
  return patch<Column>(`/projects/${seg(projectId)}/columns/${seg(columnId)}`, input);
}

export function deleteColumn(projectId: string, columnId: string, moveTo?: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/columns/${seg(columnId)}`, { move_to: moveTo });
}

export function reorderColumns(projectId: string, columnIds: string[]): Promise<unknown> {
  return put(`/projects/${seg(projectId)}/columns/order`, { column_ids: columnIds });
}

export function listLabels(projectId: string): Promise<Label[]> {
  return getData<Label[]>(`/projects/${seg(projectId)}/labels`);
}

export function createLabel(projectId: string, name: string, color: string): Promise<Label> {
  return post<Label>(`/projects/${seg(projectId)}/labels`, { name, color });
}

export function deleteLabel(projectId: string, labelId: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/labels/${seg(labelId)}`);
}

export function listMembers(projectId: string): Promise<Member[]> {
  return getData<Member[]>(`/projects/${seg(projectId)}/members`);
}

export function addMember(projectId: string, userId: string, role: ProjectRoleKey): Promise<Member> {
  return post<Member>(`/projects/${seg(projectId)}/members`, { user_id: userId, role });
}

export function changeMemberRole(projectId: string, userId: string, role: ProjectRoleKey): Promise<Member> {
  return patch<Member>(`/projects/${seg(projectId)}/members/${seg(userId)}`, { role });
}

export function removeMember(projectId: string, userId: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/members/${seg(userId)}`);
}

/** One tenant user by id. Only administrators may read users; callers treat a refusal as "name unavailable". */
export function getPerson(id: string): Promise<Person> {
  return getData<Person>(`/users/${seg(id)}`);
}

/** The first page (100) of the user directory, for choosing people to add to a project. */
export async function listPeople(): Promise<Person[]> {
  const res = await getData<Person[] | null>("/users", { query: { per_page: 100 } });
  return res ?? [];
}
