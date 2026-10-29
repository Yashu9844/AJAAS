import type { Member } from "../types/project";

/**
 * Display names for user ids. The API carries ids only; the project members list returns `display_name` (first and last
 * name, never the e-mail). Names are remembered for the session as members are loaded, and a person who is no longer
 * a member (or an API without the field) shows as a short id.
 */

const names = new Map<string, string>();
const listeners = new Set<() => void>();
let version = 0;

function bump(): void {
  version++;
  listeners.forEach((l) => l());
}

export function subscribeNames(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function namesVersion(): number {
  return version;
}

/** Name for a user id. Activity written by the system (rules, git integrations, recurrences) has no user: it shows as "System". */
export function nameFor(id: string | null | undefined): string {
  if (!id) return "System";
  return names.get(id) ?? `User ${id.slice(0, 8)}`;
}

export function hasName(id: string): boolean {
  return names.has(id);
}

/** Remembers the display names that a members response carries. */
export function rememberMembers(members: readonly Pick<Member, "user_id" | "display_name">[]): void {
  let changed = false;
  for (const m of members) {
    const name = m.display_name?.trim();
    if (name && names.get(m.user_id) !== name) {
      names.set(m.user_id, name);
      changed = true;
    }
  }
  if (changed) bump();
}

export function clearNames(): void {
  names.clear();
  bump();
}
