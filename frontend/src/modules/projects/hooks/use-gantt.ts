"use client";

import { useCallback, useState } from "react";
import { getGantt } from "../api/planning-api";
import { mergeGanttPage } from "../lib/gantt";
import type { GanttData } from "../types/planning";
import { useResource } from "./use-resource";

export interface GanttState {
  data: GanttData | undefined;
  loading: boolean;
  loadingMore: boolean;
  error: unknown;
  moreError: unknown;
  reload: () => void;
  loadMore: () => void;
}

/** Loads the Gantt window and appends further pages on demand (the cursor travels inside the data). */
export function useGantt(projectId: string, from: string, to: string): GanttState {
  const res = useResource<GanttData>(`gantt:${projectId}:${from}:${to}`, async () => {
    const d = await getGantt(projectId, from, to);
    return { ...d, tasks: d.tasks ?? [], milestones: d.milestones ?? [], dependencies: d.dependencies ?? [] };
  });
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<unknown>(undefined);
  const { data, setData } = res;

  const loadMore = useCallback(() => {
    if (!data?.next_cursor || loadingMore) return;
    setLoadingMore(true);
    setMoreError(undefined);
    getGantt(projectId, from, to, data.next_cursor)
      .then((page) => setData((prev) => (prev ? mergeGanttPage(prev, { ...page, tasks: page.tasks ?? [], milestones: page.milestones ?? [], dependencies: page.dependencies ?? [] }) : prev)))
      .catch((e: unknown) => setMoreError(e))
      .finally(() => setLoadingMore(false));
  }, [data, loadingMore, projectId, from, to, setData]);

  return { data, loading: res.loading, loadingMore, error: res.error, moreError, reload: res.reload, loadMore };
}
