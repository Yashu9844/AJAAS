"use client";

import { useMemo, useState } from "react";
import { GanttChart } from "../components/gantt/GanttChart";
import { GanttList } from "../components/gantt/GanttList";
import { TaskDrawer } from "../components/task/TaskDrawer";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { useGantt } from "../hooks/use-gantt";
import { useProject } from "../hooks/use-project";
import { toDateKey } from "../lib/dates";
import { makeScale, shiftWindow, windowAround } from "../lib/gantt";

const WINDOW_DAYS = 90;
const SHIFT_DAYS = 30;
const PX_PER_DAY = 28;

/** Gantt tab: dated tasks, milestones and "blocks" arrows for a 90-day window that can be moved, with "load more". */
export function GanttPage() {
  const { project } = useProject();
  const [todayKey] = useState(() => toDateKey(new Date()));
  const [window90, setWindow] = useState(() => windowAround(toDateKey(new Date()), 14, WINDOW_DAYS));
  const [ticket, setTicket] = useState<string | null>(null);
  const gantt = useGantt(project.id, window90.from, window90.to);
  const scale = useMemo(() => makeScale(window90.from, window90.to, PX_PER_DAY), [window90]);
  const data = gantt.data;

  return (
    <>
      <h2 className="pm-sr-only">Gantt chart</h2>
      <div className="pm-toolbar">
        <button type="button" className="pm-btn" onClick={() => setWindow((w) => shiftWindow(w, -SHIFT_DAYS))} aria-label="Earlier">
          ‹ Earlier
        </button>
        <span className="pm-muted" aria-live="polite">
          {window90.from} to {window90.to}
        </span>
        <button type="button" className="pm-btn" onClick={() => setWindow((w) => shiftWindow(w, SHIFT_DAYS))} aria-label="Later">
          Later ›
        </button>
        <button type="button" className="pm-btn" onClick={() => setWindow(windowAround(todayKey, 14, WINDOW_DAYS))}>
          Today
        </button>
      </div>
      {gantt.loading && <SkeletonRows rows={5} height={32} />}
      {gantt.error !== undefined && !data && <ErrorNotice error={gantt.error} onRetry={gantt.reload} />}
      {data && data.tasks.length === 0 && data.milestones.length === 0 && <EmptyState title="No dated tasks in this window" hint="Give tasks a due date (and a start date for a bar), or move the window." />}
      {data && (data.tasks.length > 0 || data.milestones.length > 0) && (
        <>
          <GanttChart data={data} scale={scale} todayKey={todayKey} onOpen={setTicket} />
          <GanttList data={data} onOpen={setTicket} />
        </>
      )}
      {gantt.moreError !== undefined && <ErrorNotice error={gantt.moreError} />}
      {data?.next_cursor && (
        <button type="button" className="pm-btn" onClick={gantt.loadMore} disabled={gantt.loadingMore}>
          {gantt.loadingMore ? "Loading…" : "Load more tasks"}
        </button>
      )}
      <TaskDrawer ticket={ticket} onClose={() => setTicket(null)} onChanged={gantt.reload} />
    </>
  );
}
