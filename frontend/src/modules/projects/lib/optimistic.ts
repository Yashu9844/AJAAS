import type { Board, BoardColumn, Task, TaskCard } from "../types/task";

/** Finds a card and the column that holds it. */
export function findCard(board: Board, taskId: string): { column: BoardColumn; card: TaskCard; index: number } | null {
  for (const column of board.columns) {
    const index = column.tasks.findIndex((t) => t.id === taskId);
    if (index >= 0) return { column, card: column.tasks[index], index };
  }
  return null;
}

/**
 * Returns a new board with the card moved to `toColumnId` at `index` (index counts the destination cards without the
 * dragged one). Counts follow the move. Returns null when the card or column is unknown. The input is never mutated,
 * so the caller can keep it as the rollback snapshot.
 */
export function applyMove(board: Board, taskId: string, toColumnId: string, index: number): Board | null {
  const found = findCard(board, taskId);
  const target = board.columns.find((c) => c.id === toColumnId);
  if (!found || !target) return null;
  const columns = board.columns.map((col) => {
    if (col.id === found.column.id && col.id === target.id) {
      const rest = col.tasks.filter((t) => t.id !== taskId);
      rest.splice(clamp(index, rest.length), 0, found.card);
      return { ...col, tasks: rest };
    }
    if (col.id === found.column.id) {
      return { ...col, tasks: col.tasks.filter((t) => t.id !== taskId), count: Math.max(0, col.count - 1) };
    }
    if (col.id === target.id) {
      const tasks = col.tasks.slice();
      tasks.splice(clamp(index, tasks.length), 0, { ...found.card, column_id: target.id, category: target.category });
      return { ...col, tasks, count: col.count + 1 };
    }
    return col;
  });
  return { ...board, columns };
}

function clamp(index: number, max: number): number {
  return Math.max(0, Math.min(index, max));
}

/** Copies the server's authoritative fields (version, rank, completion) onto the card after a successful move. */
export function reconcileMoved(board: Board, task: Task): Board {
  return {
    ...board,
    columns: board.columns.map((col) => ({
      ...col,
      tasks: col.tasks.map((t) =>
        t.id === task.id
          ? { ...t, version: task.version, rank: task.rank, completed_at: task.completed_at, column_id: task.column_id, category: task.column_category, updated_at: task.updated_at }
          : t,
      ),
    })),
  };
}

/** True when a column holds more open work than its WIP limit allows. */
export function isOverWipLimit(column: Pick<BoardColumn, "wip_limit" | "count">): boolean {
  return column.wip_limit !== null && column.count > column.wip_limit;
}

/** Appends a further page of cards to a column (load more), skipping cards already shown. */
export function appendColumnPage(board: Board, columnId: string, cards: TaskCard[], next: string | null): Board {
  return {
    ...board,
    columns: board.columns.map((col) => {
      if (col.id !== columnId) return col;
      const seen = new Set(col.tasks.map((t) => t.id));
      return { ...col, tasks: [...col.tasks, ...cards.filter((c) => !seen.has(c.id))], next_cursor: next };
    }),
  };
}
