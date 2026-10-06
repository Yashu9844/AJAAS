// Types for the temporary Module 0-2 verification console (NOT product UI).

export type Method = "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
export type Scope = "global" | "tenant";
export type Slot = "A" | "B";

export interface Exchange {
  id: string;
  ts: number;
  opKey?: string;
  method: Method;
  url: string;
  reqHeaders: Record<string, string>;
  reqBody?: unknown;
  status: number; // 0 = network error
  statusText: string;
  ms: number;
  resBody: unknown;
  error?: string;
}

export interface SessionUser {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  status: string;
  roles: { id: string; name: string }[];
}

export interface Session {
  slug: string;
  accessToken: string;
  refreshToken: string;
  expiresAt?: string;
  user?: SessionUser;
  tenantId?: string;
  permissions?: string[];
  invalid?: boolean; // set when an API call returned 401 with this token
}

export type ResultState = "pass" | "fail" | "partial";
export interface StoredResult {
  state: ResultState;
  at: number;
  detail: string;
}

export interface CallSpec {
  method: Method;
  scope: Scope;
  slug?: string; // tenant subdomain (scope=tenant)
  path: string; // e.g. /users/123
  query?: Record<string, string>;
  body?: unknown;
  rawBody?: string; // sent verbatim (for invalid-JSON tests)
  token?: string | null;
  opKey?: string;
  headers?: Record<string, string>; // extra request headers (negative tests)
  noPlatformKey?: boolean; // do NOT attach X-Platform-Key (negative tests)
}

// ---- declarative operation spec (drives the generic form cards) ----
export type FieldType = "text" | "number" | "bool" | "select" | "json" | "list" | "datetime" | "password";
export interface Field {
  name: string;
  label?: string;
  where: "path" | "query" | "body";
  type?: FieldType;
  options?: string[];
  required?: boolean;
  ref?: string; // id key in the captured-ids store used as default
  placeholder?: string;
  def?: string;
}
export interface OpSpec {
  key: string;
  label: string;
  method: Method;
  path: string; // may contain :id placeholders matching path-field names
  scope: Scope;
  auth: boolean;
  fields: Field[];
  expect: number[];
  capture?: { idKey: string; from: string }[]; // from = dot path inside response JSON
  fetchAfter?: string; // key of the GET op used for "fetch created record"
  note?: string;
}
export interface ChecklistItem {
  label: string;
  keys: string[]; // op keys / scenario keys that must all PASS
  unavailable?: string; // reason it cannot be tested through the API
}
