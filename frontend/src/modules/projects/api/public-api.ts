import type { Page } from "../types/common";
import type { PublicProject, PublicTask } from "../types/project";
import type { PublicTarget } from "../lib/paths";
import { tenantBaseUrl } from "./config";
import { createHttp } from "./http";
import { seg } from "./query";

export interface PublicPage {
  head: Pick<PublicProject, "project" | "columns">;
  tasks: Page<PublicTask>;
}

/**
 * Reads one page of a public project. There is no session here: no token is sent and a 401 never triggers a refresh.
 * The tenant is addressed through the sub-domain of the API host, exactly like the signed-in client.
 */
export async function fetchPublicProject(target: PublicTarget, token: string, cursor: string | undefined, signal: AbortSignal): Promise<PublicPage> {
  const base = tenantBaseUrl(target.slug, { host: target.host, port: target.port }, window.location.protocol === "https:" ? "https:" : "http:");
  const http = createHttp({ baseUrl: () => base, accessToken: () => null, refresh: async () => false, onSessionLost: () => undefined });
  const res = await http.request<PublicProject>(`/public/projects/${seg(token)}`, { query: { limit: 50, cursor }, noAuth: true, signal });
  const data = res.data;
  return {
    head: { project: data.project, columns: data.columns ?? [] },
    tasks: { items: data.tasks ?? [], next: res.meta.next_cursor ?? null },
  };
}
