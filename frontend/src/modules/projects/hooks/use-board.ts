"use client";

import { useCallback } from "react";
import { describeError, isVersionConflict } from "../api/errors";
import { getBoard, moveTask } from "../api/tasks-api";
import { computeDropTarget } from "../lib/rank-neighbours";
import { appendColumnPage, applyMove, findCard, reconcileMoved } from "../lib/optimistic";
import type { Board } from "../types/task";
import { useResource } from "./use-resource";
import { useToast } from "./use-toast";

export interface BoardState {
  board: Board | undefined;
  loading: boolean;
  error: unknown;
  reload: () => void;
  /** Moves a card to `index` among the destination cards (dragged card excluded) with optimistic update and rollback. */
  moveCard: (taskId: string, toColumnId: string, index: number) => Promise<void>;
  loadMoreInColumn: (columnId: string) => Promise<void>;
}

export function useBoard(projectId: string): BoardState {
  const res = useResource<Board>(`board:${projectId}`, () => getBoard(projectId));
  const toast = useToast();
  const { data: board, setData, reload } = res;

  const moveCard = useCallback(
    async (taskId: string, toColumnId: string, index: number) => {
      if (!board) return;
      const found = findCard(board, taskId);
      const dest = board.columns.find((c) => c.id === toColumnId);
      if (!found || !dest) return;
      if (found.column.id === toColumnId && found.index === index) return;
      const neighbours = dest.tasks.filter((t) => t.id !== taskId).map((t) => t.id);
      const target = computeDropTarget(neighbours, index);
      const next = applyMove(board, taskId, toColumnId, index);
      if (!next) return;
      setData(next);
      try {
        const task = await moveTask(taskId, { column_id: toColumnId, ...target, version: found.card.version });
        setData((prev) => (prev ? reconcileMoved(prev, task) : prev));
        if (task.warnings?.includes("WIP_LIMIT_EXCEEDED")) toast.show(`"${dest.name}" is over its work-in-progress limit.`, "warning");
      } catch (e) {
        setData(board);
        toast.show(describeError(e), "error");
        if (isVersionConflict(e)) reload();
      }
    },
    [board, setData, reload, toast],
  );

  const loadMoreInColumn = useCallback(
    async (columnId: string) => {
      const col = board?.columns.find((c) => c.id === columnId);
      if (!col?.next_cursor) return;
      try {
        const more = await getBoard(projectId, { [columnId]: col.next_cursor });
        const page = more.columns.find((c) => c.id === columnId);
        if (page) setData((prev) => (prev ? appendColumnPage(prev, columnId, page.tasks, page.next_cursor) : prev));
      } catch (e) {
        toast.show(describeError(e), "error");
      }
    },
    [board, projectId, setData, toast],
  );

  return { board, loading: res.loading, error: res.error, reload, moveCard, loadMoreInColumn };
}
