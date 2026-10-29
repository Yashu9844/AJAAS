import type { ColumnCategory, ProjectMode, ProjectRoleKey, ProjectStatus, ProjectVisibility } from "./common";

export interface Project {
  id: string;
  key: string;
  name: string;
  description: string;
  status: ProjectStatus;
  visibility: ProjectVisibility;
  mode: ProjectMode;
  owner_user_id: string;
  start_on: string | null;
  due_on: string | null;
  icon: string;
  color: string;
  task_counter: number;
  time_tracking_enabled: boolean;
  version: number;
  created_at: string;
  updated_at: string;
  last_activity_at: string;
  archived_at: string | null;
  favorite: boolean;
  /** Empty for tenant administrators who are not members. */
  my_role: ProjectRoleKey | "";
}

export interface CreateProjectInput {
  key: string;
  name: string;
  visibility: ProjectVisibility;
  mode: ProjectMode;
  description?: string;
}

export interface Column {
  id: string;
  project_id: string;
  name: string;
  key: string;
  position: number;
  color: string;
  category: ColumnCategory;
  wip_limit: number | null;
  wip_hard: boolean;
  is_default: boolean;
  task_count: number;
}

export interface ColumnInput {
  name?: string;
  category?: ColumnCategory;
  color?: string;
  wip_limit?: number;
  clear_wip_limit?: boolean;
  wip_hard?: boolean;
}

export interface Member {
  project_id: string;
  user_id: string;
  role_key: ProjectRoleKey | null;
  role_name: string;
  /** First and last name; absent when the API predates it. Never the e-mail. */
  display_name?: string;
  custom_role_id: string | null;
  active: boolean;
  source: string;
}

export interface Label {
  id: string;
  project_id: string | null;
  name: string;
  color: string;
}

/** A tenant user as the people directory returns it (names are optional: only administrators may list users). */
export interface Person {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
}

export interface PublicTask {
  key: string;
  title: string;
  column: string;
  priority: string;
  due_at: string | null;
  labels: string[];
  progress: number;
  description?: string;
}

export interface PublicProject {
  project: { key: string; name: string };
  columns: { name: string; category: string; color: string }[];
  tasks: PublicTask[];
}

/** A role choice is either a built-in role key or a custom role id (see lib/role-picker.ts). */
export type RoleChoice = { role: ProjectRoleKey } | { custom_role_id: string };
