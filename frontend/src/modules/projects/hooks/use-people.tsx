"use client";

import { useEffect, useMemo, useSyncExternalStore, type ReactNode } from "react";
import { clearNames, nameFor, namesVersion, subscribeNames } from "./people-store";
import { useSession } from "./use-session";

interface PeopleValue {
  /** Display name for a user id; a short id when the person is not a known member. */
  nameOf: (userId: string | null | undefined) => string;
}

/** Clears remembered names when the signed-in user changes. */
export function PeopleProvider({ children }: { children: ReactNode }) {
  const session = useSession();
  useEffect(() => {
    clearNames();
  }, [session.userId]);
  return <>{children}</>;
}

/** The one cached project-people lookup: names come from the members responses that the app has loaded. */
export function usePeople(): PeopleValue {
  const version = useSyncExternalStore(subscribeNames, namesVersion, namesVersion);
  // A new function identity per names version makes memoised callers (React Compiler) render the new names.
  return useMemo(() => ({ nameOf: (id: string | null | undefined) => (version >= 0 ? nameFor(id) : "") }), [version]);
}
