/** Route helpers. Every internal link is built here from validated parts. */

export const ROUTES = {
  signIn: "/workspace/sign-in",
  myWork: "/workspace",
  projects: "/workspace/projects",
} as const;

const ID_RE = /^[0-9a-fA-F-]{36}$/;
const TICKET_RE = /^[A-Za-z][A-Za-z0-9]{1,9}-[0-9]+$/;

export type ProjectTab = "board" | "list" | "calendar" | "gantt" | "backlog" | "settings";

export function isProjectId(s: string): boolean {
  return ID_RE.test(s);
}

export function projectPath(projectId: string, tab: ProjectTab = "board"): string {
  return isProjectId(projectId) ? `/workspace/projects/${projectId}/${tab}` : ROUTES.projects;
}

export function taskPath(ticket: string): string {
  return TICKET_RE.test(ticket) ? `/workspace/tasks/${ticket}` : ROUTES.myWork;
}

const SLUG_PART = /^[a-z0-9-]{1,63}$/;
const HOST_PART = /^[a-z0-9.-]{1,253}$/i;
const PORT_PART = /^[0-9]{0,5}$/;

/** URL a git provider must call: <api base>/api/v1/integrations/{provider}/{id}. Null when a part is not well formed. */
export function integrationPayloadUrl(apiBase: string | null, provider: string, integrationId: string): string | null {
  if (!apiBase || !["github", "gitea"].includes(provider) || !ID_RE.test(integrationId)) return null;
  return `${apiBase}/api/v1/integrations/${provider}/${integrationId}`;
}

/**
 * Address of the anonymous public project page. The API needs the tenant host, so the page URL carries the tenant slug
 * and the API host and port in its query (none of them secret); the token is the secret and travels in the path.
 */
export function publicPageUrl(origin: string, token: string, slug: string, host: string, port: string): string | null {
  if (!/^[A-Za-z0-9_-]{16,128}$/.test(token) || !SLUG_PART.test(slug) || !HOST_PART.test(host) || !PORT_PART.test(port)) return null;
  const q = new URLSearchParams({ t: slug, h: host });
  if (port) q.set("p", port);
  return `${origin}/public/projects/${token}?${q.toString()}`;
}

export interface PublicTarget {
  slug: string;
  host: string;
  port: string;
}

/** Reads and validates the tenant and API address from the public page query; null (shown as "not found") when missing or malformed. */
export function parsePublicTarget(params: { get(name: string): string | null }): PublicTarget | null {
  const slug = params.get("t") ?? "";
  const host = params.get("h") ?? "";
  const port = params.get("p") ?? "";
  if (!SLUG_PART.test(slug) || !HOST_PART.test(host) || !PORT_PART.test(port)) return null;
  return { slug, host, port };
}

/** Only same-site workspace paths are accepted as a post-sign-in destination (no open redirect). */
export function safeNextPath(next: string | null | undefined): string {
  if (!next || !next.startsWith("/workspace") || next.startsWith("//") || next.includes("\\") || next.includes("://")) return ROUTES.myWork;
  if (next.startsWith("/workspace/sign-in")) return ROUTES.myWork;
  return next;
}
