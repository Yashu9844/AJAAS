import type { ColumnCategory, Priority } from "./common";

/** The light task shape used by boards, calendars, My work and search. */
export interface TaskCard {
  id: string;
  project_id: string;
  project_key: string;
  number: number;
  ticket: string;
  title: string;
  column_id: string;
  category: ColumnCategory;
  priority: Priority;
  start_on: string | null;
  due_at: string | null;
  estimate_minutes: number | null;
  milestone_id: string | null;
  sprint_id: string | null;
  rank: string;
  completed_at: string | null;
  version: number;
  updated_at: string;
  assignee_ids: string[];
  label_ids: string[];
  blocked?: boolean;
}

/** The full task as GET /tasks/{id} and the task list return it. */
export interface Task {
  id: string;
  project_id: string;
  number: number;
  key: string;
  parent_task_id: string | null;
  depth: number;
  title: string;
  description: string;
  column_id: string;
  column_category: ColumnCategory;
  priority: Priority;
  reporter_user_id: string;
  start_on: string | null;
  due_at: string | null;
  estimate_minutes: number | null;
  milestone_id: string | null;
  sprint_id: string | null;
  rank: string;
  completed_at: string | null;
  archived_at: string | null;
  version: number;
  created_at: string;
  updated_at: string;
  blocked: boolean;
  overdue: boolean;
  subtask_total: number;
  subtask_done: number;
  progress: number;
  assignee_ids: string[];
  label_ids: string[];
  /** Soft WIP warning on create and move responses. */
  warnings?: string[];
}

export interface TaskUpdate {
  title?: string;
  description?: string;
  priority?: Priority;
  start_on?: string | null;
  due_at?: string | null;
  estimate_minutes?: number | null;
}

export interface TaskCreateInput {
  title: string;
  column_id?: string;
  priority?: Priority;
  description?: string;
  parent_task_id?: string;
}

export interface MoveInput {
  column_id: string;
  before_id?: string;
  after_id?: string;
  version: number;
}

export interface BoardColumn {
  id: string;
  name: string;
  key: string;
  position: number;
  color: string;
  category: ColumnCategory;
  wip_limit: number | null;
  wip_hard: boolean;
  count: number;
  tasks: TaskCard[];
  next_cursor: string | null;
}

export interface Board {
  project_id: string;
  columns: BoardColumn[];
}

export interface Subtask {
  id: string;
  key?: string;
  ticket?: string;
  title: string;
  column_id: string;
  category?: ColumnCategory;
  version: number;
}

export type RelationType = "blocks" | "related" | "duplicates";

export interface Relation {
  id: string;
  type: RelationType;
  direction: "outgoing" | "incoming";
  other_task_id: string;
  other_key: string;
  other_title: string;
  other_done: boolean;
}

export type CustomFieldType = "text" | "number" | "checkbox" | "date" | "url" | "select" | "multiselect" | "user";

export interface CustomFieldDefinition {
  id: string;
  key: string;
  name: string;
  type: CustomFieldType;
  options?: string[];
}

export interface CustomFieldValue {
  field_id: string;
  key: string;
  name: string;
  type: CustomFieldType;
  value: string | number | boolean | string[] | null;
}

export interface TaskListFilters {
  q: string;
  column_id: string;
  priority: string;
  assignee: string;
  overdue: boolean;
  blocked: boolean;
  sort: string;
  dir: "asc" | "desc";
}
