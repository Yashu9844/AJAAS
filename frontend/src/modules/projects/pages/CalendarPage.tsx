"use client";

import { useMemo, useState } from "react";
import { getCalendar } from "../api/planning-api";
import { CalendarGrid } from "../components/calendar/CalendarGrid";
import { TaskDrawer } from "../components/task/TaskDrawer";
import { ErrorNotice, SkeletonRows } from "../components/ui/States";
import { useProject } from "../hooks/use-project";
import { useResource } from "../hooks/use-resource";
import { gridWindow, monthGrid, shiftMonth } from "../lib/dates";

/** Calendar tab: a month grid fed by GET /projects/{id}/calendar (window of 42 days). */
export function CalendarPage() {
  const { project } = useProject();
  const [cursor, setCursor] = useState(() => {
    const d = new Date();
    return { year: d.getFullYear(), month: d.getMonth() };
  });
  const [ticket, setTicket] = useState<string | null>(null);
  const cells = useMemo(() => monthGrid(cursor.year, cursor.month), [cursor]);
  const { from, to } = gridWindow(cells);
  const res = useResource(`calendar:${project.id}:${from}:${to}`, () => getCalendar(project.id, from, to));
  const title = new Date(cursor.year, cursor.month, 1).toLocaleDateString(undefined, { month: "long", year: "numeric" });

  return (
    <>
      <div className="pm-toolbar">
        <button type="button" className="pm-btn" onClick={() => setCursor((c) => shiftMonth(c.year, c.month, -1))} aria-label="Previous month">
          ‹
        </button>
        <h2 className="pm-month-title" aria-live="polite">
          {title}
        </h2>
        <button type="button" className="pm-btn" onClick={() => setCursor((c) => shiftMonth(c.year, c.month, 1))} aria-label="Next month">
          ›
        </button>
      </div>
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.loading ? <SkeletonRows rows={4} height={90} /> : <CalendarGrid cells={cells} tasks={res.data?.tasks ?? []} onOpen={setTicket} />}
      {res.data?.truncated && <p className="pm-muted">Only the first tasks of this window are shown.</p>}
      <TaskDrawer ticket={ticket} onClose={() => setTicket(null)} onChanged={res.reload} />
    </>
  );
}
