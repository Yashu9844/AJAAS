import { getPerson } from "../api/projects-api";
import type { Person } from "../types/project";

/**
 * Names for user ids. The API only carries ids, and the user directory is paged and administrator-only, so names are
 * resolved one id at a time on demand and cached for the session. A refused or failed lookup is remembered as "unknown"
 * and the UI shows a short id instead.
 */

const MAX_PER_CALL = 40;
const cache = new Map<string, Person | null>();
const inflight = new Set<string>();
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

export function cachedPerson(id: string): Person | undefined {
  return cache.get(id) ?? undefined;
}

/** Stores people that were loaded another way (for example a directory page). */
export function rememberPeople(people: readonly Person[]): void {
  people.forEach((p) => cache.set(p.id, p));
  bump();
}

/** Looks up the ids that are not known yet. Safe to call repeatedly. */
export function ensureNames(ids: readonly string[]): void {
  const wanted = [...new Set(ids)].filter((id) => id && !cache.has(id) && !inflight.has(id)).slice(0, MAX_PER_CALL);
  for (const id of wanted) {
    inflight.add(id);
    getPerson(id)
      .then((p) => cache.set(id, p))
      .catch(() => cache.set(id, null))
      .finally(() => {
        inflight.delete(id);
        bump();
      });
  }
}

export function clearNames(): void {
  cache.clear();
  inflight.clear();
  bump();
}
