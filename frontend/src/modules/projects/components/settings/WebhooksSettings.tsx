"use client";

import { useState, type FormEvent } from "react";
import { createWebhook, listWebhooks } from "../../api/integrations-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { parseEventTypes, validateWebhookUrl } from "../../lib/automation-form";
import type { WebhookFormat } from "../../types/settings";
import { SecretBox } from "../ui/SecretBox";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";
import { WebhookRow } from "./WebhookRow";

interface ShownSecret {
  title: string;
  value: string;
}

/** Webhooks of the project. A signing secret appears once, in memory only, after create or rotate. */
export function WebhooksSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const toast = useToast();
  const res = useResource(`webhooks:${projectId}`, () => listWebhooks(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [secret, setSecret] = useState<ShownSecret | null>(null);
  const [format, setFormat] = useState<WebhookFormat>("generic");
  const editable = canManage && !denied;
  const items = res.data ?? [];

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    const url = String(data.get("url") ?? "").trim();
    const urlProblem = validateWebhookUrl(url);
    if (urlProblem) return toast.show(urlProblem, "error");
    const events = parseEventTypes(String(data.get("events") ?? ""));
    if (!events.ok) return toast.show(events.error, "error");
    void run(async () => {
      const created = await createWebhook(projectId, String(data.get("name") ?? "").trim(), url, format, events.input);
      setSecret({ title: `Signing secret for ${created.name}`, value: created.secret });
      form.reset();
    });
  }

  return (
    <SectionFrame title="Webhooks" id="pm-wh-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      <p className="pm-muted">Signed HTTP calls to your systems when events happen. Only https addresses on port 443 or 8443 are accepted.</p>
      {secret && <SecretBox title={secret.title} secret={secret.value} onDismiss={() => setSecret(null)} />}
      {items.length === 0 && <EmptyState title="No webhooks" />}
      <ul className="pm-plain-list">
        {items.map((h) => (
          <WebhookRow key={h.id} projectId={projectId} hook={h} editable={editable} onChanged={res.reload} onSecret={(title, value) => setSecret({ title, value })} />
        ))}
      </ul>
      {editable && (
        <form className="pm-inline-form" onSubmit={add} aria-label="New webhook">
          <label className="pm-sr-only" htmlFor="wh-name">
            Webhook name
          </label>
          <input id="wh-name" name="name" placeholder="Name" maxLength={80} required />
          <label className="pm-sr-only" htmlFor="wh-url">
            Webhook address
          </label>
          <input id="wh-url" name="url" type="url" placeholder="https://example.com/hook" required spellCheck={false} />
          <label className="pm-sr-only" htmlFor="wh-format">
            Format
          </label>
          <select id="wh-format" value={format} onChange={(e) => setFormat(e.target.value as WebhookFormat)}>
            <option value="generic">generic</option>
            <option value="slack">slack</option>
            <option value="discord">discord</option>
          </select>
          <label className="pm-sr-only" htmlFor="wh-events">
            Event types
          </label>
          <input id="wh-events" name="events" placeholder="Events, e.g. task.created, task.moved (empty = all)" />
          <button type="submit" className="pm-btn">
            Add webhook
          </button>
        </form>
      )}
    </SectionFrame>
  );
}
