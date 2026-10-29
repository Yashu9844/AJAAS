"use client";

import { useState } from "react";
import { archiveProject, restoreProject } from "../api/projects-api";
import { ColumnsSettings } from "../components/settings/ColumnsSettings";
import { CustomFieldsSettings } from "../components/settings/CustomFieldsSettings";
import { GitIntegrationsSettings } from "../components/settings/GitIntegrationsSettings";
import { GroupRulesSettings } from "../components/settings/GroupRulesSettings";
import { LabelsSettings } from "../components/settings/LabelsSettings";
import { MembersSettings } from "../components/settings/MembersSettings";
import { MilestonesSettings } from "../components/settings/MilestonesSettings";
import { PublicLinksSettings } from "../components/settings/PublicLinksSettings";
import { RecurrencesSettings } from "../components/settings/RecurrencesSettings";
import { RolesSettings } from "../components/settings/RolesSettings";
import { RulesSettings } from "../components/settings/RulesSettings";
import { WebhooksSettings } from "../components/settings/WebhooksSettings";
import { useGuardedAction } from "../hooks/use-guarded-action";
import { useProject } from "../hooks/use-project";

type Section = "general" | "access" | "automation" | "integrations";

const SECTIONS: { key: Section; label: string }[] = [
  { key: "general", label: "General" },
  { key: "access", label: "Access" },
  { key: "automation", label: "Automation" },
  { key: "integrations", label: "Webhooks & links" },
];

/** Settings tab with four sections. The server decides who may do what: a 403 hides the controls it refused. */
export function SettingsPage() {
  const { project, columns, labels, members, canManage, reload } = useProject();
  const { run, denied } = useGuardedAction(reload);
  const [section, setSection] = useState<Section>("general");
  const archived = project.status === "archived";
  const mayArchive = (project.my_role === "owner" || canManage || archived) && !denied;
  const manage = canManage || project.my_role === "owner";

  return (
    <>
      <div className="pm-subtabs" role="tablist" aria-label="Settings sections">
        {SECTIONS.map((s) => (
          <button key={s.key} type="button" role="tab" aria-selected={section === s.key} className="pm-subtab" onClick={() => setSection(s.key)}>
            {s.label}
          </button>
        ))}
      </div>
      {section === "general" && (
        <>
          <ColumnsSettings projectId={project.id} columns={columns} canManage={canManage} onChanged={reload} />
          <LabelsSettings projectId={project.id} labels={labels} canManage={canManage} onChanged={reload} />
          <MilestonesSettings projectId={project.id} canManage={manage} />
          <CustomFieldsSettings projectId={project.id} canManage={manage} />
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
      )}
      {section === "access" && (
        <>
          <MembersSettings projectId={project.id} members={members} canManage={canManage} onChanged={reload} />
          <GroupRulesSettings projectId={project.id} canManage={manage} />
          <RolesSettings />
        </>
      )}
      {section === "automation" && (
        <>
          <RulesSettings projectId={project.id} canManage={manage} />
          <RecurrencesSettings projectId={project.id} canManage={manage} />
        </>
      )}
      {section === "integrations" && (
        <>
          <WebhooksSettings projectId={project.id} canManage={manage} />
          <GitIntegrationsSettings projectId={project.id} canManage={manage} />
          <PublicLinksSettings projectId={project.id} canManage={manage} />
        </>
      )}
    </>
  );
}
