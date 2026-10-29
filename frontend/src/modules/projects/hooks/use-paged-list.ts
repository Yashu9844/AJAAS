"use client";

import { useCallback, useState } from "react";
import type { Page } from "../types/common";
import { useResource } from "./use-resource";

export interface PagedList<T> {
  items: T[];
  loading: boolean;
  loadingMore: boolean;
  error: unknown;
  moreError: unknown;
  hasMore: boolean;
  loadMore: () => void;
  reload: () => void;
  setItems: (next: T[] | ((prev: T[]) => T[])) => void;
}

interface Loaded<T> {
  items: T[];
  next: string | null;
}

/**
 * A cursor-paged list: the first page loads when `key` changes, "load more" appends the next page.
 * `fetchPage(cursor)` receives undefined for the first page.
 */
export function usePagedList<T>(key: string | null, fetchPage: (cursor: string | undefined, signal: AbortSignal) => Promise<Page<T>>): PagedList<T> {
  const first = useResource<Loaded<T>>(key, (signal) => fetchPage(undefined, signal));
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<unknown>(undefined);
  const { data, setData, reload: reloadFirst } = first;

  const loadMore = useCallback(() => {
    if (!data?.next || loadingMore) return;
    setLoadingMore(true);
    setMoreError(undefined);
    fetchPage(data.next, new AbortController().signal)
      .then((page) => setData((prev) => (prev ? { items: [...prev.items, ...page.items], next: page.next } : prev)))
      .catch((e: unknown) => setMoreError(e))
      .finally(() => setLoadingMore(false));
  }, [data, loadingMore, fetchPage, setData]);

  const setItems = useCallback(
    (next: T[] | ((prev: T[]) => T[])) => {
      setData((prev) => {
        const base = prev?.items ?? [];
        return { items: typeof next === "function" ? next(base) : next, next: prev?.next ?? null };
      });
    },
    [setData],
  );

  return {
    items: data?.items ?? [],
    loading: first.loading,
    loadingMore,
    error: first.error,
    moreError,
    hasMore: Boolean(data?.next),
    loadMore,
    reload: reloadFirst,
    setItems,
  };
}
