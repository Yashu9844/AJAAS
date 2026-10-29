"use client";

import { useCallback, useState } from "react";
import { describeError, isForbidden } from "../api/errors";
import { useToast } from "./use-toast";

/**
 * Runs a mutation, shows errors as toasts and remembers a 403 so the caller can hide the controls the server refused.
 * Hiding is a courtesy: the server stays the only authority.
 */
export function useGuardedAction(onDone?: () => void): { run: (action: () => Promise<unknown>) => Promise<boolean>; denied: boolean } {
  const toast = useToast();
  const [denied, setDenied] = useState(false);
  const run = useCallback(
    async (action: () => Promise<unknown>) => {
      try {
        await action();
        onDone?.();
        return true;
      } catch (e) {
        if (isForbidden(e)) setDenied(true);
        toast.show(describeError(e), "error");
        return false;
      }
    },
    [toast, onDone],
  );
  return { run, denied };
}
