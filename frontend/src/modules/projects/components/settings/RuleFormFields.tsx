"use client";

import { ACTION_CHOICES, conditionChoices } from "../../lib/automation-form";
import { actionKind, conditionKind, conditionsForTrigger, MAX_ACTIONS, MAX_CONDITIONS, type RuleDraft } from "../../lib/rule-draft";
import type { Column, Label, Member } from "../../types/project";
import { RULE_TRIGGERS } from "../../types/settings";
import { RuleValueInput } from "./RuleValueInput";

interface Props {
  idPrefix: string;
  draft: RuleDraft;
  columns: Column[];
  labels: Label[];
  members: Member[];
  onChange: (next: RuleDraft) => void;
}

const replaceAt = <T,>(list: readonly T[], index: number, item: T): T[] => list.map((x, i) => (i === index ? item : x));
const removeAt = <T,>(list: readonly T[], index: number): T[] => list.filter((_, i) => i !== index);

/** Rule editor: name, trigger, any number of conditions (all must hold) and actions (run in order). */
export function RuleFormFields({ idPrefix, draft, columns, labels, members, onChange }: Props) {
  const ctx = { columns, labels, members };
  const choices = conditionChoices(draft.trigger);
  const triggers: string[] = (RULE_TRIGGERS as readonly string[]).includes(draft.trigger) ? [...RULE_TRIGGERS] : [...RULE_TRIGGERS, draft.trigger];
  const set = (patch: Partial<RuleDraft>) => onChange({ ...draft, ...patch });

  return (
    <>
      <label className="pm-field">
        <span>Rule name</span>
        <input value={draft.name} onChange={(e) => set({ name: e.target.value })} maxLength={120} required />
      </label>
      <label className="pm-field">
        <span>When</span>
        <select value={draft.trigger} onChange={(e) => set({ trigger: e.target.value, conditions: conditionsForTrigger(e.target.value, draft.conditions) })}>
          {triggers.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </label>
      <fieldset className="pm-field pm-rule-group">
        <legend>If all of these are true ({draft.conditions.length}/{MAX_CONDITIONS})</legend>
        {draft.conditions.map((c, i) => (
          <div key={i} className="pm-inline-form">
            <label className="pm-sr-only" htmlFor={`${idPrefix}-cf-${i}`}>
              Condition {i + 1} field
            </label>
            <select id={`${idPrefix}-cf-${i}`} value={c.field} onChange={(e) => set({ conditions: replaceAt(draft.conditions, i, { field: e.target.value, value: "" }) })}>
              <option value="">Choose…</option>
              {choices.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
              {c.field && !choices.some((o) => o.value === c.field) && <option value={c.field}>{c.field}</option>}
            </select>
            <RuleValueInput id={`${idPrefix}-cv-${i}`} label={`Condition ${i + 1} value`} kind={conditionKind(draft.trigger, c.field)} value={c.value} onChange={(v) => set({ conditions: replaceAt(draft.conditions, i, { ...c, value: v }) })} {...ctx} />
            <button type="button" className="pm-btn pm-btn-small" onClick={() => set({ conditions: removeAt(draft.conditions, i) })} aria-label={`Remove condition ${i + 1}`}>
              Remove
            </button>
          </div>
        ))}
        <div>
          <button type="button" className="pm-btn pm-btn-small" disabled={draft.conditions.length >= MAX_CONDITIONS} onClick={() => set({ conditions: [...draft.conditions, { field: "", value: "" }] })}>
            Add condition
          </button>
        </div>
      </fieldset>
      <fieldset className="pm-field pm-rule-group">
        <legend>Then do ({draft.actions.length}/{MAX_ACTIONS})</legend>
        {draft.actions.map((a, i) => (
          <div key={i} className="pm-inline-form">
            {a.raw ? (
              <span className="pm-muted">
                <span className="pm-chip">{a.type}</span> kept as stored (not editable here)
              </span>
            ) : (
              <>
                <label className="pm-sr-only" htmlFor={`${idPrefix}-at-${i}`}>
                  Action {i + 1} type
                </label>
                <select id={`${idPrefix}-at-${i}`} value={a.type} onChange={(e) => set({ actions: replaceAt(draft.actions, i, { type: e.target.value, value: "" }) })}>
                  {ACTION_CHOICES.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
                <RuleValueInput id={`${idPrefix}-av-${i}`} label={`Action ${i + 1} value`} kind={actionKind(a.type)} value={a.value} onChange={(v) => set({ actions: replaceAt(draft.actions, i, { ...a, value: v }) })} {...ctx} />
              </>
            )}
            <button type="button" className="pm-btn pm-btn-small" onClick={() => set({ actions: removeAt(draft.actions, i) })} aria-label={`Remove action ${i + 1}`} disabled={draft.actions.length <= 1}>
              Remove
            </button>
          </div>
        ))}
        <div>
          <button type="button" className="pm-btn pm-btn-small" disabled={draft.actions.length >= MAX_ACTIONS} onClick={() => set({ actions: [...draft.actions, { type: "set_priority", value: "" }] })}>
            Add action
          </button>
        </div>
      </fieldset>
    </>
  );
}
