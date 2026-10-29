"use client";

import { useEffect, useSyncExternalStore, type ReactNode } from "react";
import { personName } from "../lib/format";
import { cachedPerson, clearNames, ensureNames, namesVersion, subscribeNames } from "./people-store";
import { useSession } from "./use-session";

interface PeopleValue {
  /** Display name for a user id; a short id until (or unless) the name could be resolved. */
  nameOf: (userId: string) => string;
}

/** Clears cached names when the signed-in user changes. */
export function PeopleProvider({ children }: { children: ReactNode }) {
  const session = useSession();
  useEffect(() => {
    clearNames();
  }, [session.userId]);
  return <>{children}</>;
}

export function usePeople(): PeopleValue {
  useSyncExternalStore(subscribeNames, namesVersion, namesVersion);
  return { nameOf: (id) => personName(cachedPerson(id), id) };
}

/** Asks for the names of these ids (call with every id a view is about to show). */
export function useEnsureNames(ids: readonly string[]): void {
  const key = ids.join(",");
  useEffect(() => {
    ensureNames(key ? key.split(",") : []);
  }, [key]);
}
