"use client";

import { useCallback } from "react";
import { describeError, isVersionConflict } from "../api/errors";
import { getTask, updateTask } from "../api/tasks-api";
import type { Task, TaskUpdate } from "../types/task";
import { useResource } from "./use-resource";
import { useToast } from "./use-toast";

export interface TaskDetailState {
  task: Task | undefined;
  loading: boolean;
  error: unknown;
  reload: () => void;
  /** Saves changes with the version the task was read at. Returns true on success; a 409 reloads the latest task. */
  save: (changes: TaskUpdate) => Promise<boolean>;
}

/** Loads one task by ticket key or id and provides version-checked saving. `onChanged` tells lists to refresh. */
export function useTaskDetail(idOrKey: string | null, onChanged?: () => void): TaskDetailState {
  const res = useResource<Task>(idOrKey ? `task:${idOrKey}` : null, () => getTask(idOrKey ?? ""));
  const toast = useToast();
  const { data: task, setData, reload } = res;

  const save = useCallback(
    async (changes: TaskUpdate) => {
      if (!task) return false;
      try {
        const updated = await updateTask(task.id, task.version, changes);
        setData(updated);
        onChanged?.();
        return true;
      } catch (e) {
        toast.show(describeError(e), "error");
        if (isVersionConflict(e)) reload();
        return false;
      }
    },
    [task, setData, reload, toast, onChanged],
  );

  return { task, loading: res.loading, error: res.error, reload, save };
}
