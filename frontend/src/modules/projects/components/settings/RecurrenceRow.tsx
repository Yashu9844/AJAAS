"use client";

import { useState } from "react";
import { deleteRecurrence, runRecurrenceNow, setRecurrenceActive, updateRecurrence } from "../../api/automation-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useToast } from "../../hooks/use-toast";
import { buildRecurrenceEdit, recurrenceToForm, type RecurrenceForm } from "../../lib/automation-form";
import { formatDateTime } from "../../lib/dates";
import type { Recurrence, RecurrenceFrequency } from "../../types/settings";

export const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

interface Props {
  projectId: string;
  rec: Recurrence;
  editable: boolean;
  onChanged: () => void;
}

/** One recurring task: schedule summary, pause/resume, run now, edit the schedule, delete. */
export function RecurrenceRow({ projectId, rec, editable, onChanged }: Props) {
  const toast = useToast();
  const { run } = useGuardedAction(onChanged);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<RecurrenceForm>(recurrenceToForm(rec));
  const set = (patch: Partial<RecurrenceForm>) => setForm((f) => ({ ...f, ...patch }));

  function save() {
    const built = buildRecurrenceEdit(form);
    if (!built.ok) return toast.show(built.error, "error");
    void run(() => updateRecurrence(projectId, rec, built.input)).then((ok) => ok && setEditing(false));
  }

  return (
    <li className="pm-card">
      <div className="pm-row-between">
        <span>
          <strong>
            Every {rec.interval > 1 ? `${rec.interval} ` : ""}
            {rec.frequency}
            {rec.frequency === "weekly" && rec.weekday !== null ? ` on ${WEEKDAYS[rec.weekday]}` : ""}
            {rec.frequency === "monthly" && rec.day_of_month !== null ? ` on day ${rec.day_of_month}` : ""}
          </strong>{" "}
          <span className="pm-chip">{rec.active ? "active" : "paused"}</span>
          {rec.flagged && <span className="pm-chip pm-pill-warning">needs attention</span>}
          <span className="pm-muted"> next {formatDateTime(rec.next_run_at)}</span>
        </span>
        {editable && (
          <span className="pm-actions">
            <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing((v) => !v)} aria-expanded={editing}>
              Edit
            </button>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => setRecurrenceActive(projectId, rec, !rec.active))}>
              {rec.active ? "Pause" : "Resume"}
            </button>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => runRecurrenceNow(projectId, rec.id))}>
              Run now
            </button>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteRecurrence(projectId, rec.id))} aria-label="Delete recurrence">
              Delete
            </button>
          </span>
        )}
      </div>
      {editing && (
        <form
          className="pm-inline-form pm-deliveries"
          aria-label="Edit recurrence"
          onSubmit={(e) => {
            e.preventDefault();
            save();
          }}
        >
          <label className="pm-sr-only" htmlFor={`re-f-${rec.id}`}>
            Frequency
          </label>
          <select id={`re-f-${rec.id}`} value={form.frequency} onChange={(e) => set({ frequency: e.target.value as RecurrenceFrequency })}>
            <option value="daily">daily</option>
            <option value="weekly">weekly</option>
            <option value="monthly">monthly</option>
          </select>
          <label className="pm-inline-label">
            every
            <input className="pm-narrow-input" type="number" min={1} max={52} value={form.interval} onChange={(e) => set({ interval: e.target.value })} aria-label="Interval" />
          </label>
          {form.frequency === "weekly" && (
            <select value={form.weekday} onChange={(e) => set({ weekday: e.target.value })} aria-label="Weekday">
              {WEEKDAYS.map((d, i) => (
                <option key={d} value={i}>
                  {d}
                </option>
              ))}
            </select>
          )}
          {form.frequency === "monthly" && (
            <label className="pm-inline-label">
              day
              <input className="pm-narrow-input" type="number" min={1} max={31} value={form.dayOfMonth} onChange={(e) => set({ dayOfMonth: e.target.value })} aria-label="Day of month" />
            </label>
          )}
          <button type="submit" className="pm-btn pm-btn-primary pm-btn-small">
            Save
          </button>
          <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(false)}>
            Cancel
          </button>
        </form>
      )}
    </li>
  );
}
