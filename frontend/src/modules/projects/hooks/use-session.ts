"use client";

import { useSyncExternalStore } from "react";
import { getServerSessionState, getSessionState, subscribeSession, type SessionState } from "../api/session-store";

/** The current session (token held in memory by the session store). */
export function useSession(): SessionState {
  return useSyncExternalStore(subscribeSession, getSessionState, getServerSessionState);
}
