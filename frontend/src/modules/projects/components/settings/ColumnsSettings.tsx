"use client";

import { useState, type FormEvent } from "react";
import { addColumn, deleteColumn, reorderColumns, updateColumn } from "../../api/projects-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { humanize } from "../../lib/format";
import type { ColumnCategory } from "../../types/common";
import type { Column } from "../../types/project";

const CATEGORIES: ColumnCategory[] = ["backlog", "todo", "in_progress", "done", "canceled"];

interface Props {
  projectId: string;
  columns: Column[];
  canManage: boolean;
  onChanged: () => void;
}

/** Column order, WIP limit, add and delete. Controls disappear when the API answers 403. */
export function ColumnsSettings({ projectId, columns, canManage, onChanged }: Props) {
  const { run, denied } = useGuardedAction(onChanged);
  const [category, setCategory] = useState<ColumnCategory>("in_progress");
  const editable = canManage && !denied;

  function move(index: number, delta: number) {
    const ids = columns.map((c) => c.id);
    const target = index + delta;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target], ids[index]];
    void run(() => reorderColumns(projectId, ids));
  }

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = String(new FormData(form).get("name") ?? "").trim();
    if (name) void run(() => addColumn(projectId, { name, category })).then((ok) => ok && form.reset());
  }

  function setWip(col: Column, raw: string) {
    const n = Number.parseInt(raw, 10);
    if (raw.trim() === "" && col.wip_limit !== null) void run(() => updateColumn(projectId, col.id, { clear_wip_limit: true }));
    else if (Number.isInteger(n) && n > 0 && n !== col.wip_limit) void run(() => updateColumn(projectId, col.id, { wip_limit: n }));
  }

  return (
    <section className="pm-section" aria-labelledby="pm-cols-h">
      <h2 id="pm-cols-h">Columns</h2>
      <ol className="pm-plain-list">
        {columns.map((c, i) => (
          <li key={c.id} className="pm-row-between pm-card">
            <span>
              <strong>{c.name}</strong> <span className="pm-chip">{humanize(c.category)}</span> <span className="pm-muted">{c.task_count} tasks</span>
            </span>
            {editable && (
              <span className="pm-actions">
                <label className="pm-inline-label">
                  WIP
                  <input key={`${c.id}-${c.wip_limit}`} className="pm-narrow-input" type="number" min={1} defaultValue={c.wip_limit ?? ""} onBlur={(e) => setWip(c, e.target.value)} aria-label={`WIP limit for ${c.name}`} />
                </label>
                <button type="button" className="pm-btn pm-btn-small" onClick={() => move(i, -1)} disabled={i === 0} aria-label={`Move ${c.name} left`}>
                  ↑
                </button>
                <button type="button" className="pm-btn pm-btn-small" onClick={() => move(i, 1)} disabled={i === columns.length - 1} aria-label={`Move ${c.name} right`}>
                  ↓
                </button>
                <button
                  type="button"
                  className="pm-btn pm-btn-small"
                  disabled={c.task_count > 0 || c.is_default}
                  title={c.task_count > 0 ? "Only empty columns can be deleted here" : undefined}
                  onClick={() => void run(() => deleteColumn(projectId, c.id))}
                  aria-label={`Delete ${c.name}`}
                >
                  Delete
                </button>
              </span>
            )}
          </li>
        ))}
      </ol>
      {editable && (
        <form className="pm-inline-form" onSubmit={add}>
          <label className="pm-sr-only" htmlFor="col-name">Column name</label>
          <input id="col-name" name="name" placeholder="New column" maxLength={60} required />
          <label className="pm-sr-only" htmlFor="col-cat">Category</label>
          <select id="col-cat" value={category} onChange={(e) => setCategory(e.target.value as ColumnCategory)}>
            {CATEGORIES.map((c) => (
              <option key={c} value={c}>
                {humanize(c)}
              </option>
            ))}
          </select>
          <button type="submit" className="pm-btn">
            Add column
          </button>
        </form>
      )}
    </section>
  );
}
