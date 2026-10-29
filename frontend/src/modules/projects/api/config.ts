/**
 * Where the API lives. The tenant is addressed by the Host header, which a browser cannot set, so tenant calls go to
 * http(s)://<slug>.<host>:<port>. Host and port are not secrets; they are kept in localStorage so the sign-in form can
 * point at another server. Defaults come from NEXT_PUBLIC_API_HOST / NEXT_PUBLIC_API_PORT (public by definition).
 */

export interface Endpoint {
  host: string;
  port: string;
}

const KEY = "jaas.workspace.endpoint";
const HOST_RE = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/i;
const PORT_RE = /^[0-9]{1,5}$/;
const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

export function defaultEndpoint(): Endpoint {
  return {
    host: process.env.NEXT_PUBLIC_API_HOST ?? "localhost",
    port: process.env.NEXT_PUBLIC_API_PORT ?? "8080",
  };
}

export function isValidEndpoint(ep: Endpoint): boolean {
  return HOST_RE.test(ep.host) && (ep.port === "" || PORT_RE.test(ep.port));
}

export function isValidSlug(slug: string): boolean {
  return SLUG_RE.test(slug);
}

export function loadEndpoint(): Endpoint {
  const fallback = defaultEndpoint();
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return fallback;
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed === "object" && parsed !== null) {
      const o = parsed as Record<string, unknown>;
      const ep = { host: typeof o.host === "string" ? o.host : fallback.host, port: typeof o.port === "string" ? o.port : fallback.port };
      return isValidEndpoint(ep) ? ep : fallback;
    }
  } catch {
    // Storage blocked or corrupt: use the defaults.
  }
  return fallback;
}

export function saveEndpoint(ep: Endpoint): void {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(ep));
  } catch {
    // Storage blocked: the endpoint is then only valid for this page load.
  }
}

/** Base URL for one tenant, for example http://uitest.localhost:8091. Slug and endpoint must be validated first. */
export function tenantBaseUrl(slug: string, ep: Endpoint, protocol: string = "http:"): string {
  const port = ep.port ? `:${ep.port}` : "";
  return `${protocol}//${slug}.${ep.host}${port}`;
}
