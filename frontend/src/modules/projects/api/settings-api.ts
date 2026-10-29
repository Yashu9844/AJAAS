import type { GroupRule, GroupRuleKind, Milestone } from "../types/settings";
import type { Label } from "../types/project";
import type { CustomFieldDefinition, CustomFieldType } from "../types/task";
import { del, getData, patch, post } from "./client";
import { seg } from "./query";

const list = async <T>(path: string): Promise<T[]> => (await getData<T[] | null>(path)) ?? [];

export function listMilestones(projectId: string): Promise<Milestone[]> {
  return list<Milestone>(`/projects/${seg(projectId)}/milestones`);
}

export function createMilestone(projectId: string, name: string, dueOn: string): Promise<Milestone> {
  return post<Milestone>(`/projects/${seg(projectId)}/milestones`, { name, due_on: dueOn || undefined });
}

export function setMilestoneStatus(projectId: string, id: string, status: "open" | "reached"): Promise<Milestone> {
  return patch<Milestone>(`/projects/${seg(projectId)}/milestones/${seg(id)}`, { status });
}

export function deleteMilestone(projectId: string, id: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/milestones/${seg(id)}`);
}

export function listFieldDefinitions(projectId: string): Promise<CustomFieldDefinition[]> {
  return list<CustomFieldDefinition>(`/projects/${seg(projectId)}/custom-fields`);
}

export function createFieldDefinition(projectId: string, name: string, type: CustomFieldType, options: string[]): Promise<CustomFieldDefinition> {
  return post<CustomFieldDefinition>(`/projects/${seg(projectId)}/custom-fields`, { name, type, options: options.length ? options : undefined });
}

export function updateFieldDefinition(projectId: string, id: string, changes: { name?: string; options?: string[] }): Promise<CustomFieldDefinition> {
  return patch<CustomFieldDefinition>(`/projects/${seg(projectId)}/custom-fields/${seg(id)}`, changes);
}

export function deleteFieldDefinition(projectId: string, id: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/custom-fields/${seg(id)}`);
}

export function listGroupRules(projectId: string): Promise<GroupRule[]> {
  return list<GroupRule>(`/projects/${seg(projectId)}/group-rules`);
}

export function addGroupRule(projectId: string, kind: GroupRuleKind, refId: string, role: string): Promise<GroupRule> {
  return post<GroupRule>(`/projects/${seg(projectId)}/group-rules`, { kind, ref_id: refId, role });
}

export function removeGroupRule(projectId: string, id: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/group-rules/${seg(id)}`);
}

export function refreshGroupRules(projectId: string): Promise<unknown> {
  return post(`/projects/${seg(projectId)}/group-rules/refresh`);
}

export function listTenantLabels(): Promise<Label[]> {
  return list<Label>("/project-labels");
}

export function createTenantLabel(name: string, color: string): Promise<Label> {
  return post<Label>("/project-labels", { name, color });
}

export function updateTenantLabel(id: string, name: string, color: string): Promise<Label> {
  return patch<Label>(`/project-labels/${seg(id)}`, { name, color });
}

export function deleteTenantLabel(id: string): Promise<unknown> {
  return del(`/project-labels/${seg(id)}`);
}
