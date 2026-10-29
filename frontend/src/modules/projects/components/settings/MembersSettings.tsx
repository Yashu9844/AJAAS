"use client";

import { useState } from "react";
import { addMember, changeMemberRole, listPeople, removeMember } from "../../api/projects-api";
import { rememberPeople } from "../../hooks/people-store";
import { useResource } from "../../hooks/use-resource";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { usePeople } from "../../hooks/use-people";
import { useSession } from "../../hooks/use-session";
import type { ProjectRoleKey } from "../../types/common";
import type { Member, Person } from "../../types/project";

const ROLES: ProjectRoleKey[] = ["owner", "manager", "member", "viewer"];

interface Props {
  projectId: string;
  members: Member[];
  canManage: boolean;
  onChanged: () => void;
}

/** Members and roles. Adding people needs the user directory (administrators); others see names as short ids. */
export function MembersSettings({ projectId, members, canManage, onChanged }: Props) {
  const { run, denied } = useGuardedAction(onChanged);
  const { nameOf } = usePeople();
  const session = useSession();
  const [newUser, setNewUser] = useState("");
  const [newRole, setNewRole] = useState<ProjectRoleKey>("member");
  const editable = canManage && !denied;
  const directory = useResource<Person[]>(editable ? "people-directory" : null, async () => {
    const page = await listPeople().catch(() => []);
    rememberPeople(page);
    return page;
  });
  const candidates = (directory.data ?? []).filter((p) => !members.some((m) => m.user_id === p.id));

  return (
    <section className="pm-section" aria-labelledby="pm-members-h">
      <h2 id="pm-members-h">Members</h2>
      <ul className="pm-plain-list">
        {members.map((m) => (
          <li key={m.user_id} className="pm-row-between pm-card">
            <span>
              {nameOf(m.user_id)}
              {m.user_id === session.userId && <span className="pm-muted"> (you)</span>}
              {m.source !== "direct" && <span className="pm-muted"> · via {m.source}</span>}
            </span>
            {editable ? (
              <span className="pm-actions">
                <label>
                  <span className="pm-sr-only">Role of {nameOf(m.user_id)}</span>
                  <select value={m.role_key ?? ""} onChange={(e) => void run(() => changeMemberRole(projectId, m.user_id, e.target.value as ProjectRoleKey))}>
                    {m.role_key === null && <option value="">{m.role_name}</option>}
                    {ROLES.map((r) => (
                      <option key={r} value={r}>
                        {r}
                      </option>
                    ))}
                  </select>
                </label>
                <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => removeMember(projectId, m.user_id))}>
                  Remove
                </button>
              </span>
            ) : (
              <span className="pm-chip pm-role">{m.role_name}</span>
            )}
          </li>
        ))}
      </ul>
      {editable && candidates.length > 0 && (
        <div className="pm-inline-form">
          <label className="pm-sr-only" htmlFor="mem-user">Person to add</label>
          <select id="mem-user" value={newUser} onChange={(e) => setNewUser(e.target.value)}>
            <option value="">Add a person…</option>
            {candidates.map((p) => (
              <option key={p.id} value={p.id}>
                {nameOf(p.id)}
              </option>
            ))}
          </select>
          <label className="pm-sr-only" htmlFor="mem-role">Role</label>
          <select id="mem-role" value={newRole} onChange={(e) => setNewRole(e.target.value as ProjectRoleKey)}>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="pm-btn"
            disabled={!newUser}
            onClick={() => void run(() => addMember(projectId, newUser, newRole)).then((ok) => ok && setNewUser(""))}
          >
            Add
          </button>
        </div>
      )}
    </section>
  );
}
