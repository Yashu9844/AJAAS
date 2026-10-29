"use client";

import { useState } from "react";
import { deleteWebhook, rotateWebhookSecret, setWebhookActive, testWebhook } from "../../api/integrations-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useToast } from "../../hooks/use-toast";
import type { Webhook } from "../../types/settings";
import { WebhookDeliveries } from "./WebhookDeliveries";

interface Props {
  projectId: string;
  hook: Webhook;
  editable: boolean;
  onChanged: () => void;
  onSecret: (title: string, secret: string) => void;
}

/** One webhook: status, enable/disable, test ping, rotate secret (confirmed), delete and its deliveries. */
export function WebhookRow({ projectId, hook, editable, onChanged, onSecret }: Props) {
  const toast = useToast();
  const { run } = useGuardedAction(onChanged);
  const [showDeliveries, setShowDeliveries] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const [confirmRotate, setConfirmRotate] = useState(false);

  async function ping() {
    await run(async () => {
      const d = await testWebhook(projectId, hook.id);
      toast.show(d.status === "delivered" ? "Test ping delivered." : `Test ping ${d.status}${d.last_error ? `: ${d.last_error}` : ""}.`, d.status === "delivered" ? "info" : "warning");
      setRefreshKey((k) => k + 1);
      setShowDeliveries(true);
    });
  }

  async function rotate() {
    setConfirmRotate(false);
    await run(async () => {
      const res = await rotateWebhookSecret(projectId, hook.id);
      onSecret(`New signing secret for ${hook.name}`, res.secret);
    });
  }

  return (
    <li className="pm-card">
      <div className="pm-row-between">
        <span>
          <strong>{hook.name}</strong> <span className="pm-chip">{hook.format}</span> <span className="pm-chip">{hook.active ? "active" : "inactive"}</span>
          {hook.disabled_at && <span className="pm-chip pm-pill-warning">disabled after failures</span>}
          <br />
          <span className="pm-mono pm-muted">{hook.url}</span>
          <br />
          <span className="pm-muted">
            events: {hook.event_types.join(", ")} · secret ends in {hook.secret_hint} · {hook.failure_count} failure(s)
          </span>
        </span>
        <span className="pm-actions">
          <button type="button" className="pm-btn pm-btn-small" onClick={() => setShowDeliveries((v) => !v)} aria-expanded={showDeliveries}>
            Deliveries
          </button>
          {editable && (
            <>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void ping()}>
                Test ping
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => setWebhookActive(projectId, hook.id, !hook.active))}>
                {hook.active ? "Disable" : "Enable"}
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => setConfirmRotate(true)}>
                Rotate secret
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteWebhook(projectId, hook.id))} aria-label={`Delete webhook ${hook.name}`}>
                Delete
              </button>
            </>
          )}
        </span>
      </div>
      {confirmRotate && (
        <div className="pm-error" role="alert">
          <span>Rotating invalidates the current secret: receivers must be updated.</span>
          <span className="pm-actions">
            <button type="button" className="pm-btn pm-btn-small" onClick={() => void rotate()}>
              Rotate
            </button>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => setConfirmRotate(false)}>
              Cancel
            </button>
          </span>
        </div>
      )}
      {showDeliveries && <WebhookDeliveries projectId={projectId} webhookId={hook.id} canManage={editable} refreshKey={refreshKey} />}
    </li>
  );
}
