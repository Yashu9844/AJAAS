"use client";

import { useMemo, useState } from "react";
import { useNow } from "../../hooks/use-now";
import type { Board } from "../../types/task";
import { BoardColumn } from "./BoardColumn";

interface BoardViewProps {
  board: Board;
  canWrite: boolean;
  onOpen: (ticket: string) => void;
  onMove: (taskId: string, columnId: string, index: number) => Promise<void>;
  onLoadMore: (columnId: string) => Promise<void>;
  onQuickAdd: (columnId: string, title: string) => Promise<void>;
}

/** Kanban columns with native HTML5 drag and drop. The page owns the data and the optimistic move. */
export function BoardView({ board, canWrite, onOpen, onMove, onLoadMore, onQuickAdd }: BoardViewProps) {
  const [dragId, setDragId] = useState<string | null>(null);
  const nowMs = useNow();
  const columnChoices = useMemo(() => board.columns.map((c) => ({ id: c.id, name: c.name })), [board.columns]);

  return (
    <div className="pm-board" role="group" aria-label="Board">
      {board.columns.map((column) => (
        <BoardColumn
          key={column.id}
          column={column}
          allColumns={columnChoices}
          nowMs={nowMs}
          canWrite={canWrite}
          dragId={dragId}
          onOpen={onOpen}
          onDragStart={setDragId}
          onDragEnd={() => setDragId(null)}
          onDrop={(taskId, columnId, index) => {
            setDragId(null);
            void onMove(taskId, columnId, index);
          }}
          onLoadMore={(id) => void onLoadMore(id)}
          onQuickAdd={onQuickAdd}
        />
      ))}
    </div>
  );
}
