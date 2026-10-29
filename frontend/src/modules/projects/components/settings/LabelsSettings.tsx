"use client";

import type { FormEvent } from "react";
import { createLabel, deleteLabel } from "../../api/projects-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import type { Label } from "../../types/project";
import { EmptyState } from "../ui/States";
import { LabelChip } from "../ui/Chips";

interface Props {
  projectId: string;
  labels: Label[];
  canManage: boolean;
  onChanged: () => void;
}

export function LabelsSettings({ projectId, labels, canManage, onChanged }: Props) {
  const { run, denied } = useGuardedAction(onChanged);
  const editable = canManage && !denied;

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    const name = String(data.get("name") ?? "").trim();
    if (name) void run(() => createLabel(projectId, name, String(data.get("color") ?? "#3B82F6"))).then((ok) => ok && form.reset());
  }

  return (
    <section className="pm-section" aria-labelledby="pm-labels-h">
      <h2 id="pm-labels-h">Labels</h2>
      {labels.length === 0 && <EmptyState title="No labels" />}
      <ul className="pm-plain-list">
        {labels.map((l) => (
          <li key={l.id} className="pm-row-between">
            <LabelChip label={l} />
            {editable && l.project_id !== null && (
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteLabel(projectId, l.id))} aria-label={`Delete label ${l.name}`}>
                Delete
              </button>
            )}
          </li>
        ))}
      </ul>
      {editable && (
        <form className="pm-inline-form" onSubmit={add}>
          <label className="pm-sr-only" htmlFor="lb-name">Label name</label>
          <input id="lb-name" name="name" placeholder="New label" maxLength={40} required />
          <label className="pm-sr-only" htmlFor="lb-color">Color</label>
          <input id="lb-color" name="color" type="color" defaultValue="#3b82f6" />
          <button type="submit" className="pm-btn">
            Add label
          </button>
        </form>
      )}
    </section>
  );
}
