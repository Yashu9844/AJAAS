"use client";

import { useState, type FormEvent } from "react";
import { createRule, listRules } from "../../api/automation-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useProject } from "../../hooks/use-project";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { buildRuleFromDraft, NEW_RULE_DRAFT, type RuleDraft } from "../../lib/rule-draft";
import { EmptyState } from "../ui/States";
import { RuleFormFields } from "./RuleFormFields";
import { RuleRow } from "./RuleRow";
import { SectionFrame } from "./SectionState";

/** Automation rules: list with enable/disable, edit, runs, dry-run test and delete, plus a form for a new rule. */
export function RulesSettings({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const { columns, labels, members } = useProject();
  const toast = useToast();
  const res = useResource(`rules:${projectId}`, () => listRules(projectId));
  const { run, denied } = useGuardedAction(res.reload);
  const [form, setForm] = useState<RuleDraft>(NEW_RULE_DRAFT);
  const editable = canManage && !denied;
  const items = res.data ?? [];

  function submit(e: FormEvent) {
    e.preventDefault();
    const built = buildRuleFromDraft(form, true);
    if (!built.ok) return toast.show(built.error, "error");
    void run(() => createRule(projectId, built.input)).then((ok) => ok && setForm(NEW_RULE_DRAFT));
  }

  return (
    <SectionFrame title="Automation rules" id="pm-rules-h" loading={res.loading} error={res.error} onRetry={res.reload}>
      {items.length === 0 && <EmptyState title="No automation rules" />}
      <ul className="pm-plain-list">
        {items.map((r) => (
          <RuleRow key={`${r.id}-${r.version}`} projectId={projectId} rule={r} editable={editable} columns={columns} labels={labels} members={members} onChanged={res.reload} />
        ))}
      </ul>
      {editable && (
        <form className="pm-form pm-card" onSubmit={submit} aria-label="New automation rule">
          <RuleFormFields idPrefix="rule" draft={form} columns={columns} labels={labels} members={members} onChange={setForm} />
          <div className="pm-actions">
            <button type="submit" className="pm-btn pm-btn-primary">
              Create rule
            </button>
          </div>
        </form>
      )}
    </SectionFrame>
  );
}
