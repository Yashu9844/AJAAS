"use client";

import { ACTION_CHOICES, conditionChoices, type RuleForm } from "../../lib/automation-form";
import type { Column, Label, Member } from "../../types/project";
import { RULE_TRIGGERS, type RuleTrigger } from "../../types/settings";
import { RuleValueInput } from "./RuleValueInput";

interface Props {
  idPrefix: string;
  form: RuleForm;
  columns: Column[];
  labels: Label[];
  members: Member[];
  onChange: (next: RuleForm) => void;
}

/** The fields of the simple rule form: name, trigger, one optional condition and one action. */
export function RuleFormFields({ idPrefix, form, columns, labels, members, onChange }: Props) {
  const set = (patch: Partial<RuleForm>) => onChange({ ...form, ...patch });
  const conditions = conditionChoices(form.trigger);
  const condition = conditions.find((c) => c.value === form.conditionField);
  const action = ACTION_CHOICES.find((a) => a.value === form.actionType) ?? ACTION_CHOICES[0];
  const ctx = { columns, labels, members };
  return (
    <>
      <label className="pm-field">
        <span>Rule name</span>
        <input value={form.name} onChange={(e) => set({ name: e.target.value })} maxLength={120} required />
      </label>
      <label className="pm-field">
        <span>When</span>
        <select value={form.trigger} onChange={(e) => set({ trigger: e.target.value as RuleTrigger, conditionField: "", conditionValue: "" })}>
          {RULE_TRIGGERS.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </label>
      <div className="pm-inline-form">
        <label className="pm-field">
          <span>If (optional)</span>
          <select value={form.conditionField} onChange={(e) => set({ conditionField: e.target.value, conditionValue: "" })}>
            <option value="">No condition</option>
            {conditions.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
        {condition && <RuleValueInput id={`${idPrefix}-cond-value`} label="Condition value" kind={condition.kind} value={form.conditionValue} onChange={(v) => set({ conditionValue: v })} {...ctx} />}
      </div>
      <div className="pm-inline-form">
        <label className="pm-field">
          <span>Then</span>
          <select value={form.actionType} onChange={(e) => set({ actionType: e.target.value, actionValue: "" })}>
            {ACTION_CHOICES.map((a) => (
              <option key={a.value} value={a.value}>
                {a.label}
              </option>
            ))}
          </select>
        </label>
        <RuleValueInput id={`${idPrefix}-action-value`} label="Action value" kind={action.kind} value={form.actionValue} onChange={(v) => set({ actionValue: v })} {...ctx} />
      </div>
    </>
  );
}
