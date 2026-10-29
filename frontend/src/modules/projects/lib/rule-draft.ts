import type { Rule, RuleAction, RuleCondition, RuleInput } from "../types/settings";
import { ACTION_CHOICES, actionValueOf, buildAction, conditionChoices, type FormResult, type ValueKind } from "./automation-form";

/**
 * Editing model of an automation rule: any number of conditions and actions (the API allows 10 of each). Parts the form
 * does not know (an action type or a condition field outside the vocabulary) are kept as they are, so editing a rule never
 * drops anything it could not show.
 */

export const MAX_CONDITIONS = 10;
export const MAX_ACTIONS = 10;

export interface ConditionDraft {
  field: string;
  value: string;
}

export interface ActionDraft {
  type: string;
  value: string;
  /** The stored action when its type is unknown to the form; it is sent back unchanged. */
  raw?: RuleAction;
}

export interface RuleDraft {
  name: string;
  trigger: string;
  conditions: ConditionDraft[];
  actions: ActionDraft[];
}

export const NEW_RULE_DRAFT: RuleDraft = { name: "", trigger: "task.moved", conditions: [], actions: [{ type: "set_priority", value: "" }] };

const knownActionType = (type: string) => ACTION_CHOICES.some((a) => a.value === type);

export function ruleToDraft(rule: Pick<Rule, "name" | "trigger" | "conditions" | "actions">): RuleDraft {
  return {
    name: rule.name,
    trigger: rule.trigger,
    conditions: rule.conditions.map((c) => ({ field: c.field, value: String(c.value ?? "") })),
    actions: rule.actions.map((a) => (knownActionType(a.type) ? { type: a.type, value: actionValueOf(a) } : { type: a.type, value: "", raw: a })),
  };
}

/** The value control kind of a condition field for a trigger; unknown fields are plain text. */
export function conditionKind(trigger: string, field: string): ValueKind {
  return conditionChoices(trigger).find((c) => c.value === field)?.kind ?? "text";
}

export function actionKind(type: string): ValueKind {
  return ACTION_CHOICES.find((a) => a.value === type)?.kind ?? "text";
}

/** After a trigger change, drops the conditions that only make sense for the old trigger (unknown fields are kept). */
export function conditionsForTrigger(trigger: string, conditions: readonly ConditionDraft[]): ConditionDraft[] {
  const allKnown = new Set(["to_column_id", "from_column_id", "assigned_user_id", ...conditionChoices("task.created").map((c) => c.value)]);
  const valid = new Set(conditionChoices(trigger).map((c) => c.value));
  return conditions.filter((c) => valid.has(c.field) || !allKnown.has(c.field));
}

/** Builds the API body. Every condition needs a field and a value; every known action needs a usable value; at least one action. */
export function buildRuleFromDraft(d: RuleDraft, active: boolean): FormResult<RuleInput> {
  const name = d.name.trim();
  if (!name) return { ok: false, error: "Give the rule a name." };
  if (d.actions.length === 0) return { ok: false, error: "A rule needs at least one action." };
  if (d.conditions.length > MAX_CONDITIONS || d.actions.length > MAX_ACTIONS) return { ok: false, error: "At most 10 conditions and 10 actions." };
  const conditions: RuleCondition[] = [];
  for (const c of d.conditions) {
    if (!c.field || !c.value.trim()) return { ok: false, error: "Fill in every condition or remove it." };
    conditions.push({ field: c.field, value: c.value.trim() });
  }
  const actions: RuleAction[] = [];
  for (const a of d.actions) {
    const built = a.raw ?? buildAction(a.type, a.value);
    if (!built) return { ok: false, error: "Choose a value for every action or remove it." };
    actions.push(built);
  }
  return { ok: true, input: { name, trigger: d.trigger, conditions, actions, active } };
}
