"use client";

import { useState, type FormEvent } from "react";
import { addGroupRule, listGroupRules, refreshGroupRules, removeGroupRule } from "../../api/settings-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { isUuid } from "../../lib/automation-form";
import { formatDateTime } from "../../lib/dates";
import { humanize } from "../../lib/format";
import type { GroupRuleKind } from "../../types/settings";
import { EmptyState } from "../ui/States";
import { SectionFrame } from "./SectionState";

const KINDS: GroupRuleKind[] = ["department", "team", "reports_of"];

/** Membership rules: everyone in a department or team, or the direct reports of a manager, join with a role. */
export function GroupRulesSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const toast = useToast();
  const res = useResource(`group-rules:${projectId}`, () => listGroupRules(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [kind, setKind] = useState<GroupRuleKind>("department");
  const editable = canManage && !denied;
  const items = res.data ?? [];

  function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const data = new FormData(form);
    const refId = String(data.get("ref") ?? "").trim();
    if (!isUuid(refId)) return toast.show("The reference id must be an id like 123e4567-e89b-12d3-a456-426614174000.", "error");
    void run(() => addGroupRule(projectId, kind, refId, String(data.get("role") ?? "member"))).then((ok) => ok && form.reset());
  }

  return (
    <SectionFrame title="Access rules" id="pm-gr-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      <p className="pm-muted">Rules add people to the project automatically. Members they grant leave when the rule is removed.</p>
      {items.length === 0 && <EmptyState title="No access rules" />}
      <ul className="pm-plain-list">
        {items.map((r) => (
          <li key={r.id} className="pm-row-between pm-card">
            <span>
              <strong>{humanize(r.kind)}</strong> <span className="pm-mono">{r.ref_id}</span> as <span className="pm-chip">{r.role_key}</span>
              <span className="pm-muted"> {r.refreshed_at ? `refreshed ${formatDateTime(r.refreshed_at)}` : "not refreshed yet"}</span>
            </span>
            {editable && (
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => removeGroupRule(projectId, r.id))} aria-label={`Remove rule ${r.ref_id}`}>
                Remove
              </button>
            )}
          </li>
        ))}
      </ul>
      {editable && (
        <>
          <form className="pm-inline-form" onSubmit={add}>
            <label className="pm-sr-only" htmlFor="gr-kind">
              Rule kind
            </label>
            <select id="gr-kind" value={kind} onChange={(e) => setKind(e.target.value as GroupRuleKind)}>
              {KINDS.map((k) => (
                <option key={k} value={k}>
                  {humanize(k)}
                </option>
              ))}
            </select>
            <label className="pm-sr-only" htmlFor="gr-ref">
              Reference id
            </label>
            <input id="gr-ref" name="ref" placeholder="Department, team or manager id" required spellCheck={false} />
            <label className="pm-sr-only" htmlFor="gr-role">
              Role
            </label>
            <select id="gr-role" name="role" defaultValue="member">
              <option value="manager">manager</option>
              <option value="member">member</option>
              <option value="viewer">viewer</option>
            </select>
            <button type="submit" className="pm-btn">
              Add rule
            </button>
            <button type="button" className="pm-btn" onClick={() => void run(() => refreshGroupRules(projectId))}>
              Refresh now
            </button>
          </form>
          <p className="pm-muted">
            Paste the id of the department or team (or, for reports of, the manager user id). There is no directory of departments in this screen yet, so ids come from the HR
            screens or the API.
          </p>
        </>
      )}
    </SectionFrame>
  );
}
