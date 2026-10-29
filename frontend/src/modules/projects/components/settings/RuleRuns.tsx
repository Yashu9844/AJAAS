"use client";

import { listRuleRuns } from "../../api/automation-api";
import { useResource } from "../../hooks/use-resource";
import { formatDateTime } from "../../lib/dates";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

/** What a rule did, newest first (status ok, skipped or failed). */
export function RuleRuns({ projectId, ruleId }: { projectId: string; ruleId: string }) {
  const res = useResource(`rule-runs:${ruleId}`, () => listRuleRuns(projectId, ruleId));
  const items = res.data ?? [];
  return (
    <div className="pm-deliveries" role="region" aria-label="Rule runs">
      {res.loading && <SkeletonRows rows={2} height={22} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && items.length === 0 && <EmptyState title="This rule has not run yet" />}
      <ul className="pm-plain-list">
        {items.map((r) => (
          <li key={r.id}>
            <span className={r.status === "ok" ? "pm-chip pm-pill-ok" : r.status === "failed" ? "pm-chip pm-pill-overdue" : "pm-chip"}>{r.status}</span> {r.action_count} action(s)
            {r.error_code && <span className="pm-muted"> · {r.error_code}</span>}
            <span className="pm-muted"> · {formatDateTime(r.created_at)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
