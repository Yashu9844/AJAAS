"use client";

import { useState } from "react";
import { listTasks } from "../api/tasks-api";
import { BulkBar } from "../components/list/BulkBar";
import { DEFAULT_FILTERS, ListFilters } from "../components/list/ListFilters";
import { ListTable } from "../components/list/ListTable";
import { SavedViewsBar } from "../components/list/SavedViewsBar";
import { TaskDrawer } from "../components/task/TaskDrawer";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { useDebouncedValue } from "../hooks/use-debounced-value";
import { useNow } from "../hooks/use-now";
import { usePagedList } from "../hooks/use-paged-list";
import { useProject } from "../hooks/use-project";
import { pruneSelection, toggleSelection, togglePage } from "../lib/bulk";
import type { Task, TaskListFilters } from "../types/task";

/** List tab: saved views, filters, sort, cursor paging, multi-select with bulk actions and the task drawer. */
export function ListPage() {
  const { project, columns, members, canWrite, canManage } = useProject();
  const [filters, setFilters] = useState<TaskListFilters>(DEFAULT_FILTERS);
  const [ticket, setTicket] = useState<string | null>(null);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const nowMs = useNow();
  const q = useDebouncedValue(filters.q.trim(), 300);
  const applied: TaskListFilters = { ...filters, q };
  const key = `list:${project.id}:${JSON.stringify(applied)}`;
  const list = usePagedList<Task>(key, (cursor, signal) => listTasks(project.id, applied, cursor, signal));
  const visibleIds = list.items.map((t) => t.id);
  const selectedIds = [...pruneSelection(selected, visibleIds)];

  return (
    <>
      <SavedViewsBar projectId={project.id} current={filters} canShare={canManage} onApply={setFilters} />
      <ListFilters value={filters} columns={columns} members={members} onChange={setFilters} />
      {canWrite && (
        <BulkBar
          projectId={project.id}
          selectedIds={selectedIds}
          tasks={list.items}
          columns={columns}
          onClear={() => setSelected(new Set())}
          onDone={() => {
            setSelected(new Set());
            list.reload();
          }}
        />
      )}
      {list.loading && <SkeletonRows rows={6} height={40} />}
      {list.error !== undefined && <ErrorNotice error={list.error} onRetry={list.reload} />}
      {!list.loading && list.error === undefined && list.items.length === 0 && <EmptyState title="No tasks match" hint="Change the filters or add a task from the board." />}
      {list.items.length > 0 && (
        <ListTable
          tasks={list.items}
          columns={columns}
          nowMs={nowMs}
          onOpen={setTicket}
          selection={canWrite ? { selected: new Set(selectedIds), onToggle: (id) => setSelected(toggleSelection(selected, id)), onTogglePage: () => setSelected(togglePage(selected, visibleIds)) } : undefined}
        />
      )}
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
