"use client";

import { formatDay, isOverdue } from "../../lib/dates";
import type { Column } from "../../types/project";
import type { Task } from "../../types/task";
import { PriorityChip, StatusPill } from "../ui/Chips";

interface Props {
  tasks: Task[];
  columns: Column[];
  nowMs: number;
  onOpen: (ticket: string) => void;
}

/** Task table; the whole row's title is a button so the list is fully keyboard operable. */
export function ListTable({ tasks, columns, nowMs, onOpen }: Props) {
  const nameOfColumn = (id: string) => columns.find((c) => c.id === id)?.name ?? "";
  return (
    <div className="pm-table-wrap">
      <table className="pm-table">
        <thead>
          <tr>
            <th scope="col">Key</th>
            <th scope="col">Title</th>
            <th scope="col">Status</th>
            <th scope="col">Priority</th>
            <th scope="col">Assignees</th>
            <th scope="col">Due</th>
          </tr>
        </thead>
        <tbody>
          {tasks.map((t) => {
            const overdue = isOverdue(t.due_at, t.column_category, nowMs);
            return (
              <tr key={t.id}>
                <td className="pm-key">{t.key}</td>
                <td>
                  <button type="button" className="pm-link-btn" onClick={() => onOpen(t.key)}>
                    {t.title}
                  </button>
                  {t.blocked && <StatusPill kind="blocked">Blocked</StatusPill>}
                </td>
                <td>{nameOfColumn(t.column_id)}</td>
                <td>
                  <PriorityChip priority={t.priority} />
                </td>
                <td>{t.assignee_ids.length || ""}</td>
                <td className={overdue ? "pm-due-overdue" : undefined}>{formatDay(t.due_at)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
