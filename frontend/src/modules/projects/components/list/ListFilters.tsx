"use client";

import { usePeople } from "../../hooks/use-people";
import { humanize } from "../../lib/format";
import { PRIORITIES } from "../../types/common";
import type { Column, Member } from "../../types/project";
import type { TaskListFilters } from "../../types/task";

export const DEFAULT_FILTERS: TaskListFilters = { q: "", column_id: "", priority: "", assignee: "", overdue: false, blocked: false, sort: "rank", dir: "asc" };

const SORTS = ["rank", "created", "updated", "due", "priority", "number"];

interface Props {
  value: TaskListFilters;
  columns: Column[];
  members: Member[];
  onChange: (next: TaskListFilters) => void;
}

/** Filter and sort bar of the list view. Text is debounced by the page. */
export function ListFilters({ value, columns, members, onChange }: Props) {
  const { nameOf } = usePeople();
  const set = <K extends keyof TaskListFilters>(key: K, v: TaskListFilters[K]) => onChange({ ...value, [key]: v });
  return (
    <form className="pm-toolbar" role="search" aria-label="Filter tasks" onSubmit={(e) => e.preventDefault()}>
      <label className="pm-field pm-grow">
        <span className="pm-sr-only">Search text</span>
        <input type="search" value={value.q} onChange={(e) => set("q", e.target.value)} placeholder="Search title or description" maxLength={120} />
      </label>
      <label className="pm-field">
        <span className="pm-sr-only">Column</span>
        <select value={value.column_id} onChange={(e) => set("column_id", e.target.value)}>
          <option value="">All columns</option>
          {columns.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </label>
      <label className="pm-field">
        <span className="pm-sr-only">Priority</span>
        <select value={value.priority} onChange={(e) => set("priority", e.target.value)}>
          <option value="">Any priority</option>
          {PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {humanize(p)}
            </option>
          ))}
        </select>
      </label>
      <label className="pm-field">
        <span className="pm-sr-only">Assignee</span>
        <select value={value.assignee} onChange={(e) => set("assignee", e.target.value)}>
          <option value="">Anyone</option>
          {members.map((m) => (
            <option key={m.user_id} value={m.user_id}>
              {nameOf(m.user_id)}
            </option>
          ))}
        </select>
      </label>
      <label className="pm-check">
        <input type="checkbox" checked={value.overdue} onChange={(e) => set("overdue", e.target.checked)} />
        <span>Overdue</span>
      </label>
      <label className="pm-check">
        <input type="checkbox" checked={value.blocked} onChange={(e) => set("blocked", e.target.checked)} />
        <span>Blocked</span>
      </label>
      <label className="pm-field">
        <span className="pm-sr-only">Sort by</span>
        <select value={value.sort} onChange={(e) => set("sort", e.target.value)}>
          {SORTS.map((s) => (
            <option key={s} value={s}>
              Sort: {humanize(s)}
            </option>
          ))}
        </select>
      </label>
      <button type="button" className="pm-btn" onClick={() => set("dir", value.dir === "asc" ? "desc" : "asc")} aria-label={`Sort direction: ${value.dir === "asc" ? "ascending" : "descending"}`}>
        {value.dir === "asc" ? "↑" : "↓"}
      </button>
    </form>
  );
}
