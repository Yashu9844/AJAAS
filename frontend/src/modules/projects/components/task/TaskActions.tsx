"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { describeError } from "../../api/errors";
import { listProjects } from "../../api/projects-api";
import { archiveTask, duplicateTask, moveTaskToProject, restoreTask } from "../../api/tasks-api";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { taskPath } from "../../lib/paths";
import type { Task } from "../../types/task";
import { Dialog } from "../ui/Dialog";

interface Props {
  task: Task;
  canWrite: boolean;
  onChanged: () => void;
}

type Mode = "none" | "archive" | "move";

/** Duplicate, archive or restore, and move to another project (a project pick is all the API needs). */
export function TaskActions({ task, canWrite, onChanged }: Props) {
  const router = useRouter();
  const toast = useToast();
  const [mode, setMode] = useState<Mode>("none");
  const [target, setTarget] = useState("");
  const [copyLabels, setCopyLabels] = useState(true);
  const [copyAssignees, setCopyAssignees] = useState(false);
  const [busy, setBusy] = useState(false);
  const projects = useResource(mode === "move" ? "move-targets" : null, async () => (await listProjects({ status: "active" })).items);
  const archived = task.archived_at !== null;

  async function run(action: () => Promise<void>) {
    setBusy(true);
    try {
      await action();
    } catch (e) {
      toast.show(describeError(e), "error");
    } finally {
      setBusy(false);
    }
  }

  const duplicate = () =>
    run(async () => {
      const copy = await duplicateTask(task.id, copyLabels, copyAssignees);
      toast.show(`Created ${copy.key}.`);
      onChanged();
    });
  const toggleArchive = () =>
    run(async () => {
      if (archived) await restoreTask(task.id);
      else await archiveTask(task.id);
      setMode("none");
      onChanged();
    });
  const move = () =>
    run(async () => {
      const moved = await moveTaskToProject(task.id, target, task.version);
      setMode("none");
      toast.show(`Moved: it is now ${moved.key}. The old key still opens it.`);
      onChanged();
      router.push(taskPath(moved.key));
    });

  if (!canWrite) return null;
  return (
    <section className="pm-section" aria-label="Task actions">
      <div className="pm-actions">
        <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => void duplicate()}>
          Duplicate
        </button>
        <label className="pm-check">
          <input type="checkbox" checked={copyLabels} onChange={(e) => setCopyLabels(e.target.checked)} />
          <span>with labels</span>
        </label>
        <label className="pm-check">
          <input type="checkbox" checked={copyAssignees} onChange={(e) => setCopyAssignees(e.target.checked)} />
          <span>with assignees</span>
        </label>
        <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => (archived ? void toggleArchive() : setMode("archive"))}>
          {archived ? "Restore" : "Archive"}
        </button>
        <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => setMode("move")}>
          Move to project…
        </button>
      </div>
      <Dialog title={`Archive ${task.key}`} open={mode === "archive"} onClose={() => setMode("none")}>
        <p>Archive this task? It leaves the board and list and can be restored from this page.</p>
        <div className="pm-actions">
          <button type="button" className="pm-btn pm-btn-primary" onClick={() => void toggleArchive()}>
            Archive
          </button>
          <button type="button" className="pm-btn" onClick={() => setMode("none")}>
            Cancel
          </button>
        </div>
      </Dialog>
      <Dialog title={`Move ${task.key}`} open={mode === "move"} onClose={() => setMode("none")}>
        <p className="pm-muted">The task gets a new ticket key in the target project and keeps its comments, attachments and history. A task with subtasks or a parent cannot be moved.</p>
        <label className="pm-field">
          <span>Target project</span>
          <select value={target} onChange={(e) => setTarget(e.target.value)}>
            <option value="">Choose a project…</option>
            {(projects.data ?? [])
              .filter((p) => p.id !== task.project_id)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.key} · {p.name}
                </option>
              ))}
          </select>
        </label>
        <div className="pm-actions">
          <button type="button" className="pm-btn pm-btn-primary" disabled={!target || busy} onClick={() => void move()}>
            Move task
          </button>
          <button type="button" className="pm-btn" onClick={() => setMode("none")}>
            Cancel
          </button>
        </div>
      </Dialog>
    </section>
  );
}
