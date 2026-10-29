"use client";

import { useState, type FormEvent } from "react";
import { deleteTimeEntry, getRunningTimer, listTimeEntries, logTime, startTimer, stopTimer } from "../../api/collab-api";
import { describeError } from "../../api/errors";
import { usePeople } from "../../hooks/use-people";
import { useNow } from "../../hooks/use-now";
import { useResource } from "../../hooks/use-resource";
import { useSession } from "../../hooks/use-session";
import { useToast } from "../../hooks/use-toast";
import { formatDateTime } from "../../lib/dates";
import { formatMinutes, parseMinutes } from "../../lib/format";
import type { Task } from "../../types/task";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

export function TimeSection({ task, canLog, enabled }: { task: Task; canLog: boolean; enabled: boolean }) {
  const toast = useToast();
  const session = useSession();
  const { nameOf } = usePeople();
  const now = useNow();
  const [duration, setDuration] = useState("");
  const [note, setNote] = useState("");
  const entries = useResource(`time:${task.id}`, () => listTimeEntries(task.id));
  const timer = useResource(`timer:${session.userId}`, () => getRunningTimer());

  async function run(action: () => Promise<unknown>, after: () => void) {
    try {
      await action();
      after();
    } catch (e) {
      toast.show(describeError(e), "error");
    }
  }

  function refresh() {
    entries.reload();
    timer.reload();
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    const minutes = parseMinutes(duration);
    if (minutes === null || minutes < 1 || minutes > 1440) return toast.show("Enter 1 minute to 24 hours, for example 45m or 1h 30m.", "error");
    void run(() => logTime(task.id, minutes, note.trim()), () => {
      setDuration("");
      setNote("");
      refresh();
    });
  }

  const running = timer.data ?? null;
  const runningHere = running !== null && running.task_id === task.id;
  const elapsed = running ? Math.max(0, Math.floor((now - new Date(running.started_at).getTime()) / 60000)) : 0;
  const rows = entries.data?.items ?? [];
  const total = rows.reduce((sum, r) => sum + r.minutes, 0);

  return (
    <section className="pm-section" aria-labelledby={`tm-${task.id}`}>
      <h2 id={`tm-${task.id}`}>
        Time <span className="pm-muted">{formatMinutes(total)} logged{task.estimate_minutes ? ` of ${formatMinutes(task.estimate_minutes)}` : ""}</span>
      </h2>
      {!enabled && <p className="pm-muted">Time tracking is turned off for this project.</p>}
      {enabled && canLog && (
        <>
          <div className="pm-actions">
            {runningHere ? (
              <button type="button" className="pm-btn pm-btn-primary" onClick={() => void run(stopTimer, refresh)}>
                Stop timer ({formatMinutes(elapsed)})
              </button>
            ) : (
              <button type="button" className="pm-btn" disabled={running !== null} onClick={() => void run(() => startTimer(task.id), refresh)}>
                Start timer
              </button>
            )}
            {running && !runningHere && <span className="pm-muted">A timer is running on another task.</span>}
          </div>
          <form className="pm-inline-form" onSubmit={submit}>
            <label className="pm-sr-only" htmlFor={`td-${task.id}`}>
              Time spent
            </label>
            <input id={`td-${task.id}`} value={duration} onChange={(e) => setDuration(e.target.value)} placeholder="1h 30m" maxLength={12} />
            <label className="pm-sr-only" htmlFor={`tn-${task.id}`}>
              Note
            </label>
            <input id={`tn-${task.id}`} value={note} onChange={(e) => setNote(e.target.value)} placeholder="Note (optional)" maxLength={500} />
            <button type="submit" className="pm-btn" disabled={!duration.trim()}>
              Log time
            </button>
          </form>
        </>
      )}
      {entries.loading && <SkeletonRows rows={2} height={24} />}
      {entries.error !== undefined && <ErrorNotice error={entries.error} onRetry={entries.reload} />}
      {entries.data && rows.length === 0 && <EmptyState title="No time logged" />}
      <ul className="pm-plain-list">
        {rows.map((r) => (
          <li key={r.id} className="pm-row-between">
            <span>
              {formatMinutes(r.minutes)} · {nameOf(r.user_id)} · {formatDateTime(r.started_at)}
              {r.ended_at === null && " · running"}
              {r.note && <span className="pm-muted"> · {r.note}</span>}
            </span>
            {r.user_id === session.userId && (
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteTimeEntry(r.id), refresh)} aria-label="Delete time entry">
                Delete
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
