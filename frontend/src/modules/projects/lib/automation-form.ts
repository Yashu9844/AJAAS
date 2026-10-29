import type { Priority } from "../types/common";
import type { Recurrence, RecurrenceFrequency, RecurrenceInput, RuleAction } from "../types/settings";

/** Pure helpers for the automation, recurrence and webhook forms: payload building and validation. */

export type ValueKind = "column" | "category" | "priority" | "label" | "member" | "text" | "days";

export interface FieldChoice {
  value: string;
  label: string;
  kind: ValueKind;
}

const BASE_CONDITIONS: FieldChoice[] = [
  { value: "column_id", label: "Column is", kind: "column" },
  { value: "column_category", label: "Column category is", kind: "category" },
  { value: "priority", label: "Priority is", kind: "priority" },
  { value: "label_id", label: "Has label", kind: "label" },
  { value: "assignee_id", label: "Assigned to", kind: "member" },
];

/** Condition fields offered for a trigger (moves and assignments add their own). */
export function conditionChoices(trigger: string): FieldChoice[] {
  if (trigger === "task.moved") {
    return [{ value: "to_column_id", label: "Moved to column", kind: "column" }, { value: "from_column_id", label: "Moved from column", kind: "column" }, ...BASE_CONDITIONS];
  }
  if (trigger === "task.assigned") return [{ value: "assigned_user_id", label: "Assigned user is", kind: "member" }, ...BASE_CONDITIONS];
  return BASE_CONDITIONS;
}

export const ACTION_CHOICES: FieldChoice[] = [
  { value: "move_to_column", label: "Move to column", kind: "column" },
  { value: "set_priority", label: "Set priority", kind: "priority" },
  { value: "add_label", label: "Add label", kind: "label" },
  { value: "assign_user", label: "Assign user", kind: "member" },
  { value: "add_comment", label: "Add comment", kind: "text" },
  { value: "set_due_in_days", label: "Set due date in days", kind: "days" },
];

const ACTION_KEY: Record<string, keyof RuleAction> = {
  move_to_column: "column_id",
  set_priority: "priority",
  add_label: "label_id",
  assign_user: "user_id",
  add_comment: "text",
  set_due_in_days: "days",
};

/** Builds one action object from its type and the chosen value. Returns null when the value is unusable. */
export function buildAction(type: string, value: string): RuleAction | null {
  const key = ACTION_KEY[type];
  if (!key || !value.trim()) return null;
  if (key === "days") {
    const days = Number(value);
    return Number.isInteger(days) && days >= 0 && days <= 365 ? { type, days } : null;
  }
  if (key === "priority") return { type, priority: value as Priority };
  return { type, [key]: value.trim() } as RuleAction;
}

export type FormResult<T> = { ok: true; input: T } | { ok: false; error: string };

/** The value an action carries, as the form control holds it. */
export function actionValueOf(a: RuleAction): string {
  const v = a.column_id ?? a.priority ?? a.label_id ?? a.user_id ?? a.text ?? (a.days !== undefined ? String(a.days) : "");
  return String(v);
}

export interface RecurrenceForm {
  templateId: string;
  frequency: RecurrenceFrequency;
  interval: string;
  weekday: string;
  dayOfMonth: string;
}

export function buildRecurrenceInput(f: RecurrenceForm): FormResult<RecurrenceInput> {
  const interval = Number(f.interval || "1");
  if (!f.templateId) return { ok: false, error: "Choose the template task." };
  if (!Number.isInteger(interval) || interval < 1 || interval > 52) return { ok: false, error: "The interval must be 1 to 52." };
  const input: RecurrenceInput = { template_task_id: f.templateId, frequency: f.frequency, interval };
  if (f.frequency === "weekly") input.weekday = Number(f.weekday || "1");
  if (f.frequency === "monthly") {
    const day = Number(f.dayOfMonth || "1");
    if (!Number.isInteger(day) || day < 1 || day > 31) return { ok: false, error: "The day of the month must be 1 to 31." };
    input.day_of_month = day;
  }
  return { ok: true, input };
}

/** Fills the recurrence form from a stored recurrence. */
export function recurrenceToForm(r: Recurrence): RecurrenceForm {
  return {
    templateId: r.template_task_id,
    frequency: r.frequency,
    interval: String(r.interval),
    weekday: String(r.weekday ?? 1),
    dayOfMonth: String(r.day_of_month ?? 1),
  };
}

/** The PATCH body of a recurrence edit (the template task cannot be changed). */
export function buildRecurrenceEdit(f: RecurrenceForm): FormResult<{ frequency: RecurrenceFrequency; interval: number; weekday?: number; day_of_month?: number }> {
  const built = buildRecurrenceInput(f);
  if (!built.ok) return built;
  const { frequency, interval, weekday, day_of_month } = built.input;
  return { ok: true, input: { frequency, interval, ...(weekday !== undefined ? { weekday } : {}), ...(day_of_month !== undefined ? { day_of_month } : {}) } };
}

const EVENT_RE = /^(\*|[a-z][a-z0-9_]*(\.[a-z0-9_]+)+)$/;

/** Splits "task.created, task.moved" into event names; an empty text means all events ("*"). */
export function parseEventTypes(text: string): FormResult<string[]> {
  const parts = text.split(/[\s,]+/).filter(Boolean);
  if (parts.length === 0) return { ok: true, input: ["*"] };
  if (parts.length > 30) return { ok: false, error: "At most 30 event types." };
  const bad = parts.find((p) => !EVENT_RE.test(p));
  return bad ? { ok: false, error: `"${bad}" is not an event name such as task.created.` } : { ok: true, input: [...new Set(parts)] };
}

/** Client-side check that mirrors the server rule: https, port 443 or 8443, no credentials. The server decides. */
export function validateWebhookUrl(raw: string): string | null {
  let u: URL;
  try {
    u = new URL(raw.trim());
  } catch {
    return "Enter a full https:// address.";
  }
  if (u.protocol !== "https:") return "Only https addresses are accepted.";
  if (u.username || u.password) return "The address must not contain credentials.";
  if (u.port && u.port !== "443" && u.port !== "8443") return "Only ports 443 and 8443 are accepted.";
  return null;
}

/** A GUID check for the reference ids of group rules (the API has no department directory yet). */
export function isUuid(s: string): boolean {
  return /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(s.trim());
}

/** Splits comma or line separated select options, trimmed, unique, non-empty. */
export function parseOptions(text: string): string[] {
  return [...new Set(text.split(/[\n,]/).map((o) => o.trim()).filter(Boolean))];
}
