import type { Priority } from "./common";

export interface Milestone {
  id: string;
  name: string;
  due_on: string | null;
  status: "open" | "reached";
  total_tasks: number;
  done_tasks: number;
}

export type GroupRuleKind = "department" | "team" | "reports_of";

export interface GroupRule {
  id: string;
  kind: GroupRuleKind;
  ref_id: string;
  role_key: string;
  refreshed_at: string | null;
}

export const RULE_TRIGGERS = ["task.created", "task.moved", "task.assigned", "task.completed", "task.commented", "task.due_changed", "task.pr_merged"] as const;
export type RuleTrigger = (typeof RULE_TRIGGERS)[number];

export interface RuleCondition {
  field: string;
  value: string;
}

export interface RuleAction {
  type: string;
  column_id?: string;
  priority?: Priority;
  label_id?: string;
  user_id?: string;
  text?: string;
  days?: number;
}

export interface Rule {
  id: string;
  name: string;
  trigger: string;
  conditions: RuleCondition[];
  actions: RuleAction[];
  active: boolean;
  run_count: number;
  last_run_at: string | null;
  consecutive_failures: number;
  flagged: string | null;
  version: number;
}

export interface RuleInput {
  name: string;
  trigger: string;
  conditions: RuleCondition[];
  actions: RuleAction[];
  active: boolean;
}

export type RecurrenceFrequency = "daily" | "weekly" | "monthly";

export interface Recurrence {
  id: string;
  template_task_id: string;
  frequency: RecurrenceFrequency;
  interval: number;
  weekday: number | null;
  day_of_month: number | null;
  next_run_at: string;
  until_on: string | null;
  active: boolean;
  flagged: string | null;
  last_run_at: string | null;
  version: number;
}

export interface RecurrenceInput {
  template_task_id: string;
  frequency: RecurrenceFrequency;
  interval: number;
  weekday?: number;
  day_of_month?: number;
}

export interface PublicLink {
  id: string;
  show_descriptions: boolean;
  created_at: string;
  revoked_at: string | null;
}

/** The create response carries the secret token once. */
export interface CreatedPublicLink extends PublicLink {
  token: string;
  path: string;
}

export type WebhookFormat = "generic" | "slack" | "discord";

export interface Webhook {
  id: string;
  name: string;
  url: string;
  format: WebhookFormat;
  event_types: string[];
  active: boolean;
  secret_hint: string;
  failure_count: number;
  disabled_at: string | null;
}

/** Create and rotate responses carry the signing secret once. */
export interface WebhookWithSecret extends Webhook {
  secret: string;
}

export interface WebhookDelivery {
  id: string;
  event_type: string;
  status: string;
  attempts: number;
  response_status: number | null;
  last_error: string | null;
  created_at: string;
  delivered_at: string | null;
}

export type GitProvider = "github" | "gitea";

export interface GitIntegration {
  id: string;
  provider: GitProvider;
  name: string;
  secret_hint: string;
  active: boolean;
}

/** Create and rotate responses carry the secret once (create also returns the incoming path). */
export interface GitIntegrationWithSecret extends GitIntegration {
  secret: string;
  path?: string;
}

export interface TaskLink {
  id: string;
  provider: string;
  kind: string;
  external_id: string;
  url: string;
  state: string;
}

export interface RuleRun {
  id: string;
  status: string;
  task_id: string | null;
  error_code: string | null;
  action_count: number;
  created_at: string;
}

export interface RuleTestAction {
  type: string;
  would_run: boolean;
  reason?: string;
}

export interface RuleTestResult {
  matches: boolean;
  failed_conditions: string[];
  assumed_conditions: string[];
  creator_allowed: boolean;
  actions: RuleTestAction[];
}

export interface ProjectRole {
  id: string;
  name: string;
  description: string;
  permissions: string[];
}

export interface ProjectPermissions {
  permissions: string[];
}
