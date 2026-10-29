"use client";

import { listDeliveries, redeliver } from "../../api/integrations-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { formatDateTime } from "../../lib/dates";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

/** Recent deliveries of one webhook with status, attempts and the short last error. */
export function WebhookDeliveries({ projectId, webhookId, canManage, refreshKey }: { projectId: string; webhookId: string; canManage: boolean; refreshKey: number }) {
  const res = useResource(`deliveries:${webhookId}:${refreshKey}`, () => listDeliveries(projectId, webhookId));
  const { run, denied } = useGuardedAction(res.reload);
  const items = res.data ?? [];
  return (
    <div className="pm-deliveries" role="region" aria-label="Deliveries">
      {res.loading && <SkeletonRows rows={2} height={24} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && items.length === 0 && <EmptyState title="No deliveries yet" />}
      <ul className="pm-plain-list">
        {items.map((d) => (
          <li key={d.id} className="pm-row-between">
            <span>
              <span className={d.status === "delivered" ? "pm-chip pm-pill-ok" : "pm-chip"}>{d.status}</span> {d.event_type}{" "}
              <span className="pm-muted">
                {d.attempts} attempt(s){d.response_status ? ` · HTTP ${d.response_status}` : ""}
                {d.last_error ? ` · ${d.last_error}` : ""} · {formatDateTime(d.created_at)}
              </span>
            </span>
            {canManage && !denied && d.status !== "delivered" && (
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => redeliver(projectId, webhookId, d.id))}>
                Redeliver
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
