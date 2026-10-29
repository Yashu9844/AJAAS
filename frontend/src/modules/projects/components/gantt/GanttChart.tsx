"use client";

import { useMemo } from "react";
import { arrowLine, barBox, dayOffset, scaleWidth, todayOffset, weekTicks, type BarBox, type GanttScale } from "../../lib/gantt";
import type { GanttData } from "../../types/planning";

const LABEL_W = 190;
const ROW_H = 32;
const HEAD_H = 28;
const MILESTONE_H = 28;

interface Props {
  data: GanttData;
  scale: GanttScale;
  todayKey: string;
  onOpen: (ticket: string) => void;
}

/** Horizontally scrolling timeline: week ticks, a today line, milestone markers, task bars and "blocks" arrows. */
export function GanttChart({ data, scale, todayKey, onOpen }: Props) {
  const width = scaleWidth(scale);
  const today = todayOffset(scale, todayKey);
  const rows = useMemo(
    () => data.tasks.map((t, row) => ({ task: t, row, box: barBox(scale, t.start_on, t.due_at) })),
    [data.tasks, scale],
  );
  const boxes = useMemo(() => {
    const map = new Map<string, { box: BarBox; row: number }>();
    rows.forEach((r) => r.box && map.set(r.task.id, { box: r.box, row: r.row }));
    return map;
  }, [rows]);
  const arrows = data.dependencies.filter((d) => d.type === "blocks").map((d) => arrowLine(d.source_task_id, d.target_task_id, boxes, ROW_H)).filter((a) => a !== null);
  const bodyHeight = rows.length * ROW_H;

  return (
    <div className="pm-gantt-scroll" tabIndex={0} role="region" aria-label="Gantt timeline, scrolls horizontally">
      <div className="pm-gantt" style={{ width: LABEL_W + width }}>
        <div className="pm-gantt-head" style={{ height: HEAD_H }}>
          <div className="pm-gantt-label pm-muted">Task</div>
          <div className="pm-gantt-track" style={{ width }}>
            {weekTicks(scale).map((t) => (
              <span key={t.key} className="pm-gantt-tick" style={{ left: t.left }}>
                {t.key.slice(5)}
              </span>
            ))}
          </div>
        </div>
        <div className="pm-gantt-row" style={{ height: MILESTONE_H }}>
          <div className="pm-gantt-label pm-muted">Milestones</div>
          <div className="pm-gantt-track" style={{ width }}>
            {data.milestones.map((m) =>
              m.due_on ? (
                <span key={m.id} className="pm-gantt-milestone" style={{ left: dayOffset(scale, m.due_on) + scale.pxPerDay / 2 }} title={`${m.name} (${m.due_on})`}>
                  <span aria-hidden="true">◆</span> {m.name}
                </span>
              ) : null,
            )}
          </div>
        </div>
        <div className="pm-gantt-body" style={{ height: bodyHeight }}>
          {rows.map(({ task, row, box }) => (
            <div key={task.id} className="pm-gantt-row" style={{ height: ROW_H, top: row * ROW_H }}>
              <div className="pm-gantt-label">
                <button type="button" className="pm-link-btn" onClick={() => onOpen(task.ticket)} title={task.title}>
                  <span className="pm-key">{task.ticket}</span> {task.title}
                </button>
              </div>
              <div className="pm-gantt-track" style={{ width }}>
                {box && (
                  <button
                    type="button"
                    className={`pm-gantt-bar pm-gantt-${box.kind} pm-prio-${task.priority}${task.category === "done" ? " pm-gantt-done" : ""}`}
                    style={{ left: box.left, width: box.width }}
                    onClick={() => onOpen(task.ticket)}
                    aria-label={`${task.ticket} ${task.title}, ${task.start_on ?? "no start"} to ${task.due_at?.slice(0, 10) ?? "no due date"}`}
                  />
                )}
              </div>
            </div>
          ))}
          <svg className="pm-gantt-arrows" style={{ left: LABEL_W, width, height: bodyHeight }} aria-hidden="true">
            <defs>
              <marker id="pm-arrow" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
                <path d="M0,0 L8,4 L0,8 z" fill="currentColor" />
              </marker>
            </defs>
            {arrows.map((a) => {
              const mid = Math.max(a.x1 + 6, (a.x1 + a.x2) / 2);
              return <path key={`${a.fromId}-${a.toId}`} d={`M${a.x1},${a.y1} H${mid} V${a.y2} H${a.x2}`} fill="none" stroke="currentColor" strokeWidth="1.5" markerEnd="url(#pm-arrow)" />;
            })}
          </svg>
          {today !== null && <div className="pm-gantt-today" style={{ left: LABEL_W + today, height: bodyHeight }} title="Today" />}
        </div>
      </div>
    </div>
  );
}
