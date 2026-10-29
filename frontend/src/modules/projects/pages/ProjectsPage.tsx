"use client";

import Link from "next/link";
import { useState } from "react";
import { describeError } from "../api/errors";
import { listProjects, setFavorite } from "../api/projects-api";
import { CreateProjectDialog } from "../components/projects/CreateProjectDialog";
import { RoleBadge } from "../components/ui/Chips";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { useDebouncedValue } from "../hooks/use-debounced-value";
import { usePagedList } from "../hooks/use-paged-list";
import { useToast } from "../hooks/use-toast";
import { projectPath } from "../lib/paths";
import type { Project } from "../types/project";

type StatusFilter = "active" | "archived";

/** Project list with search, status filter, favourites, role badge, create dialog and "load more". */
export function ProjectsPage() {
  const toast = useToast();
  const [text, setText] = useState("");
  const [status, setStatus] = useState<StatusFilter>("active");
  const [onlyFavorites, setOnlyFavorites] = useState(false);
  const [creating, setCreating] = useState(false);
  const q = useDebouncedValue(text.trim(), 300);

  const list = usePagedList<Project>(`projects:${q}:${status}:${onlyFavorites}`, (cursor, signal) =>
    listProjects({ q, status, favorite: onlyFavorites || undefined, cursor }, signal),
  );

  async function toggleFavorite(p: Project) {
    const next = !p.favorite;
    list.setItems((items) => items.map((x) => (x.id === p.id ? { ...x, favorite: next } : x)));
    try {
      await setFavorite(p.id, next);
    } catch (e) {
      list.setItems((items) => items.map((x) => (x.id === p.id ? { ...x, favorite: p.favorite } : x)));
      toast.show(describeError(e), "error");
    }
  }

  return (
    <div className="pm-page">
      <div className="pm-page-head">
        <h1>Projects</h1>
        <button type="button" className="pm-btn pm-btn-primary" onClick={() => setCreating(true)}>
          New project
        </button>
      </div>
      <div className="pm-toolbar">
        <label className="pm-field pm-grow">
          <span className="pm-sr-only">Search projects</span>
          <input type="search" value={text} onChange={(e) => setText(e.target.value)} placeholder="Search by key or name" maxLength={120} />
        </label>
        <label className="pm-field">
          <span className="pm-sr-only">Status</span>
          <select value={status} onChange={(e) => setStatus(e.target.value as StatusFilter)}>
            <option value="active">Active</option>
            <option value="archived">Archived</option>
          </select>
        </label>
        <label className="pm-check">
          <input type="checkbox" checked={onlyFavorites} onChange={(e) => setOnlyFavorites(e.target.checked)} />
          <span>Favourites</span>
        </label>
      </div>
      {list.loading && <SkeletonRows rows={5} height={64} />}
      {list.error !== undefined && <ErrorNotice error={list.error} onRetry={list.reload} />}
      {!list.loading && list.error === undefined && list.items.length === 0 && (
        <EmptyState title="No projects found" hint={q || onlyFavorites ? "Try another search or filter." : "Create a project to get started."} />
      )}
      <ul className="pm-project-list">
        {list.items.map((p) => (
          <li key={p.id} className="pm-card pm-project-row">
            <button
              type="button"
              className="pm-star"
              aria-pressed={p.favorite}
              aria-label={p.favorite ? `Remove ${p.name} from favourites` : `Add ${p.name} to favourites`}
              onClick={() => void toggleFavorite(p)}
            >
              {p.favorite ? "★" : "☆"}
            </button>
            <Link href={projectPath(p.id)} className="pm-project-link">
              <span className="pm-key">{p.key}</span>
              <span className="pm-project-name">{p.name}</span>
            </Link>
            <span className="pm-chip">{p.mode}</span>
            <RoleBadge role={p.my_role} />
          </li>
        ))}
      </ul>
      {list.moreError !== undefined && <ErrorNotice error={list.moreError} />}
      {list.hasMore && (
        <button type="button" className="pm-btn" onClick={list.loadMore} disabled={list.loadingMore}>
          {list.loadingMore ? "Loading…" : "Load more"}
        </button>
      )}
      <CreateProjectDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}
