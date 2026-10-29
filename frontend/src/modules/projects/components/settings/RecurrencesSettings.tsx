"use client";

import { useState, type FormEvent } from "react";
import { createRecurrence, listRecurrences } from "../../api/automation-api";
import { describeError } from "../../api/errors";
import { getTask, isTicketKey } from "../../api/tasks-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { buildRecurrenceInput, type RecurrenceForm } from "../../lib/automation-form";
import type { RecurrenceFrequency } from "../../types/settings";
import { EmptyState } from "../ui/States";
import { RecurrenceRow, WEEKDAYS } from "./RecurrenceRow";
import { SectionFrame } from "./SectionState";

const INITIAL: RecurrenceForm = { templateId: "", frequency: "weekly", interval: "1", weekday: "1", dayOfMonth: "1" };

/** Recurring tasks: a template task is copied daily, weekly or monthly. The template is chosen by its ticket key. */
export function RecurrencesSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const toast = useToast();
  const res = useResource(`recurrences:${projectId}`, () => listRecurrences(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [form, setForm] = useState<RecurrenceForm>(INITIAL);
  const [ticket, setTicket] = useState("");
  const editable = canManage && !denied;
  const items = res.data ?? [];
  const set = (patch: Partial<RecurrenceForm>) => setForm((f) => ({ ...f, ...patch }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    const key = ticket.trim().toUpperCase();
    if (!isTicketKey(key)) return toast.show("Enter the ticket key of the template task, such as ABC-12.", "error");
    let templateId = "";
    try {
      templateId = (await getTask(key)).id;
    } catch (err) {
      return toast.show(describeError(err), "error");
    }
    const built = buildRecurrenceInput({ ...form, templateId });
    if (!built.ok) return toast.show(built.error, "error");
    if (await run(() => createRecurrence(projectId, built.input))) {
      setForm(INITIAL);
      setTicket("");
    }
  }

  return (
    <SectionFrame title="Recurring tasks" id="pm-rec-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      {items.length === 0 && <EmptyState title="No recurring tasks" />}
      <ul className="pm-plain-list">
        {items.map((r) => (
          <RecurrenceRow key={`${r.id}-${r.version}`} projectId={projectId} rec={r} editable={editable} onChanged={res.reload} />
        ))}
      </ul>
      {editable && (
        <form className="pm-inline-form" onSubmit={(e) => void submit(e)} aria-label="New recurring task">
          <label className="pm-sr-only" htmlFor="rec-ticket">
            Template task key
          </label>
          <input id="rec-ticket" value={ticket} onChange={(e) => setTicket(e.target.value)} placeholder="Template task, e.g. ABC-12" maxLength={20} required />
          <label className="pm-sr-only" htmlFor="rec-freq">
            Frequency
          </label>
          <select id="rec-freq" value={form.frequency} onChange={(e) => set({ frequency: e.target.value as RecurrenceFrequency })}>
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
          <button type="submit" className="pm-btn">
            Add recurrence
          </button>
        </form>
      )}
    </SectionFrame>
  );
}
