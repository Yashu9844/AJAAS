"use client";

import { useState } from "react";
import { describeError } from "../api/errors";
import { createTask } from "../api/tasks-api";
import { BoardView } from "../components/board/BoardView";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { TaskDrawer } from "../components/task/TaskDrawer";
import { useBoard } from "../hooks/use-board";
import { useProject } from "../hooks/use-project";
import { useToast } from "../hooks/use-toast";

/** Board tab: kanban with drag and drop, quick add and the task drawer. */
export function BoardPage() {
  const { project, canWrite } = useProject();
  const toast = useToast();
  const board = useBoard(project.id);
  const [ticket, setTicket] = useState<string | null>(null);

  async function quickAdd(columnId: string, title: string) {
    try {
      const task = await createTask(project.id, { title, column_id: columnId });
      if (task.warnings?.includes("WIP_LIMIT_EXCEEDED")) toast.show("That column is over its work-in-progress limit.", "warning");
      board.reload();
    } catch (e) {
      toast.show(describeError(e), "error");
      throw e;
    }
  }

  return (
    <>
      <h2 className="pm-sr-only">Board</h2>
      {board.loading && <SkeletonRows rows={3} height={90} />}
      {board.error !== undefined && !board.board && <ErrorNotice error={board.error} onRetry={board.reload} />}
      {board.board && board.board.columns.length === 0 && <EmptyState title="This board has no columns" hint="Add columns in Settings." />}
      {board.board && (
        <BoardView
          board={board.board}
          canWrite={canWrite}
          onOpen={setTicket}
          onMove={board.moveCard}
          onLoadMore={board.loadMoreInColumn}
          onQuickAdd={quickAdd}
        />
      )}
      <TaskDrawer ticket={ticket} onClose={() => setTicket(null)} onChanged={board.reload} />
    </>
  );
}
