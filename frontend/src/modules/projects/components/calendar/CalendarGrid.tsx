"use client";

import { useMemo } from "react";
import { isoToDateKey, type CalendarCell } from "../../lib/dates";
import type { TaskCard } from "../../types/task";

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const MAX_PER_CELL = 3;

interface Props {
  cells: CalendarCell[];
  tasks: TaskCard[];
  onOpen: (ticket: string) => void;
}

/** A 6-week month grid; each task appears on its due day (or start day when it has no due date). */
export function CalendarGrid({ cells, tasks, onOpen }: Props) {
  const byDay = useMemo(() => {
    const map = new Map<string, TaskCard[]>();
    for (const t of tasks) {
      const key = isoToDateKey(t.due_at) || (t.start_on ?? "");
      if (!key) continue;
      map.set(key, [...(map.get(key) ?? []), t]);
    }
    return map;
  }, [tasks]);

  return (
    <div className="pm-cal-scroll">
      <div className="pm-cal" role="grid" aria-label="Month calendar">
        {WEEKDAYS.map((d) => (
          <div key={d} className="pm-cal-head" role="columnheader">
            {d}
          </div>
        ))}
        {cells.map((cell) => {
          const items = byDay.get(cell.key) ?? [];
          return (
            <div key={cell.key} className={`pm-cal-cell${cell.inMonth ? "" : " pm-cal-out"}`} role="gridcell" aria-label={cell.key}>
              <span className="pm-cal-day">{cell.day}</span>
              {items.slice(0, MAX_PER_CELL).map((t) => (
                <button key={t.id} type="button" className={`pm-cal-task pm-prio-${t.priority}`} onClick={() => onOpen(t.ticket)} title={`${t.ticket} ${t.title}`}>
                  {t.ticket}
                </button>
              ))}
              {items.length > MAX_PER_CELL && <span className="pm-muted pm-cal-more">+{items.length - MAX_PER_CELL} more</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
