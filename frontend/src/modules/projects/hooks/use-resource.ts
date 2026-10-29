"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export interface Resource<T> {
  data: T | undefined;
  error: unknown;
  /** True until the first result for the current key has arrived (a reload keeps showing the old data). */
  loading: boolean;
  reload: () => void;
  /** Replaces the cached data, for optimistic updates. */
  setData: (next: T | ((prev: T | undefined) => T | undefined)) => void;
}

interface Settled<T> {
  key: string;
  token: string;
  data: T | undefined;
  error: unknown;
}

function isAbort(e: unknown): boolean {
  return e instanceof DOMException && e.name === "AbortError";
}

/**
 * Loads one value whenever `key` changes (pass null to stay idle). The loader is read through a ref so callers can pass
 * an inline function without retriggering the load; only the key decides when to fetch.
 */
export function useResource<T>(key: string | null, load: (signal: AbortSignal) => Promise<T>): Resource<T> {
  const [tick, setTick] = useState(0);
  const [settled, setSettled] = useState<Settled<T> | null>(null);
  const loaderRef = useRef(load);
  useEffect(() => {
    loaderRef.current = load;
  });
  const token = `${key}#${tick}`;

  useEffect(() => {
    if (key === null) return;
    const controller = new AbortController();
    loaderRef.current(controller.signal).then(
      (data) => setSettled({ key, token, data, error: undefined }),
      (error: unknown) => {
        if (isAbort(error)) return;
        setSettled((prev) => ({ key, token, data: prev && prev.key === key ? prev.data : undefined, error }));
      },
    );
    return () => controller.abort();
  }, [key, token]);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  const setData = useCallback(
    (next: T | ((prev: T | undefined) => T | undefined)) => {
      setSettled((prev) => {
        const base = prev && prev.key === key ? prev : { key: key ?? "", token: "", data: undefined, error: undefined };
        const value = typeof next === "function" ? (next as (p: T | undefined) => T | undefined)(base.data) : next;
        return { ...base, data: value, error: undefined };
      });
    },
    [key],
  );

  const current = settled && settled.key === key ? settled : null;
  return {
    data: current?.data,
    error: current?.error,
    loading: key !== null && current === null,
    reload,
    setData,
  };
}
