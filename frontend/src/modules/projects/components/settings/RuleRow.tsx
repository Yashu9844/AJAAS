"use client";

import { useState } from "react";
import { deleteRule, setRuleActive, updateRule } from "../../api/automation-api";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useToast } from "../../hooks/use-toast";
import { buildRuleFromDraft, ruleToDraft, type RuleDraft } from "../../lib/rule-draft";
import { formatDateTime } from "../../lib/dates";
import type { Column, Label, Member } from "../../types/project";
import type { Rule } from "../../types/settings";
import { RuleFormFields } from "./RuleFormFields";
import { RuleRuns } from "./RuleRuns";
import { RuleTestPanel } from "./RuleTestPanel";

interface Props {
  projectId: string;
  rule: Rule;
  editable: boolean;
  columns: Column[];
  labels: Label[];
  members: Member[];
  onChanged: () => void;
}

type Panel = "none" | "edit" | "runs" | "test";

/** One rule: status, enable/disable, edit (simple rules), runs, dry-run test and delete. */
export function RuleRow({ projectId, rule, editable, columns, labels, members, onChanged }: Props) {
  const toast = useToast();
  const { run } = useGuardedAction(onChanged);
  const [panel, setPanel] = useState<Panel>("none");
  const [draft, setDraft] = useState<RuleDraft>(ruleToDraft(rule));
  const toggle = (p: Panel) => setPanel((cur) => (cur === p ? "none" : p));

  function save() {
    const built = buildRuleFromDraft(draft, rule.active);
    if (!built.ok) return toast.show(built.error, "error");
    const { name, trigger, conditions, actions } = built.input;
    void run(() => updateRule(projectId, rule, { name, trigger, conditions, actions })).then((ok) => ok && setPanel("none"));
  }

  return (
    <li className="pm-card">
      <div className="pm-row-between">
        <span>
          <strong>{rule.name}</strong> <span className="pm-chip">{rule.trigger}</span> <span className="pm-chip">{rule.active ? "enabled" : "disabled"}</span>
          {rule.flagged && <span className="pm-chip pm-pill-warning">needs attention</span>}
          <span className="pm-muted">
            {" "}
            {rule.actions.map((a) => a.type).join(", ")} · ran {rule.run_count}x{rule.last_run_at ? `, last ${formatDateTime(rule.last_run_at)}` : ""}
          </span>
        </span>
        <span className="pm-actions">
          {editable && (
            <>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => toggle("runs")} aria-expanded={panel === "runs"}>
                Runs
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => toggle("test")} aria-expanded={panel === "test"}>
                Test
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => toggle("edit")} aria-expanded={panel === "edit"}>
                Edit
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => setRuleActive(projectId, rule, !rule.active))}>
                {rule.active ? "Disable" : "Enable"}
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteRule(projectId, rule.id))} aria-label={`Delete rule ${rule.name}`}>
                Delete
              </button>
            </>
          )}
        </span>
      </div>
      {panel === "runs" && <RuleRuns projectId={projectId} ruleId={rule.id} />}
      {panel === "test" && <RuleTestPanel projectId={projectId} rule={rule} />}
      {panel === "edit" && (
        <form
          className="pm-form pm-deliveries"
          aria-label={`Edit rule ${rule.name}`}
          onSubmit={(e) => {
            e.preventDefault();
            save();
          }}
        >
          <RuleFormFields idPrefix={`edit-${rule.id}`} draft={draft} columns={columns} labels={labels} members={members} onChange={setDraft} />
          <div className="pm-actions">
            <button type="submit" className="pm-btn pm-btn-primary pm-btn-small">
              Save rule
            </button>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => setPanel("none")}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </li>
  );
}
