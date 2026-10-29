import type { TaskCard } from "./task";

export type SprintStatus = "planned" | "active" | "completed";

export interface Sprint {
  id: string;
  project_id: string;
  name: string;
  goal: string;
  start_on: string | null;
  end_on: string | null;
  status: SprintStatus;
  version: number;
  task_count: number;
}

export type MyWorkGroupKey = "overdue" | "today" | "this_week" | "later" | "no_date";

export interface MyWorkCounts {
  overdue: number;
  today: number;
  this_week: number;
  later: number;
  no_date: number;
  total: number;
  watching: number;
  mentions: number;
}

/** Groups can be null when empty. */
export type MyWorkGroups = Record<MyWorkGroupKey, TaskCard[] | null> & {
  watching: TaskCard[] | null;
  mentions: TaskCard[] | null;
};

export interface MyWork {
  counts: MyWorkCounts;
  groups: MyWorkGroups;
}

export interface SearchHit extends TaskCard {
  match: string;
  score: number;
}

export interface CalendarData {
  from: string;
  to: string;
  tasks: TaskCard[];
  truncated: boolean;
}
