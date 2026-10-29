export interface TokenIdentity {
  userId: string;
  email: string;
  roles: string[];
  expiresAtMs: number | null;
}

function decodePayload(token: string): Record<string, unknown> | null {
  const part = token.split(".")[1];
  if (!part) return null;
  try {
    const b64 = part.replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const parsed: unknown = JSON.parse(atob(padded));
    return typeof parsed === "object" && parsed !== null ? (parsed as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

/** Reads display claims from an access token. The client never trusts them for authorisation; the API does. */
export function identityFromToken(token: string): TokenIdentity | null {
  const p = decodePayload(token);
  if (!p || typeof p.sub !== "string") return null;
  return {
    userId: p.sub,
    email: typeof p.email === "string" ? p.email : "",
    roles: Array.isArray(p.roles) ? p.roles.filter((r): r is string => typeof r === "string") : [],
    expiresAtMs: typeof p.exp === "number" ? p.exp * 1000 : null,
  };
}
