import type { SprintReport } from "../types/reports";

/** Pure helpers for the report screens and the calendar feed. */

export interface LoggedVsEstimate {
  /** Percent of the estimate that is logged (can exceed 100); null when there is no estimate. */
  percent: number | null;
  over: boolean;
}

export function loggedVsEstimate(loggedMinutes: number, estimateMinutes: number): LoggedVsEstimate {
  if (!(estimateMinutes > 0)) return { percent: null, over: false };
  return { percent: Math.round((loggedMinutes / estimateMinutes) * 100), over: loggedMinutes > estimateMinutes };
}

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/** True when from and to are dates, in order, and at most one year apart (the report is meant for a period). */
export function isValidReportRange(from: string, to: string): boolean {
  if (!DATE_RE.test(from) || !DATE_RE.test(to) || to < from) return false;
  const days = (Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / 86_400_000;
  return days <= 366;
}

export interface SprintSummary {
  committed: number;
  completed: number;
  /** Percent of committed tasks that were completed; null when nothing was committed. */
  completionPercent: number | null;
  committedEstimate: number;
  completedEstimate: number;
  movedUnfinished: number;
}

export function summarizeSprintReport(r: SprintReport["report"]): SprintSummary {
  return {
    committed: r.committed_count,
    completed: r.completed_count,
    completionPercent: r.committed_count > 0 ? Math.round((r.completed_count / r.committed_count) * 100) : null,
    committedEstimate: r.committed_estimate_minutes,
    completedEstimate: r.completed_estimate_minutes,
    movedUnfinished: r.moved_unfinished_count,
  };
}

const FEED_PATH_RE = /^\/api\/v1\/calendar\/feed\.ics\?token=[A-Za-z0-9_-]{16,128}$/;

/** Full feed address from the API base and the feed path of a create response; null when either looks wrong. */
export function feedUrl(apiBase: string | null, feedPath: string): string | null {
  if (!apiBase || !FEED_PATH_RE.test(feedPath)) return null;
  return `${apiBase}${feedPath}`;
}
