import type { Member, RoleChoice } from "../types/project";
import type { ProjectRoleOption } from "../types/settings";

const ID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const NIL_ID = "00000000-0000-0000-0000-000000000000";
const ORDER = ["owner", "manager", "member", "viewer"];

export interface RoleOption {
  value: string;
  label: string;
}

/** Options of a role picker from GET /projects/{id}/roles: the four predefined roles first (in rank order), then custom roles by name. */
export function roleOptions(roles: readonly ProjectRoleOption[]): RoleOption[] {
  const rank = (r: ProjectRoleOption) => (r.key ? ORDER.indexOf(r.key) : ORDER.length);
  return [...roles]
    .sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
    .map((r) => ({ value: r.role_id, label: r.key ? r.key : `${r.name} (custom)` }));
}

/** The select value of a member's current role ("" when the member has none, for example after a migration). */
export function currentRoleValue(m: Pick<Member, "role_id">): string {
  return m.role_id && m.role_id !== NIL_ID ? m.role_id : "";
}

/** Turns a select value into the API field; null for anything that is not a role id. */
export function decodeRole(value: string): RoleChoice | null {
  return ID_RE.test(value) && value !== NIL_ID ? { role_id: value } : null;
}

/** The role id of a predefined role key (for flows that know "member" but not its id); undefined when the list has none. */
export function roleIdForKey(roles: readonly ProjectRoleOption[], key: string): string | undefined {
  return roles.find((r) => r.key === key)?.role_id;
}
