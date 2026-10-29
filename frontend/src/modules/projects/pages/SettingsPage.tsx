"use client";

import { archiveProject, restoreProject } from "../api/projects-api";
import { ColumnsSettings } from "../components/settings/ColumnsSettings";
import { LabelsSettings } from "../components/settings/LabelsSettings";
import { MembersSettings } from "../components/settings/MembersSettings";
import { useGuardedAction } from "../hooks/use-guarded-action";
import { useProject } from "../hooks/use-project";

/** Settings tab: columns, labels, members and archive/restore. The server decides who may do what (403 hides controls). */
export function SettingsPage() {
  const { project, columns, labels, members, canManage, reload } = useProject();
  const { run, denied } = useGuardedAction(reload);
  const archived = project.status === "archived";
  const mayArchive = (project.my_role === "owner" || canManage || archived) && !denied;

  return (
    <>
      <ColumnsSettings projectId={project.id} columns={columns} canManage={canManage} onChanged={reload} />
      <LabelsSettings projectId={project.id} labels={labels} canManage={canManage} onChanged={reload} />
      <MembersSettings projectId={project.id} members={members} canManage={canManage} onChanged={reload} />
      {mayArchive && (
        <section className="pm-section" aria-labelledby="pm-archive-h">
          <h2 id="pm-archive-h">{archived ? "Restore project" : "Archive project"}</h2>
          <p className="pm-muted">
            {archived ? "Restoring makes the project editable again." : "Archiving freezes tasks and hides the project from the default list. It can be restored."}
          </p>
          <button type="button" className="pm-btn" onClick={() => void run(() => (archived ? restoreProject(project.id) : archiveProject(project.id)))}>
            {archived ? "Restore" : "Archive"}
          </button>
        </section>
      )}
    </>
  );
}
