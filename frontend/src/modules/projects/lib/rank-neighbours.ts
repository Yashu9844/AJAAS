/** The neighbour the API needs to place a task: it ranks the task relative to ONE existing task. */
export interface DropTarget {
  before_id?: string;
  after_id?: string;
}

/**
 * Computes the neighbour for a drop.
 * `idsWithoutDragged` is the destination column's task ids in display order with the dragged task removed;
 * `index` is where the dragged task lands among them (0 = top). The task is placed before the card that will follow
 * it; at the bottom it is placed after the last card; an empty column needs no neighbour.
 */
export function computeDropTarget(idsWithoutDragged: readonly string[], index: number): DropTarget {
  const n = idsWithoutDragged.length;
  if (n === 0) return {};
  const at = Math.max(0, Math.min(index, n));
  if (at < n) return { before_id: idsWithoutDragged[at] };
  return { after_id: idsWithoutDragged[n - 1] };
}

/**
 * Converts a pointer position over a column's cards into an insertion index.
 * `midpoints` are the vertical centres of the cards (dragged card excluded) from top to bottom.
 */
export function insertionIndexFromPointer(midpoints: readonly number[], pointerY: number): number {
  let i = 0;
  while (i < midpoints.length && pointerY > midpoints[i]) i++;
  return i;
}
