"use client";

import { listActivity } from "../../api/collab-api";
import { useEnsureNames, usePeople } from "../../hooks/use-people";
import { usePagedList } from "../../hooks/use-paged-list";
import { formatDateTime } from "../../lib/dates";
import { humanize } from "../../lib/format";
import type { ActivityEntry } from "../../types/collab";
import type { Column } from "../../types/project";
import type { Task } from "../../types/task";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

function summarize(entry: ActivityEntry, columns: Column[]): string {
  const to = entry.data.to_column_id;
  if (entry.type === "moved" && typeof to === "string") {
    const name = columns.find((c) => c.id === to)?.name;
    return name ? `moved to ${name}` : "moved";
  }
  const field = entry.data.field;
  const base = humanize(entry.type).toLowerCase();
  return typeof field === "string" ? `${base} (${field})` : base;
}

/** Append-only history, newest first, with "load more". */
export function ActivitySection({ task, columns }: { task: Task; columns: Column[] }) {
  const { nameOf } = usePeople();
  const list = usePagedList<ActivityEntry>(`activity:${task.id}:${task.version}`, (cursor) => listActivity(task.id, cursor));
  useEnsureNames(list.items.map((a) => a.actor_user_id));
  return (
    <section className="pm-section" aria-labelledby={`ac-${task.id}`}>
      <h3 id={`ac-${task.id}`}>Activity</h3>
      {list.loading && <SkeletonRows rows={3} height={22} />}
      {list.error !== undefined && <ErrorNotice error={list.error} onRetry={list.reload} />}
      {!list.loading && list.items.length === 0 && list.error === undefined && <EmptyState title="No activity yet" />}
      <ul className="pm-plain-list">
        {list.items.map((a) => (
          <li key={a.id}>
            <strong>{nameOf(a.actor_user_id)}</strong> {summarize(a, columns)} <span className="pm-muted">{formatDateTime(a.occurred_at)}</span>
          </li>
        ))}
      </ul>
      {list.moreError !== undefined && <ErrorNotice error={list.moreError} />}
      {list.hasMore && (
        <button type="button" className="pm-btn pm-btn-small" onClick={list.loadMore} disabled={list.loadingMore}>
          {list.loadingMore ? "Loading…" : "Load more"}
        </button>
      )}
    </section>
  );
}
