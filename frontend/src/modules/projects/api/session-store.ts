import { identityFromToken } from "../lib/jwt";
import { isValidEndpoint, isValidSlug, loadEndpoint, saveEndpoint, tenantBaseUrl, type Endpoint } from "./config";
import { ApiError, isApiError } from "./errors";
import { createHttp } from "./http";

/**
 * Session state. The access token lives only in this module's memory. The refresh token is kept in sessionStorage
 * (tab-scoped, gone when the tab closes) so a reload does not sign the person out; see docs notes-ui.md for the risks.
 */

export type SessionStatus = "booting" | "signed-in" | "signed-out";

export interface SessionState {
  status: SessionStatus;
  slug: string;
  userId: string;
  email: string;
  roles: string[];
  accessToken: string | null;
}

const STORAGE_KEY = "jaas.ws.session";
const REFRESH_MARGIN_MS = 60_000;
const MIN_REFRESH_DELAY_MS = 5_000;
const BOOTING: SessionState = { status: "booting", slug: "", userId: "", email: "", roles: [], accessToken: null };
const SIGNED_OUT: SessionState = { ...BOOTING, status: "signed-out" };

interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_at?: string;
}

let state: SessionState = BOOTING;
let endpoint: Endpoint | null = null;
let refreshToken: string | null = null;
let refreshInFlight: Promise<boolean> | null = null;
let timer: ReturnType<typeof setTimeout> | null = null;
const listeners = new Set<() => void>();

function publish(next: SessionState): void {
  state = next;
  listeners.forEach((l) => l());
}

export function subscribeSession(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getSessionState(): SessionState {
  return state;
}

export function getServerSessionState(): SessionState {
  return BOOTING;
}

export function currentBaseUrl(): string | null {
  if (!state.slug || !endpoint) return null;
  return tenantBaseUrl(state.slug, endpoint, window.location.protocol === "https:" ? "https:" : "http:");
}

const authHttp = createHttp({ baseUrl: currentBaseUrl, accessToken: () => null, refresh: async () => false, onSessionLost: () => undefined });

function persist(): void {
  try {
    if (refreshToken && state.slug) window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ slug: state.slug, refresh: refreshToken }));
    else window.sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // sessionStorage unavailable: the session then survives only until reload.
  }
}

function clearTimer(): void {
  if (timer) clearTimeout(timer);
  timer = null;
}

function scheduleRefresh(expiresAtMs: number | null): void {
  clearTimer();
  if (expiresAtMs === null) return;
  const delay = Math.max(expiresAtMs - Date.now() - REFRESH_MARGIN_MS, MIN_REFRESH_DELAY_MS);
  timer = setTimeout(() => void refreshTokens(), delay);
}

function applyTokens(slug: string, tokens: TokenPair): boolean {
  const id = identityFromToken(tokens.access_token);
  if (!id) return false;
  refreshToken = tokens.refresh_token;
  publish({ status: "signed-in", slug, userId: id.userId, email: id.email, roles: id.roles, accessToken: tokens.access_token });
  persist();
  scheduleRefresh(id.expiresAtMs);
  return true;
}

function endSession(): void {
  clearTimer();
  refreshToken = null;
  persist();
  publish(SIGNED_OUT);
}

/** Refreshes the tokens; concurrent callers share one request because refresh tokens rotate. */
export function refreshTokens(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;
  const token = refreshToken;
  const slug = state.slug;
  if (!token || !slug) return Promise.resolve(false);
  refreshInFlight = authHttp
    .request<TokenPair>("/auth/refresh", { method: "POST", body: { refresh_token: token }, noAuth: true })
    .then((r) => applyTokens(slug, r.data))
    .catch((e: unknown) => {
      if (isApiError(e) && e.status !== 0 && e.status !== 429) endSession();
      return false;
    })
    .finally(() => {
      refreshInFlight = null;
    });
  return refreshInFlight;
}

/** Signs in. Throws ApiError (INVALID_INPUT for a malformed slug or server address). */
export async function signIn(slug: string, email: string, password: string, ep: Endpoint): Promise<void> {
  const cleanSlug = slug.trim().toLowerCase();
  if (!isValidSlug(cleanSlug) || !isValidEndpoint(ep)) {
    throw new ApiError(400, "INVALID_INPUT", "Check the organisation name and the server address.");
  }
  saveEndpoint(ep);
  endpoint = ep;
  publish({ ...SIGNED_OUT, slug: cleanSlug });
  const res = await authHttp.request<TokenPair>("/auth/login", {
    method: "POST",
    body: { tenant_slug: cleanSlug, email: email.trim(), password },
    noAuth: true,
  });
  if (!applyTokens(cleanSlug, res.data)) throw new ApiError(500, "BAD_RESPONSE", "The server answered with something unexpected.");
}

/** Restores a session after a reload from the tab's refresh token. */
export async function restoreSession(): Promise<void> {
  endpoint = loadEndpoint();
  try {
    const raw = window.sessionStorage.getItem(STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : null;
    if (typeof parsed === "object" && parsed !== null) {
      const o = parsed as Record<string, unknown>;
      if (typeof o.slug === "string" && typeof o.refresh === "string" && isValidSlug(o.slug)) {
        refreshToken = o.refresh;
        publish({ ...SIGNED_OUT, slug: o.slug });
        if (await refreshTokens()) return;
      }
    }
  } catch {
    // Unreadable storage: fall through to signed out.
  }
  endSession();
}

/** Ends the session on the server (best effort) and locally. */
export async function signOut(): Promise<void> {
  const token = state.accessToken;
  const base = currentBaseUrl();
  endSession();
  if (!token || !base) return;
  try {
    await createHttp({ baseUrl: () => base, accessToken: () => token, refresh: async () => false, onSessionLost: () => undefined }).request(
      "/auth/logout",
      { method: "POST" },
    );
  } catch {
    // Local sign-out already happened; a failed server call only leaves the server session to expire.
  }
}

/** Called by the HTTP layer when a refresh failed after a 401. */
export function markSessionLost(): void {
  endSession();
}

export function currentAccessToken(): string | null {
  return state.accessToken;
}
