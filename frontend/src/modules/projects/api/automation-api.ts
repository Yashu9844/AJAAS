import type { Recurrence, RecurrenceFrequency, RecurrenceInput, Rule, RuleAction, RuleCondition, RuleInput, RuleRun, RuleTestResult } from "../types/settings";
import { del, getData, getPage, patch, post } from "./client";
import { seg } from "./query";

const list = async <T>(path: string): Promise<T[]> => (await getData<T[] | null>(path)) ?? [];

export function listRules(projectId: string): Promise<Rule[]> {
  return list<Rule>(`/projects/${seg(projectId)}/rules`);
}

export function createRule(projectId: string, input: RuleInput): Promise<Rule> {
  return post<Rule>(`/projects/${seg(projectId)}/rules`, input);
}

/** Enables or disables a rule; the version guards against a concurrent edit. */
export function setRuleActive(projectId: string, rule: Rule, active: boolean): Promise<Rule> {
  return patch<Rule>(`/projects/${seg(projectId)}/rules/${seg(rule.id)}`, { active, version: rule.version });
}

export function deleteRule(projectId: string, id: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/rules/${seg(id)}`);
}

export function listRecurrences(projectId: string): Promise<Recurrence[]> {
  return list<Recurrence>(`/projects/${seg(projectId)}/recurrences`);
}

export function createRecurrence(projectId: string, input: RecurrenceInput): Promise<Recurrence> {
  return post<Recurrence>(`/projects/${seg(projectId)}/recurrences`, input);
}

export function setRecurrenceActive(projectId: string, rec: Recurrence, active: boolean): Promise<Recurrence> {
  return patch<Recurrence>(`/projects/${seg(projectId)}/recurrences/${seg(rec.id)}`, { active, version: rec.version });
}

export function runRecurrenceNow(projectId: string, id: string): Promise<unknown> {
  return post(`/projects/${seg(projectId)}/recurrences/${seg(id)}/run-now`);
}

export function deleteRecurrence(projectId: string, id: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/recurrences/${seg(id)}`);
}

export interface RuleEdit {
  name: string;
  trigger: string;
  conditions: RuleCondition[];
  actions: RuleAction[];
}

/** Edits a rule; any edit clears its failure flag on the server and the version guards against a concurrent edit. */
export function updateRule(projectId: string, rule: Rule, edit: RuleEdit): Promise<Rule> {
  return patch<Rule>(`/projects/${seg(projectId)}/rules/${seg(rule.id)}`, { ...edit, version: rule.version });
}

export async function listRuleRuns(projectId: string, ruleId: string): Promise<RuleRun[]> {
  return (await getPage<RuleRun>(`/projects/${seg(projectId)}/rules/${seg(ruleId)}/runs`, { limit: 20 })).items;
}

/** Dry run against one task: nothing changes; the answer says which actions would run. */
export function testRule(projectId: string, ruleId: string, taskId: string): Promise<RuleTestResult> {
  return post<RuleTestResult>(`/projects/${seg(projectId)}/rules/${seg(ruleId)}/test`, { task_id: taskId });
}

export interface RecurrenceEdit {
  frequency: RecurrenceFrequency;
  interval: number;
  weekday?: number;
  day_of_month?: number;
}

export function updateRecurrence(projectId: string, rec: Recurrence, edit: RecurrenceEdit): Promise<Recurrence> {
  return patch<Recurrence>(`/projects/${seg(projectId)}/recurrences/${seg(rec.id)}`, { ...edit, version: rec.version });
}
