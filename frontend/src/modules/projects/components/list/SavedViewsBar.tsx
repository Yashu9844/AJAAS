"use client";

import { useState, type FormEvent } from "react";
import { createView, deleteView, listViews } from "../../api/views-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { filtersToView, viewHasUnshownParts, viewToFilters } from "../../lib/saved-views";
import type { TaskListFilters } from "../../types/task";
import type { SavedView } from "../../types/views";

interface Props {
  projectId: string;
  current: TaskListFilters;
  canShare: boolean;
  onApply: (filters: TaskListFilters) => void;
}

/** Saved views of the project: apply, save the current filters (personal, or shared when allowed) and delete. */
export function SavedViewsBar({ projectId, current, canShare, onApply }: Props) {
  const toast = useToast();
  const views = useResource<SavedView[]>(`views:${projectId}`, async () => (await listViews(projectId)).items.filter((v) => v.layout === "list"));
  const { run, denied } = useGuardedAction(views.reload);
  const [selectedId, setSelectedId] = useState("");
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  const selected = (views.data ?? []).find((v) => v.id === selectedId);

  function apply(id: string) {
    setSelectedId(id);
    const view = (views.data ?? []).find((v) => v.id === id);
    if (!view) return;
    onApply(viewToFilters(view));
    if (viewHasUnshownParts(view)) toast.show("This view has more than one value in a filter; only the first is applied here.", "warning");
  }

  function save(e: FormEvent) {
    e.preventDefault();
    const clean = name.trim();
    if (!clean) return;
    const { filters, sort } = filtersToView(current);
    void run(() => createView({ name: clean, layout: "list", project_id: projectId, filters, sort, shared: shared && canShare && !denied })).then((ok) => {
      if (ok) setName("");
    });
  }

  return (
    <div className="pm-toolbar" role="group" aria-label="Saved views">
      <label className="pm-field">
        <span className="pm-sr-only">Saved views</span>
        <select value={selectedId} onChange={(e) => apply(e.target.value)}>
          <option value="">Saved views…</option>
          {(views.data ?? []).map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
              {v.shared ? " (shared)" : ""}
            </option>
          ))}
        </select>
      </label>
      {selected && (
        <button
          type="button"
          className="pm-btn pm-btn-small"
          onClick={() => void run(() => deleteView(selected.id)).then((ok) => ok && setSelectedId(""))}
          aria-label={`Delete view ${selected.name}`}
        >
          Delete view
        </button>
      )}
      <form className="pm-inline-form" onSubmit={save}>
        <label className="pm-sr-only" htmlFor="sv-name">
          Name for the view
        </label>
        <input id="sv-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Save current filters as…" maxLength={80} />
        {canShare && !denied && (
          <label className="pm-check">
            <input type="checkbox" checked={shared} onChange={(e) => setShared(e.target.checked)} />
            <span>Share with project</span>
          </label>
        )}
        <button type="submit" className="pm-btn" disabled={!name.trim()}>
          Save view
        </button>
      </form>
    </div>
  );
}
