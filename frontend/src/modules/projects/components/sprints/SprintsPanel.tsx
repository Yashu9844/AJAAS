"use client";

import { useState, type FormEvent } from "react";
import { completeSprint, createSprint, startSprint } from "../../api/planning-api";
import { describeError } from "../../api/errors";
import { useToast } from "../../hooks/use-toast";
import type { Sprint } from "../../types/planning";
import { EmptyState } from "../ui/States";

interface Props {
  projectId: string;
  sprints: Sprint[];
  canManage: boolean;
  onChanged: () => void;
}

/** Sprint list with create, start and complete (unfinished tasks go to the backlog or the next planned sprint). */
export function SprintsPanel({ projectId, sprints, canManage, onChanged }: Props) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    try {
      await action();
      onChanged();
    } catch (e) {
      toast.show(describeError(e), "error");
    } finally {
      setBusy(false);
    }
  }

  function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    void run(async () => {
      await createSprint(projectId, String(data.get("name") ?? "").trim(), String(data.get("start") ?? ""), String(data.get("end") ?? ""));
      form.reset();
    });
  }

  const nextPlanned = (current: Sprint) => sprints.find((s) => s.status === "planned" && s.id !== current.id);
  return (
    <section className="pm-section" aria-labelledby="pm-sprints-h">
      <h2 id="pm-sprints-h">Sprints</h2>
      {sprints.length === 0 && <EmptyState title="No sprints yet" hint={canManage ? "Create the first sprint below." : undefined} />}
      <ul className="pm-plain-list">
        {sprints.map((s) => (
          <li key={s.id} className="pm-row-between pm-card">
            <span>
              <strong>{s.name}</strong> <span className={`pm-chip pm-sprint-${s.status}`}>{s.status}</span>
              <span className="pm-muted"> {s.start_on ?? "?"} to {s.end_on ?? "?"} · {s.task_count} tasks</span>
            </span>
            {canManage && s.status === "planned" && (
              <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => void run(() => startSprint(s.id))}>
                Start
              </button>
            )}
            {canManage && s.status === "active" && (
              <span className="pm-actions">
                <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => void run(() => completeSprint(s.id, "backlog"))}>
                  Complete (unfinished to backlog)
                </button>
                {nextPlanned(s) && (
                  <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => void run(() => completeSprint(s.id, nextPlanned(s)?.id ?? "backlog"))}>
                    Complete (unfinished to next sprint)
                  </button>
                )}
              </span>
            )}
          </li>
        ))}
      </ul>
      {canManage && (
        <form className="pm-inline-form" onSubmit={create}>
          <label className="pm-sr-only" htmlFor="sp-name">Sprint name</label>
          <input id="sp-name" name="name" placeholder="Sprint name" required maxLength={120} />
          <label className="pm-sr-only" htmlFor="sp-start">Start date</label>
          <input id="sp-start" name="start" type="date" required />
          <label className="pm-sr-only" htmlFor="sp-end">End date</label>
          <input id="sp-end" name="end" type="date" required />
          <button type="submit" className="pm-btn" disabled={busy}>
            Create sprint
          </button>
        </form>
      )}
    </section>
  );
}
