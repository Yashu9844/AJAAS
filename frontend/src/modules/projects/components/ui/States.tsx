"use client";

import type { ReactNode } from "react";
import { describeError } from "../../api/errors";

/** A grey placeholder block that reserves space so content does not shift when it arrives. */
export function Skeleton({ height = 16, width = "100%" }: { height?: number; width?: number | string }) {
  return <span className="pm-skeleton" style={{ height, width }} aria-hidden="true" />;
}

export function SkeletonRows({ rows = 5, height = 44 }: { rows?: number; height?: number }) {
  return (
    <div className="pm-skeleton-rows" role="status" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} height={height} />
      ))}
    </div>
  );
}

export function EmptyState({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="pm-empty">
      <p className="pm-empty-title">{title}</p>
      {hint && <p className="pm-muted">{hint}</p>}
      {action}
    </div>
  );
}

export function ErrorNotice({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <div className="pm-error" role="alert">
      <span>{describeError(error)}</span>
      {onRetry && (
        <button type="button" className="pm-btn pm-btn-small" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  );
}
