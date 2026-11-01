"use client";

import { listProjectRoleOptions } from "../../api/projects-api";
import { useResource } from "../../hooks/use-resource";
import { roleOptions } from "../../lib/role-picker";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

/**
 * Roles that can be assigned on this project: the four predefined ones and the custom project roles of the organisation.
 * Read-only here: roles and their permissions are managed under Access control (M0) by an administrator.
 */
export function RolesSettings({ projectId }: { projectId: string }) {
  const res = useResource(`project-roles:${projectId}`, () => listProjectRoleOptions(projectId));
  const options = roleOptions(res.data ?? []);
  return (
    <SectionFrame title="Roles" id="pm-roles-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      <p className="pm-muted">
        These roles can be given to project members. Custom roles and their permissions are managed under Access control by an administrator, not here.
      </p>
      {options.length === 0 && <EmptyState title="No roles available" />}
      <ul className="pm-plain-list">
        {options.map((o) => (
          <li key={o.value} className="pm-card">
            <strong>{o.label}</strong>
          </li>
        ))}
      </ul>
    </SectionFrame>
  );
}
