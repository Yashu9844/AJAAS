export interface TimeReportRow {
  /** Task id for a per-task report, user id for a per-user report. */
  key: string;
  number?: number;
  title?: string;
  minutes: number;
  entries: number;
}

export interface TimeReport {
  from: string;
  to: string;
  group_by: "task" | "user";
  total_minutes: number;
  entries: number;
  estimate_minutes: number;
  rows: TimeReportRow[];
}

export interface SprintReport {
  sprint_id: string;
  status: string;
  /** True while the sprint is not completed: the numbers are computed now, not stored. */
  live: boolean;
  report: {
    committed_count: number;
    completed_count: number;
    committed_estimate_minutes: number;
    completed_estimate_minutes: number;
    moved_unfinished_count: number;
  };
}

export interface CalendarToken {
  id: string;
  label: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

/** The create response carries the raw token and the feed path once. */
export interface CreatedCalendarToken extends CalendarToken {
  token: string;
  feed_path: string;
}
