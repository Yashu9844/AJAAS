"use client";

import { useState, type FormEvent } from "react";
import { createProjectRole, deleteProjectRole, listProjectPermissions, listProjectRoles, updateProjectRole } from "../../api/projects-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { togglePermission } from "../../lib/role-picker";
import type { ProjectRole } from "../../types/settings";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

interface Draft {
  id: string | null;
  name: string;
  description: string;
  permissions: string[];
}

const NEW_DRAFT: Draft = { id: null, name: "", description: "", permissions: ["project.read", "task.read"] };

/** Tenant-defined project roles. Needs the tenant permission: a 403 on the list hides the whole section. */
export function RolesSettings() {
  const toast = useToast();
  const roles = useResource<ProjectRole[]>("project-roles", listProjectRoles);
  const perms = useResource<string[]>("project-permissions", listProjectPermissions);
  const { run, denied } = useGuardedAction(roles.reload);
  const [draft, setDraft] = useState<Draft | null>(null);
  const items = roles.data ?? [];

  function save(e: FormEvent) {
    e.preventDefault();
    if (!draft) return;
    const name = draft.name.trim();
    if (!name) return toast.show("Give the role a name.", "error");
    if (draft.permissions.length === 0) return toast.show("Choose at least one permission.", "error");
    const action = draft.id ? () => updateProjectRole(draft.id ?? "", name, draft.description, draft.permissions) : () => createProjectRole(name, draft.description, draft.permissions);
    void run(action).then((ok) => ok && setDraft(null));
  }

  return (
    <SectionFrame title="Custom roles" id="pm-roles-h" loading={roles.loading} error={roles.error} onRetry={roles.reload}>
      <p className="pm-muted">Roles you define for the whole organisation. They appear in the role pickers of every project. Creating roles needs a tenant permission.</p>
      {items.length === 0 && <EmptyState title="No custom roles" />}
      <ul className="pm-plain-list">
        {items.map((r) => (
          <li key={r.id} className="pm-row-between pm-card">
            <span>
              <strong>{r.name}</strong> <span className="pm-muted">{r.permissions.join(", ")}</span>
            </span>
            {!denied && (
              <span className="pm-actions">
                <button type="button" className="pm-btn pm-btn-small" onClick={() => setDraft({ id: r.id, name: r.name, description: r.description, permissions: r.permissions })}>
                  Edit
                </button>
                <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteProjectRole(r.id))} aria-label={`Delete role ${r.name}`}>
                  Delete
                </button>
              </span>
            )}
          </li>
        ))}
      </ul>
      {!denied && !draft && (
        <button type="button" className="pm-btn" onClick={() => setDraft(NEW_DRAFT)}>
          New role
        </button>
      )}
      {draft && !denied && (
        <form className="pm-form pm-card" onSubmit={save} aria-label={draft.id ? "Edit role" : "New role"}>
          <label className="pm-field">
            <span>Role name</span>
            <input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} maxLength={60} required />
          </label>
          <label className="pm-field">
            <span>Description</span>
            <input value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} maxLength={200} />
          </label>
          <fieldset className="pm-field pm-checks">
            <legend>Permissions</legend>
            {(perms.data ?? []).map((p) => (
              <label key={p} className="pm-check">
                <input type="checkbox" checked={draft.permissions.includes(p)} onChange={(e) => setDraft({ ...draft, permissions: togglePermission(draft.permissions, p, e.target.checked) })} />
                <span>{p}</span>
              </label>
            ))}
          </fieldset>
          <div className="pm-actions">
            <button type="submit" className="pm-btn pm-btn-primary">
              {draft.id ? "Save role" : "Create role"}
            </button>
            <button type="button" className="pm-btn" onClick={() => setDraft(null)}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </SectionFrame>
  );
}
