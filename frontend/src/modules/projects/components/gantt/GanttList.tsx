"use client";

import { isoToDateKey } from "../../lib/dates";
import type { GanttData } from "../../types/planning";

/** Accessible text version of the Gantt chart: dates, milestones and "blocks" relations as a table and a list. */
export function GanttList({ data, onOpen }: { data: GanttData; onOpen: (ticket: string) => void }) {
  const ticketOf = new Map(data.tasks.map((t) => [t.id, t.ticket]));
  const blocks = (id: string) =>
    data.dependencies
      .filter((d) => d.type === "blocks" && d.source_task_id === id)
      .map((d) => ticketOf.get(d.target_task_id) ?? "another task")
      .join(", ");
  return (
    <details className="pm-details">
      <summary>Show as a list</summary>
      <div className="pm-table-wrap">
        <table className="pm-table">
          <caption className="pm-sr-only">Dated tasks of the selected window</caption>
          <thead>
            <tr>
              <th scope="col">Key</th>
              <th scope="col">Title</th>
              <th scope="col">Start</th>
              <th scope="col">Due</th>
              <th scope="col">Blocks</th>
            </tr>
          </thead>
          <tbody>
            {data.tasks.map((t) => (
              <tr key={t.id}>
                <td className="pm-key">{t.ticket}</td>
                <td>
                  <button type="button" className="pm-link-btn" onClick={() => onOpen(t.ticket)}>
                    {t.title}
                  </button>
                </td>
                <td>{t.start_on ?? ""}</td>
                <td>{isoToDateKey(t.due_at)}</td>
                <td>{blocks(t.id)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data.milestones.length > 0 && (
        <>
          <h3>Milestones</h3>
          <ul className="pm-plain-list">
            {data.milestones.map((m) => (
              <li key={m.id}>
                {m.name} {m.due_on && <span className="pm-muted">due {m.due_on}</span>} <span className="pm-muted">({m.status})</span>
              </li>
            ))}
          </ul>
        </>
      )}
    </details>
  );
}
