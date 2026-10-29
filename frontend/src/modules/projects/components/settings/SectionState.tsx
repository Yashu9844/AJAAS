"use client";

import type { ReactNode } from "react";
import { isForbidden } from "../../api/errors";
import { ErrorNotice, SkeletonRows } from "../ui/States";

interface Props {
  title: string;
  id: string;
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  children: ReactNode;
}

/** Common frame of a settings section: heading, loading skeleton, error, and a plain note when the API answers 403. */
export function SectionFrame({ title, id, loading, error, onRetry, children }: Props) {
  return (
    <section className="pm-section" aria-labelledby={id}>
      <h2 id={id}>{title}</h2>
      {loading && <SkeletonRows rows={2} height={36} />}
      {isForbidden(error) && <p className="pm-muted">Not available for your role.</p>}
      {error !== undefined && !isForbidden(error) && <ErrorNotice error={error} onRetry={onRetry} />}
      {error === undefined && !loading && children}
    </section>
  );
}
