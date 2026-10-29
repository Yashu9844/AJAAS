"use client";

import { getSprintReport } from "../../api/planning-api";
import { useResource } from "../../hooks/use-resource";
import { formatMinutes } from "../../lib/format";
import { summarizeSprintReport } from "../../lib/reports";
import { ErrorNotice, SkeletonRows } from "../ui/States";

/** Committed versus completed counts and estimates of a sprint (stored when completed, live otherwise). */
export function SprintReportPanel({ sprintId }: { sprintId: string }) {
  const res = useResource(`sprint-report:${sprintId}`, () => getSprintReport(sprintId));
  const sum = res.data ? summarizeSprintReport(res.data.report) : null;
  return (
    <div className="pm-deliveries" role="region" aria-label="Sprint report">
      {res.loading && <SkeletonRows rows={2} height={22} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {sum && res.data && (
        <>
          <p>
            <strong>
              {sum.completed} of {sum.committed} tasks completed
            </strong>
            {sum.completionPercent !== null && <span className="pm-chip"> {sum.completionPercent}%</span>}
            {res.data.live && <span className="pm-muted"> (live numbers; stored when the sprint is completed)</span>}
          </p>
          <ul className="pm-plain-list">
            <li>
              Estimate: {formatMinutes(sum.completedEstimate)} completed of {formatMinutes(sum.committedEstimate)} committed
            </li>
            <li>{sum.movedUnfinished} unfinished task(s) moved on when the sprint was completed</li>
          </ul>
        </>
      )}
    </div>
  );
}
