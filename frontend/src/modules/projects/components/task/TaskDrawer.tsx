"use client";

import Link from "next/link";
import { taskPath } from "../../lib/paths";
import { Dialog } from "../ui/Dialog";
import { TaskDetail } from "./TaskDetail";

interface Props {
  ticket: string | null;
  onClose: () => void;
  onChanged: () => void;
}

/** Right-hand drawer with the task; the full page lives at /workspace/tasks/KEY-12. */
export function TaskDrawer({ ticket, onClose, onChanged }: Props) {
  return (
    <Dialog title={ticket ?? "Task"} open={ticket !== null} onClose={onClose} variant="drawer">
      {ticket && (
        <>
          <p className="pm-drawer-link">
            <Link href={taskPath(ticket)}>Open full page</Link>
          </p>
          <TaskDetail key={ticket} idOrKey={ticket} onChanged={onChanged} />
        </>
      )}
    </Dialog>
  );
}
