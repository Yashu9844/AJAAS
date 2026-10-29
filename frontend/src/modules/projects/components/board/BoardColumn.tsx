"use client";

import { useState, type DragEvent, type FormEvent } from "react";
import { insertionIndexFromPointer } from "../../lib/rank-neighbours";
import { isOverWipLimit } from "../../lib/optimistic";
import type { BoardColumn as BoardColumnData } from "../../types/task";
import { StatusPill } from "../ui/Chips";
import { BoardCard } from "./BoardCard";

interface BoardColumnProps {
  column: BoardColumnData;
  allColumns: { id: string; name: string }[];
  nowMs: number;
  canWrite: boolean;
  dragId: string | null;
  onOpen: (ticket: string) => void;
  onDragStart: (taskId: string) => void;
  onDragEnd: () => void;
  onDrop: (taskId: string, columnId: string, index: number) => void;
  onLoadMore: (columnId: string) => void;
  onQuickAdd: (columnId: string, title: string) => Promise<void>;
}

/** Midpoints of the visible cards in a column body, the dragged card excluded. */
function cardMidpoints(body: HTMLElement): number[] {
  const cards = body.querySelectorAll<HTMLElement>("[data-card]:not([data-dragging])");
  return Array.from(cards, (el) => {
    const r = el.getBoundingClientRect();
    return r.top + r.height / 2;
  });
}

/** Which side of a card shows the drop indicator; computed without changing layout so the pointer maths stays stable. */
function dropMarkFor(tasks: { id: string }[], dragId: string | null, hoverIndex: number | null, cardId: string): "before" | "after" | null {
  if (hoverIndex === null || cardId === dragId) return null;
  const others = tasks.filter((t) => t.id !== dragId);
  const at = others.findIndex((t) => t.id === cardId);
  if (at === hoverIndex) return "before";
  if (at === others.length - 1 && hoverIndex >= others.length) return "after";
  return null;
}

export function BoardColumn(props: BoardColumnProps) {
  const { column, dragId, canWrite } = props;
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const [title, setTitle] = useState("");
  const [adding, setAdding] = useState(false);
  const over = isOverWipLimit(column);

  function handleOver(e: DragEvent<HTMLElement>) {
    if (!dragId || !canWrite) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setHoverIndex(insertionIndexFromPointer(cardMidpoints(e.currentTarget), e.clientY));
  }

  function handleDrop(e: DragEvent<HTMLElement>) {
    e.preventDefault();
    const id = dragId ?? e.dataTransfer.getData("text/plain");
    const index = insertionIndexFromPointer(cardMidpoints(e.currentTarget), e.clientY);
    setHoverIndex(null);
    if (id) props.onDrop(id, column.id, index);
  }

  async function submitQuickAdd(e: FormEvent) {
    e.preventDefault();
    const t = title.trim();
    if (!t) return;
    setAdding(true);
    try {
      await props.onQuickAdd(column.id, t);
      setTitle("");
    } catch {
      // The page already told the person why; the typed title stays so it can be retried.
    } finally {
      setAdding(false);
    }
  }

  return (
    <section className={`pm-column${hoverIndex !== null ? " pm-column-over" : ""}`} aria-label={`${column.name}, ${column.count} tasks`}>
      <header className="pm-column-head">
        <span className="pm-dot" style={{ background: /^#[0-9a-fA-F]{3,8}$/.test(column.color) ? column.color : "#94a3b8" }} aria-hidden="true" />
        <h3>{column.name}</h3>
        <span className="pm-count-badge">{column.wip_limit !== null ? `${column.count}/${column.wip_limit}` : column.count}</span>
        {over && <StatusPill kind="warning">WIP limit</StatusPill>}
      </header>
      <ul className="pm-column-body" onDragOver={handleOver} onDragLeave={() => setHoverIndex(null)} onDrop={handleDrop}>
        {column.tasks.map((card) => (
          <BoardCard
            key={card.id}
            card={card}
            nowMs={props.nowMs}
            dragging={dragId === card.id}
            draggable={canWrite}
            columns={props.allColumns}
            onOpen={props.onOpen}
            onDragStart={props.onDragStart}
            onDragEnd={() => {
              setHoverIndex(null);
              props.onDragEnd();
            }}
            onMoveTo={(id, to) => props.onDrop(id, to, Number.MAX_SAFE_INTEGER)}
            dropMark={dropMarkFor(column.tasks, dragId, hoverIndex, card.id)}
          />
        ))}
        {column.tasks.length === 0 && <li className="pm-column-empty pm-muted">No tasks</li>}
      </ul>
      {column.next_cursor && (
        <button type="button" className="pm-btn pm-btn-small pm-column-more" onClick={() => props.onLoadMore(column.id)}>
          Load more
        </button>
      )}
      {canWrite && (
        <form className="pm-quick-add" onSubmit={submitQuickAdd}>
          <label className="pm-sr-only" htmlFor={`qa-${column.id}`}>
            Add a task to {column.name}
          </label>
          <input id={`qa-${column.id}`} value={title} onChange={(e) => setTitle(e.target.value)} placeholder="+ Add task" maxLength={300} disabled={adding} />
        </form>
      )}
    </section>
  );
}
