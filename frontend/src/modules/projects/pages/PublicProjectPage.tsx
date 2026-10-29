"use client";

import { useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { isApiError } from "../api/errors";
import { fetchPublicProject, type PublicPage } from "../api/public-api";
import { PublicProjectView } from "../components/public/PublicProjectView";
import { SkeletonRows } from "../components/ui/States";
import { useResource } from "../hooks/use-resource";
import { parsePublicTarget } from "../lib/paths";
import type { PublicProject, PublicTask } from "../types/project";

const TOKEN_RE = /^[A-Za-z0-9_-]{16,128}$/;

function NotFound() {
  return (
    <div className="pm-empty" role="alert">
      <p className="pm-empty-title">Not found</p>
      <p className="pm-muted">This page does not exist or is no longer available.</p>
    </div>
  );
}

/** Anonymous read-only project page. It uses no session; every failure that could reveal anything shows "Not found". */
export function PublicProjectPage({ token }: { token: string }) {
  const params = useSearchParams();
  const target = useMemo(() => parsePublicTarget(params), [params]);
  const valid = target !== null && TOKEN_RE.test(token);
  const first = useResource<PublicPage>(valid ? `public:${token}:${target.slug}:${target.host}:${target.port}` : null, (signal) =>
    fetchPublicProject(target ?? { slug: "", host: "", port: "" }, token, undefined, signal),
  );
  const [more, setMore] = useState<{ tasks: PublicTask[]; next: string | null | undefined }>({ tasks: [], next: undefined });
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreFailed, setMoreFailed] = useState(false);

  if (!valid || (first.error !== undefined && isApiError(first.error) && first.error.status === 404)) return <NotFound />;
  if (first.error !== undefined) {
    return (
      <p className="pm-error" role="alert">
        This page is temporarily unavailable. Try again later.
      </p>
    );
  }
  if (!first.data) return <SkeletonRows rows={4} height={64} />;

  const head = first.data;
  const next = more.next === undefined ? head.tasks.next : more.next;
  const data: PublicProject = { project: head.head.project, columns: head.head.columns, tasks: [...head.tasks.items, ...more.tasks] };

  async function loadMore() {
    if (!next || !target) return;
    setLoadingMore(true);
    setMoreFailed(false);
    try {
      const page = await fetchPublicProject(target, token, next, new AbortController().signal);
      setMore((m) => ({ tasks: [...m.tasks, ...page.tasks.items.filter((t) => !m.tasks.some((x) => x.key === t.key))], next: page.tasks.next }));
    } catch {
      setMoreFailed(true);
    } finally {
      setLoadingMore(false);
    }
  }

  return (
    <div className="pm-page">
      <div className="pm-page-head">
        <h1>
          <span className="pm-key">{data.project.key}</span> {data.project.name}
        </h1>
        <span className="pm-chip">Read-only public view</span>
      </div>
      <PublicProjectView data={data} />
      {moreFailed && (
        <p className="pm-error" role="alert">
          More tasks could not be loaded.
        </p>
      )}
      {next && (
        <button type="button" className="pm-btn" onClick={() => void loadMore()} disabled={loadingMore}>
          {loadingMore ? "Loading…" : "Load more tasks"}
        </button>
      )}
    </div>
  );
}
