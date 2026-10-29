import type { CreatedPublicLink, GitIntegration, GitIntegrationWithSecret, GitProvider, PublicLink, TaskLink, Webhook, WebhookDelivery, WebhookFormat, WebhookWithSecret } from "../types/settings";
import { del, getData, getPage, patch, post } from "./client";
import { seg } from "./query";

const list = async <T>(path: string): Promise<T[]> => (await getData<T[] | null>(path)) ?? [];
const hooks = (projectId: string) => `/projects/${seg(projectId)}/webhooks`;

export function listWebhooks(projectId: string): Promise<Webhook[]> {
  return list<Webhook>(hooks(projectId));
}

export function createWebhook(projectId: string, name: string, url: string, format: WebhookFormat, eventTypes: string[]): Promise<WebhookWithSecret> {
  return post<WebhookWithSecret>(hooks(projectId), { name, url, format, event_types: eventTypes });
}

export function setWebhookActive(projectId: string, id: string, active: boolean): Promise<Webhook> {
  return patch<Webhook>(`${hooks(projectId)}/${seg(id)}`, { active });
}

export function deleteWebhook(projectId: string, id: string): Promise<unknown> {
  return del(`${hooks(projectId)}/${seg(id)}`);
}

export function rotateWebhookSecret(projectId: string, id: string): Promise<WebhookWithSecret> {
  return post<WebhookWithSecret>(`${hooks(projectId)}/${seg(id)}/rotate-secret`);
}

export function testWebhook(projectId: string, id: string): Promise<WebhookDelivery> {
  return post<WebhookDelivery>(`${hooks(projectId)}/${seg(id)}/test`);
}

export async function listDeliveries(projectId: string, id: string): Promise<WebhookDelivery[]> {
  return (await getPage<WebhookDelivery>(`${hooks(projectId)}/${seg(id)}/deliveries`, { limit: 20 })).items;
}

export function redeliver(projectId: string, id: string, deliveryId: string): Promise<unknown> {
  return post(`${hooks(projectId)}/${seg(id)}/deliveries/${seg(deliveryId)}/redeliver`);
}

export function listPublicLinks(projectId: string): Promise<PublicLink[]> {
  return list<PublicLink>(`/projects/${seg(projectId)}/public-links`);
}

export function createPublicLink(projectId: string, showDescriptions: boolean): Promise<CreatedPublicLink> {
  return post<CreatedPublicLink>(`/projects/${seg(projectId)}/public-links`, { show_descriptions: showDescriptions });
}

export function revokePublicLink(projectId: string, id: string): Promise<unknown> {
  return del(`/projects/${seg(projectId)}/public-links/${seg(id)}`);
}

const gits = (projectId: string) => `/projects/${seg(projectId)}/integrations`;

export function listGitIntegrations(projectId: string): Promise<GitIntegration[]> {
  return list<GitIntegration>(gits(projectId));
}

export function createGitIntegration(projectId: string, provider: GitProvider, name: string): Promise<GitIntegrationWithSecret> {
  return post<GitIntegrationWithSecret>(gits(projectId), { provider, name });
}

export function rotateGitSecret(projectId: string, id: string): Promise<GitIntegrationWithSecret> {
  return post<GitIntegrationWithSecret>(`${gits(projectId)}/${seg(id)}/rotate-secret`);
}

export function deleteGitIntegration(projectId: string, id: string): Promise<unknown> {
  return del(`${gits(projectId)}/${seg(id)}`);
}

export function listTaskLinks(taskId: string): Promise<TaskLink[]> {
  return list<TaskLink>(`/tasks/${seg(taskId)}/links`);
}
