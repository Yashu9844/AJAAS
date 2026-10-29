"use client";

import { useState, type FormEvent } from "react";
import { describeError } from "../../api/errors";
import { createTask, listSubtasks } from "../../api/tasks-api";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { taskPath } from "../../lib/paths";
import type { Task } from "../../types/task";
import Link from "next/link";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

export function SubtasksSection({ task, canWrite }: { task: Task; canWrite: boolean }) {
  const toast = useToast();
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const res = useResource(`subtasks:${task.id}`, () => listSubtasks(task.id));

  async function add(e: FormEvent) {
    e.preventDefault();
    const t = title.trim();
    if (!t) return;
    setBusy(true);
    try {
      await createTask(task.project_id, { title: t, parent_task_id: task.id });
      setTitle("");
      res.reload();
    } catch (err) {
      toast.show(describeError(err), "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="pm-section" aria-labelledby={`sub-${task.id}`}>
      <h2 id={`sub-${task.id}`}>
        Subtasks {task.subtask_total > 0 && <span className="pm-muted">{task.subtask_done}/{task.subtask_total} done</span>}
      </h2>
      {res.loading && <SkeletonRows rows={2} height={28} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && res.data.length === 0 && <EmptyState title="No subtasks" />}
      <ul className="pm-plain-list">
        {(res.data ?? []).map((s) => (
          <li key={s.id}>
            {s.key ? <Link href={taskPath(s.key)}>{s.key}</Link> : null} <span>{s.title}</span>
          </li>
        ))}
      </ul>
      {canWrite && (
        <form className="pm-inline-form" onSubmit={add}>
          <label className="pm-sr-only" htmlFor={`sub-add-${task.id}`}>
            New subtask title
          </label>
          <input id={`sub-add-${task.id}`} value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Add a subtask" maxLength={300} />
          <button type="submit" className="pm-btn" disabled={busy || !title.trim()}>
            Add
          </button>
        </form>
      )}
    </section>
  );
}
