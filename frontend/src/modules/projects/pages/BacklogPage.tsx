"use client";

import { useState } from "react";
import { describeError } from "../api/errors";
import { getBacklog, listSprints, planTasks } from "../api/planning-api";
import { SprintsPanel } from "../components/sprints/SprintsPanel";
import { TaskDrawer } from "../components/task/TaskDrawer";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { PriorityChip } from "../components/ui/Chips";
import { useProject } from "../hooks/use-project";
import { useResource } from "../hooks/use-resource";
import { useToast } from "../hooks/use-toast";

/** Backlog and sprints tab (scrum projects only; kanban projects answer 409 PROJECT_MODE). */
export function BacklogPage() {
  const { project, canWrite, canManage } = useProject();
  const toast = useToast();
  const [ticket, setTicket] = useState<string | null>(null);
  const sprints = useResource(`sprints:${project.id}`, () => listSprints(project.id));
  const backlog = useResource(`backlog:${project.id}`, () => getBacklog(project.id));

  function refresh() {
    sprints.reload();
    backlog.reload();
  }

  async function moveToSprint(taskId: string, sprintId: string) {
    try {
      const results = await planTasks(project.id, [taskId], sprintId || null);
      const failed = results.find((r) => r.error);
      if (failed?.error) toast.show(failed.error, "error");
      refresh();
    } catch (e) {
      toast.show(describeError(e), "error");
    }
  }

  const open = (sprints.data ?? []).filter((s) => s.status !== "completed");
  const items = backlog.data?.items ?? [];
  return (
    <>
      {sprints.loading && <SkeletonRows rows={2} height={48} />}
      {sprints.error !== undefined && <ErrorNotice error={sprints.error} onRetry={sprints.reload} />}
      {sprints.data && <SprintsPanel projectId={project.id} sprints={sprints.data} canManage={canManage} onChanged={refresh} />}
      <section className="pm-section" aria-labelledby="pm-backlog-h">
        <h2 id="pm-backlog-h">Backlog</h2>
        {backlog.loading && <SkeletonRows rows={4} height={36} />}
        {backlog.error !== undefined && <ErrorNotice error={backlog.error} onRetry={backlog.reload} />}
        {backlog.data && items.length === 0 && <EmptyState title="The backlog is empty" />}
        <ul className="pm-plain-list">
          {items.map((t) => (
            <li key={t.id} className="pm-row-between pm-card">
              <button type="button" className="pm-link-btn" onClick={() => setTicket(t.ticket)}>
                <span className="pm-key">{t.ticket}</span> {t.title}
              </button>
              <span className="pm-actions">
                <PriorityChip priority={t.priority} />
                {canWrite && open.length > 0 && (
                  <label>
                    <span className="pm-sr-only">Move {t.ticket} to a sprint</span>
                    <select value="" onChange={(e) => e.target.value && void moveToSprint(t.id, e.target.value)}>
                      <option value="">Move to sprint…</option>
                      {open.map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
              </span>
            </li>
          ))}
        </ul>
      </section>
      <TaskDrawer ticket={ticket} onClose={() => setTicket(null)} onChanged={refresh} />
    </>
  );
}
