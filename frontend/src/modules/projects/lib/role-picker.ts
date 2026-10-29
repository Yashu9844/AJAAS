import type { ProjectRoleKey } from "../types/common";
import type { Member, RoleChoice } from "../types/project";
import type { ProjectRole } from "../types/settings";

export const BUILTIN_ROLES: ProjectRoleKey[] = ["owner", "manager", "member", "viewer"];

const CUSTOM_PREFIX = "custom:";
const ID_RE = /^[0-9a-fA-F-]{36}$/;

export interface RoleOption {
  value: string;
  label: string;
}

/** Options of a role picker: the four built-in roles, then the tenant's custom roles. */
export function roleOptions(custom: readonly Pick<ProjectRole, "id" | "name">[]): RoleOption[] {
  return [...BUILTIN_ROLES.map((r) => ({ value: r, label: r })), ...custom.map((c) => ({ value: `${CUSTOM_PREFIX}${c.id}`, label: `${c.name} (custom)` }))];
}

/** The select value of a member's current role. */
export function currentRoleValue(m: Pick<Member, "role_key" | "custom_role_id">): string {
  if (m.custom_role_id) return `${CUSTOM_PREFIX}${m.custom_role_id}`;
  return m.role_key ?? "";
}

/** Turns a select value into the API fields; null for an unknown value. */
export function decodeRole(value: string): RoleChoice | null {
  if (value.startsWith(CUSTOM_PREFIX)) {
    const id = value.slice(CUSTOM_PREFIX.length);
    return ID_RE.test(id) ? { custom_role_id: id } : null;
  }
  return (BUILTIN_ROLES as string[]).includes(value) ? { role: value as ProjectRoleKey } : null;
}

/** Permissions toggled in a role editor (returns a new sorted list). */
export function togglePermission(current: readonly string[], permission: string, on: boolean): string[] {
  const next = new Set(current);
  if (on) next.add(permission);
  else next.delete(permission);
  return [...next].sort();
}
