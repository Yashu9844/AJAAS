"use client";

import { useState } from "react";
import { listTasks } from "../api/tasks-api";
import { DEFAULT_FILTERS, ListFilters } from "../components/list/ListFilters";
import { ListTable } from "../components/list/ListTable";
import { TaskDrawer } from "../components/task/TaskDrawer";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { useDebouncedValue } from "../hooks/use-debounced-value";
import { useNow } from "../hooks/use-now";
import { usePagedList } from "../hooks/use-paged-list";
import { useProject } from "../hooks/use-project";
import type { Task, TaskListFilters } from "../types/task";

/** List tab: filters, sort and cursor paging over the project's top-level tasks. */
export function ListPage() {
  const { project, columns, members } = useProject();
  const [filters, setFilters] = useState<TaskListFilters>(DEFAULT_FILTERS);
  const [ticket, setTicket] = useState<string | null>(null);
  const nowMs = useNow();
  const q = useDebouncedValue(filters.q.trim(), 300);
  const applied: TaskListFilters = { ...filters, q };
  const key = `list:${project.id}:${JSON.stringify(applied)}`;
  const list = usePagedList<Task>(key, (cursor, signal) => listTasks(project.id, applied, cursor, signal));

  return (
    <>
      <ListFilters value={filters} columns={columns} members={members} onChange={setFilters} />
      {list.loading && <SkeletonRows rows={6} height={40} />}
      {list.error !== undefined && <ErrorNotice error={list.error} onRetry={list.reload} />}
      {!list.loading && list.error === undefined && list.items.length === 0 && <EmptyState title="No tasks match" hint="Change the filters or add a task from the board." />}
      {list.items.length > 0 && <ListTable tasks={list.items} columns={columns} nowMs={nowMs} onOpen={setTicket} />}
      {list.moreError !== undefined && <ErrorNotice error={list.moreError} />}
      {list.hasMore && (
        <button type="button" className="pm-btn" onClick={list.loadMore} disabled={list.loadingMore}>
          {list.loadingMore ? "Loading…" : "Load more"}
        </button>
      )}
      <TaskDrawer ticket={ticket} onClose={() => setTicket(null)} onChanged={list.reload} />
    </>
  );
}
