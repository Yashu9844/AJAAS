"use client";

import { useState } from "react";
import { addMember, changeMemberRole, listPeople, listProjectRoles, removeMember } from "../../api/projects-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { usePeople } from "../../hooks/use-people";
import { useResource } from "../../hooks/use-resource";
import { useSession } from "../../hooks/use-session";
import { directoryLabel } from "../../lib/format";
import { currentRoleValue, decodeRole, roleOptions } from "../../lib/role-picker";
import type { Member, Person } from "../../types/project";
import type { ProjectRole } from "../../types/settings";

interface Props {
  projectId: string;
  members: Member[];
  canManage: boolean;
  onChanged: () => void;
}

/** Members and roles (built-in and custom). Adding people needs the user directory (administrators). */
export function MembersSettings({ projectId, members, canManage, onChanged }: Props) {
  const { run, denied } = useGuardedAction(onChanged);
  const { nameOf } = usePeople();
  const session = useSession();
  const [newUser, setNewUser] = useState("");
  const [newRole, setNewRole] = useState("member");
  const editable = canManage && !denied;
  const directory = useResource<Person[]>(editable ? "people-directory" : null, () => listPeople().catch(() => []));
  const customRoles = useResource<ProjectRole[]>(editable ? "project-roles-picker" : null, () => listProjectRoles().catch(() => []));
  const options = roleOptions(customRoles.data ?? []);
  const candidates = (directory.data ?? []).filter((p) => !members.some((m) => m.user_id === p.id));

  function change(userId: string, value: string) {
    const choice = decodeRole(value);
    if (choice) void run(() => changeMemberRole(projectId, userId, choice));
  }

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
                  <select value={currentRoleValue(m)} onChange={(e) => change(m.user_id, e.target.value)}>
                    {currentRoleValue(m) === "" && <option value="">{m.role_name}</option>}
                    {options.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
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
          <label className="pm-sr-only" htmlFor="mem-user">
            Person to add
          </label>
          <select id="mem-user" value={newUser} onChange={(e) => setNewUser(e.target.value)}>
            <option value="">Add a person…</option>
            {candidates.map((p) => (
              <option key={p.id} value={p.id}>
                {directoryLabel(p)}
              </option>
            ))}
          </select>
          <label className="pm-sr-only" htmlFor="mem-role">
            Role
          </label>
          <select id="mem-role" value={newRole} onChange={(e) => setNewRole(e.target.value)}>
            {options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="pm-btn"
            disabled={!newUser}
            onClick={() => {
              const choice = decodeRole(newRole);
              if (choice) void run(() => addMember(projectId, newUser, choice)).then((ok) => ok && setNewUser(""));
            }}
          >
            Add
          </button>
        </div>
      )}
    </section>
  );
}
