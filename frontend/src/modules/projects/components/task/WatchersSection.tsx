"use client";

import { getWatchState, listWatchers, unwatchTask, watchTask } from "../../api/collab-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { usePeople } from "../../hooks/use-people";
import { useResource } from "../../hooks/use-resource";
import { useSession } from "../../hooks/use-session";
import { humanize } from "../../lib/format";
import type { Task } from "../../types/task";
import { ErrorNotice } from "../ui/States";

/** Who watches the task (reporters, assignees, commenters and mentioned people watch automatically) and a watch toggle for me. */
export function WatchersSection({ task }: { task: Task }) {
  const session = useSession();
  const { nameOf } = usePeople();
  const state = useResource(`watch:${task.id}`, () => getWatchState(task.id));
  const list = useResource(`watchers:${task.id}`, () => listWatchers(task.id));
  const { run } = useGuardedAction(() => {
    state.reload();
    list.reload();
  });
  const watching = state.data?.watching === true;
  const watchers = list.data ?? [];

  return (
    <section className="pm-section" aria-labelledby={`wt-${task.id}`}>
      <h2 id={`wt-${task.id}`}>
        Watchers <span className="pm-muted">{watchers.length}</span>
      </h2>
      {(state.error !== undefined || list.error !== undefined) && <ErrorNotice error={state.error ?? list.error} onRetry={() => { state.reload(); list.reload(); }} />}
      <div className="pm-actions">
        <button type="button" className="pm-btn pm-btn-small" disabled={state.data === undefined} onClick={() => void run(() => (watching ? unwatchTask(task.id) : watchTask(task.id)))}>
          {watching ? "Stop watching" : "Watch this task"}
        </button>
        {watching && state.data && <span className="pm-muted">You watch it ({humanize(state.data.reason)}).</span>}
      </div>
      <ul className="pm-plain-list">
        {watchers.map((w) => (
          <li key={w.user_id}>
            {nameOf(w.user_id)}
            {w.user_id === session.userId && <span className="pm-muted"> (you)</span>} <span className="pm-muted">{humanize(w.reason)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
