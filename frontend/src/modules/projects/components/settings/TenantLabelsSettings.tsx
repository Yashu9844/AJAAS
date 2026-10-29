"use client";

import { useState, type FormEvent } from "react";
import { createTenantLabel, deleteTenantLabel, listTenantLabels, updateTenantLabel } from "../../api/settings-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import type { Label } from "../../types/project";
import { LabelChip } from "../ui/Chips";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

/** Organisation-wide labels, usable in every project. Changing them needs a tenant permission: a 403 hides the controls. */
export function TenantLabelsSettings({ onChanged }: { onChanged: () => void }) {
  const res = useResource("tenant-labels", listTenantLabels);
  const { run, denied } = useGuardedAction(() => {
    res.reload();
    onChanged();
  });
  const [editing, setEditing] = useState<string | null>(null);
  const items: Label[] = res.data ?? [];

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    const name = String(data.get("name") ?? "").trim();
    if (name) void run(() => createTenantLabel(name, String(data.get("color") ?? "#3b82f6"))).then((ok) => ok && form.reset());
  }

  return (
    <SectionFrame title="Organisation labels" id="pm-tl-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      <p className="pm-muted">Labels shared by every project of the organisation.</p>
      {items.length === 0 && <EmptyState title="No organisation labels" />}
      <ul className="pm-plain-list">
        {items.map((l) => (
          <li key={l.id} className="pm-row-between">
            {editing === l.id ? (
              <form
                className="pm-inline-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  const data = new FormData(e.currentTarget);
                  void run(() => updateTenantLabel(l.id, String(data.get("name") ?? "").trim(), String(data.get("color") ?? l.color))).then((ok) => ok && setEditing(null));
                }}
              >
                <label className="pm-sr-only" htmlFor={`tln-${l.id}`}>
                  Label name
                </label>
                <input id={`tln-${l.id}`} name="name" defaultValue={l.name} maxLength={40} required />
                <label className="pm-sr-only" htmlFor={`tlc-${l.id}`}>
                  Label colour
                </label>
                <input id={`tlc-${l.id}`} name="color" type="color" defaultValue={/^#[0-9a-fA-F]{6}$/.test(l.color) ? l.color : "#3b82f6"} />
                <button type="submit" className="pm-btn pm-btn-primary pm-btn-small">
                  Save
                </button>
                <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(null)}>
                  Cancel
                </button>
              </form>
            ) : (
              <>
                <LabelChip label={l} />
                {!denied && (
                  <span className="pm-actions">
                    <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(l.id)} aria-label={`Edit label ${l.name}`}>
                      Edit
                    </button>
                    <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteTenantLabel(l.id))} aria-label={`Delete label ${l.name}`}>
                      Delete
                    </button>
                  </span>
                )}
              </>
            )}
          </li>
        ))}
      </ul>
      {!denied && (
        <form className="pm-inline-form" onSubmit={add} aria-label="New organisation label">
          <label className="pm-sr-only" htmlFor="tl-name">
            Label name
          </label>
          <input id="tl-name" name="name" placeholder="New organisation label" maxLength={40} required />
          <label className="pm-sr-only" htmlFor="tl-color">
            Label colour
          </label>
          <input id="tl-color" name="color" type="color" defaultValue="#3b82f6" />
          <button type="submit" className="pm-btn">
            Add organisation label
          </button>
        </form>
      )}
    </SectionFrame>
  );
}
