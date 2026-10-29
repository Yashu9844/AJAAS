"use client";

import { formatDay, isOverdue } from "../../lib/dates";
import type { TaskCard } from "../../types/task";
import { PriorityChip, StatusPill } from "../ui/Chips";

interface BoardCardProps {
  card: TaskCard;
  nowMs: number;
  dragging: boolean;
  dropMark: "before" | "after" | null;
  draggable: boolean;
  columns: { id: string; name: string }[];
  onOpen: (ticket: string) => void;
  onDragStart: (taskId: string) => void;
  onDragEnd: () => void;
  onMoveTo: (taskId: string, columnId: string) => void;
}

/** One kanban card: key, title, priority, due date, assignee count and overdue/blocked markers. */
export function BoardCard({ card, nowMs, dragging, dropMark, draggable, columns, onOpen, onDragStart, onDragEnd, onMoveTo }: BoardCardProps) {
  const overdue = isOverdue(card.due_at, card.category, nowMs);
  return (
    <li
      className={`pm-task-card${dragging ? " pm-dragging" : ""}${dropMark ? ` pm-drop-${dropMark}` : ""}`}
      data-card={card.id}
      data-dragging={dragging ? "true" : undefined}
      draggable={draggable}
      onDragStart={(e) => {
        e.dataTransfer.setData("text/plain", card.id);
        e.dataTransfer.effectAllowed = "move";
        onDragStart(card.id);
      }}
      onDragEnd={onDragEnd}
    >
      <button type="button" className="pm-task-open" onClick={() => onOpen(card.ticket)}>
        <span className="pm-key">{card.ticket}</span>
        <span className="pm-task-title">{card.title}</span>
      </button>
      <div className="pm-task-meta">
        <PriorityChip priority={card.priority} />
        {card.due_at && <span className={overdue ? "pm-due pm-due-overdue" : "pm-due"}>{formatDay(card.due_at)}</span>}
        {overdue && <StatusPill kind="overdue">Overdue</StatusPill>}
        {card.blocked && <StatusPill kind="blocked">Blocked</StatusPill>}
        {card.assignee_ids.length > 0 && (
          <span className="pm-count" title={`${card.assignee_ids.length} assignee(s)`}>
            {card.assignee_ids.length} assigned
          </span>
        )}
      </div>
      {draggable && (
        <label className="pm-card-move">
          <span className="pm-sr-only">Move {card.ticket} to column</span>
          <select value={card.column_id} onChange={(e) => onMoveTo(card.id, e.target.value)}>
            {columns.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
      )}
    </li>
  );
}
