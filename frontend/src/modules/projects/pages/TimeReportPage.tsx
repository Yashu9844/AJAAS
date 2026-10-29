"use client";

import { useState } from "react";
import { getTimeReport } from "../api/planning-api";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { usePeople } from "../hooks/use-people";
import { useProject } from "../hooks/use-project";
import { useResource } from "../hooks/use-resource";
import { toDateKey } from "../lib/dates";
import { formatMinutes } from "../lib/format";
import { windowAround } from "../lib/gantt";
import { isValidReportRange, loggedVsEstimate } from "../lib/reports";

type GroupBy = "task" | "user";

/** Time tab: logged minutes of a period per task or per person, against the total estimate of the tasks. */
export function TimeReportPage() {
  const { project } = useProject();
  const { nameOf } = usePeople();
  const [range, setRange] = useState(() => windowAround(toDateKey(new Date()), 29, 30));
  const [groupBy, setGroupBy] = useState<GroupBy>("task");
  const valid = isValidReportRange(range.from, range.to);
  const res = useResource(valid ? `time-report:${project.id}:${range.from}:${range.to}:${groupBy}` : null, () => getTimeReport(project.id, range.from, range.to, groupBy));
  const report = res.data;
  const ratio = report ? loggedVsEstimate(report.total_minutes, report.estimate_minutes) : null;

  return (
    <section className="pm-section" aria-labelledby="pm-time-h">
      <h2 id="pm-time-h">Time report</h2>
      <div className="pm-toolbar">
        <label className="pm-field">
          <span>From</span>
          <input type="date" value={range.from} onChange={(e) => setRange({ ...range, from: e.target.value })} />
        </label>
        <label className="pm-field">
          <span>To</span>
          <input type="date" value={range.to} onChange={(e) => setRange({ ...range, to: e.target.value })} />
        </label>
        <label className="pm-field">
          <span>Group by</span>
          <select value={groupBy} onChange={(e) => setGroupBy(e.target.value as GroupBy)}>
            <option value="task">Task</option>
            <option value="user">Person</option>
          </select>
        </label>
      </div>
      {!valid && <p className="pm-error" role="alert">Choose a period of at most one year, with the start before the end.</p>}
      {res.loading && <SkeletonRows rows={4} height={32} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {report && (
        <>
          <p>
            <strong>{formatMinutes(report.total_minutes)}</strong> logged in {report.entries} entr{report.entries === 1 ? "y" : "ies"}
            {ratio && ratio.percent !== null ? (
              <>
                {" "}
                against an estimate of <strong>{formatMinutes(report.estimate_minutes)}</strong>{" "}
                <span className={ratio.over ? "pm-chip pm-pill-overdue" : "pm-chip"}>{ratio.percent}% of estimate</span>
              </>
            ) : (
              <span className="pm-muted"> (the tasks have no estimate)</span>
            )}
          </p>
          {ratio?.percent !== null && ratio && (
            <progress className="pm-progress" max={100} value={Math.min(ratio.percent ?? 0, 100)} aria-label="Logged time as a percent of the estimate" />
          )}
          {report.rows.length === 0 ? (
            <EmptyState title="No time logged in this period" />
          ) : (
            <div className="pm-table-wrap">
              <table className="pm-table">
                <caption className="pm-sr-only">Logged time per {groupBy === "task" ? "task" : "person"}</caption>
                <thead>
                  <tr>
                    <th scope="col">{groupBy === "task" ? "Task" : "Person"}</th>
                    <th scope="col">Logged</th>
                    <th scope="col">Entries</th>
                  </tr>
                </thead>
                <tbody>
                  {report.rows.map((r) => (
                    <tr key={r.key}>
                      <td>{groupBy === "task" ? `${project.key}-${r.number ?? ""} ${r.title ?? ""}` : nameOf(r.key)}</td>
                      <td>{formatMinutes(r.minutes)}</td>
                      <td>{r.entries}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </section>
  );
}
