"use client";

import { useState, type FormEvent } from "react";
import { createGitIntegration, deleteGitIntegration, listGitIntegrations, rotateGitSecret } from "../../api/integrations-api";
import { currentBaseUrl } from "../../api/session-store";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { integrationPayloadUrl } from "../../lib/paths";
import type { GitProvider } from "../../types/settings";
import { SecretBox } from "../ui/SecretBox";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

interface Shown {
  title: string;
  secret: string;
  payloadUrl: string | null;
}

/** GitHub and Gitea integrations: tasks named KEY-123 in pull requests and pushes are linked. The secret is shown once. */
export function GitIntegrationsSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const res = useResource(`git-integrations:${projectId}`, () => listGitIntegrations(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [provider, setProvider] = useState<GitProvider>("github");
  const [shown, setShown] = useState<Shown | null>(null);
  const editable = canManage && !denied;
  const items = res.data ?? [];

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = String(new FormData(form).get("name") ?? "").trim();
    if (!name) return;
    void run(async () => {
      const created = await createGitIntegration(projectId, provider, name);
      setShown({ title: `Secret for ${created.name}`, secret: created.secret, payloadUrl: integrationPayloadUrl(currentBaseUrl(), created.provider, created.id) });
      form.reset();
    });
  }

  return (
    <SectionFrame title="Git integrations" id="pm-git-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      <p className="pm-muted">Configure a webhook in your repository with the payload URL and the secret (content type JSON; events: pull requests and pushes).</p>
      {shown && (
        <>
          {shown.payloadUrl && (
            <p>
              Payload URL: <span className="pm-mono">{shown.payloadUrl}</span>
            </p>
          )}
          <SecretBox title={shown.title} secret={shown.secret} onDismiss={() => setShown(null)} />
        </>
      )}
      {items.length === 0 && <EmptyState title="No git integrations" />}
      <ul className="pm-plain-list">
        {items.map((g) => (
          <li key={g.id} className="pm-card">
            <div className="pm-row-between">
              <span>
                <strong>{g.name}</strong> <span className="pm-chip">{g.provider}</span> <span className="pm-muted">secret ends in {g.secret_hint}</span>
                <br />
                <span className="pm-mono pm-muted">{integrationPayloadUrl(currentBaseUrl(), g.provider, g.id) ?? ""}</span>
              </span>
              {editable && (
                <span className="pm-actions">
                  <button
                    type="button"
                    className="pm-btn pm-btn-small"
                    onClick={() =>
                      void run(async () => {
                        const r = await rotateGitSecret(projectId, g.id);
                        setShown({ title: `New secret for ${g.name}`, secret: r.secret, payloadUrl: integrationPayloadUrl(currentBaseUrl(), g.provider, g.id) });
                      })
                    }
                  >
                    Rotate secret
                  </button>
                  <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteGitIntegration(projectId, g.id))} aria-label={`Delete integration ${g.name}`}>
                    Delete
                  </button>
                </span>
              )}
            </div>
          </li>
        ))}
      </ul>
      {editable && (
        <form className="pm-inline-form" onSubmit={add} aria-label="New git integration">
          <label className="pm-sr-only" htmlFor="gi-provider">
            Provider
          </label>
          <select id="gi-provider" value={provider} onChange={(e) => setProvider(e.target.value as GitProvider)}>
            <option value="github">github</option>
            <option value="gitea">gitea</option>
          </select>
          <label className="pm-sr-only" htmlFor="gi-name">
            Name
          </label>
          <input id="gi-name" name="name" placeholder="Name, e.g. main repository" maxLength={80} required />
          <button type="submit" className="pm-btn">
            Add integration
          </button>
        </form>
      )}
    </SectionFrame>
  );
}
