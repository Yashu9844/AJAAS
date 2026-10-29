/** Route helpers. Every internal link is built here from validated parts. */

export const ROUTES = {
  signIn: "/workspace/sign-in",
  myWork: "/workspace",
  projects: "/workspace/projects",
} as const;

const ID_RE = /^[0-9a-fA-F-]{36}$/;
const TICKET_RE = /^[A-Za-z][A-Za-z0-9]{1,9}-[0-9]+$/;

export type ProjectTab = "board" | "list" | "calendar" | "backlog" | "settings";

export function isProjectId(s: string): boolean {
  return ID_RE.test(s);
}

export function projectPath(projectId: string, tab: ProjectTab = "board"): string {
  return isProjectId(projectId) ? `/workspace/projects/${projectId}/${tab}` : ROUTES.projects;
}

export function taskPath(ticket: string): string {
  return TICKET_RE.test(ticket) ? `/workspace/tasks/${ticket}` : ROUTES.myWork;
}

/** Only same-site workspace paths are accepted as a post-sign-in destination (no open redirect). */
export function safeNextPath(next: string | null | undefined): string {
  if (!next || !next.startsWith("/workspace") || next.startsWith("//") || next.includes("\\") || next.includes("://")) return ROUTES.myWork;
  if (next.startsWith("/workspace/sign-in")) return ROUTES.myWork;
  return next;
}
