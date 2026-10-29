"use client";

import type { FormEvent } from "react";
import { createMilestone, deleteMilestone, listMilestones, setMilestoneStatus } from "../../api/settings-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

export function MilestonesSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const res = useResource(`milestones:${projectId}`, () => listMilestones(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const editable = canManage && !denied;
  const items = res.data ?? [];

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    const name = String(data.get("name") ?? "").trim();
    if (name) void run(() => createMilestone(projectId, name, String(data.get("due") ?? ""))).then((ok) => ok && form.reset());
  }

  return (
    <SectionFrame title="Milestones" id="pm-ms-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      {items.length === 0 && <EmptyState title="No milestones" />}
      <ul className="pm-plain-list">
        {items.map((m) => (
          <li key={m.id} className="pm-row-between pm-card">
            <span>
              <strong>{m.name}</strong> <span className="pm-chip">{m.status}</span>{" "}
              <span className="pm-muted">
                {m.due_on ? `due ${m.due_on} · ` : ""}
                {m.done_tasks}/{m.total_tasks} tasks done
              </span>
            </span>
            {editable && (
              <span className="pm-actions">
                <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => setMilestoneStatus(projectId, m.id, m.status === "open" ? "reached" : "open"))}>
                  {m.status === "open" ? "Mark reached" : "Reopen"}
                </button>
                <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteMilestone(projectId, m.id))} aria-label={`Delete milestone ${m.name}`}>
                  Delete
                </button>
              </span>
            )}
          </li>
        ))}
      </ul>
      {editable && (
        <form className="pm-inline-form" onSubmit={add}>
          <label className="pm-sr-only" htmlFor="ms-name">
            Milestone name
          </label>
          <input id="ms-name" name="name" placeholder="New milestone" maxLength={120} required />
          <label className="pm-sr-only" htmlFor="ms-due">
            Due date
          </label>
          <input id="ms-due" name="due" type="date" />
          <button type="submit" className="pm-btn">
            Add milestone
          </button>
        </form>
      )}
    </SectionFrame>
  );
}
