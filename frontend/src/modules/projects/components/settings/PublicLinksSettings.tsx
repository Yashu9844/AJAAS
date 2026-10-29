"use client";

import { useState } from "react";
import { createPublicLink, listPublicLinks, revokePublicLink } from "../../api/integrations-api";
import { loadEndpoint } from "../../api/config";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { useSession } from "../../hooks/use-session";
import { formatDateTime } from "../../lib/dates";
import { publicPageUrl } from "../../lib/paths";
import { SecretBox } from "../ui/SecretBox";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

/** Read-only public links. The page URL (which holds the secret token) is shown once after creation and never stored. */
export function PublicLinksSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const session = useSession();
  const res = useResource(`public-links:${projectId}`, () => listPublicLinks(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [shown, setShown] = useState<string | null>(null);
  const [showDescriptions, setShowDescriptions] = useState(false);
  const editable = canManage && !denied;
  const items = (res.data ?? []).filter((l) => !l.revoked_at);

  return (
    <SectionFrame title="Public links" id="pm-pl-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      <p className="pm-muted">Anyone with a link can read the project board without signing in (people are left out). At most 3 active links.</p>
      {shown && <SecretBox title="Public page link" secret={shown} note="Copy the link now. It contains the secret token, which is shown only once; revoke the link to stop access." onDismiss={() => setShown(null)} />}
      {items.length === 0 && <EmptyState title="No active public links" />}
      <ul className="pm-plain-list">
        {items.map((l) => (
          <li key={l.id} className="pm-row-between pm-card">
            <span>
              Link created {formatDateTime(l.created_at)} <span className="pm-muted">{l.show_descriptions ? "with descriptions" : "without descriptions"}</span>
            </span>
            {editable && (
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => revokePublicLink(projectId, l.id))}>
                Revoke
              </button>
            )}
          </li>
        ))}
      </ul>
      {editable && (
        <div className="pm-inline-form">
          <label className="pm-check">
            <input type="checkbox" checked={showDescriptions} onChange={(e) => setShowDescriptions(e.target.checked)} />
            <span>Show task descriptions</span>
          </label>
          <button
            type="button"
            className="pm-btn"
            onClick={() =>
              void run(async () => {
                const created = await createPublicLink(projectId, showDescriptions);
                const ep = loadEndpoint();
                setShown(publicPageUrl(window.location.origin, created.token, session.slug, ep.host, ep.port) ?? "");
              })
            }
          >
            Create public link
          </button>
        </div>
      )}
    </SectionFrame>
  );
}
