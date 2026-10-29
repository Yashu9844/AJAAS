"use client";

import { TaskDetail } from "../components/task/TaskDetail";

/** Full-page task view at /workspace/tasks/KEY-12 (deep link; also the target of search results). */
export function TaskPage({ ticket }: { ticket: string }) {
  return (
    <div className="pm-page pm-page-narrow">
      <TaskDetail key={ticket} idOrKey={ticket} showProjectLink />
    </div>
  );
}
