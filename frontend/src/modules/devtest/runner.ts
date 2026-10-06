import { callApi, decodeJwt, errorOf, getPath, type Endpoint } from "./api";
import { getState, patchSession, pushExchange, setId, setResult } from "./store";
import type { CallSpec, Exchange, Field, OpSpec, Session } from "./types";

// ------------------------------------------------------------------
// Single-operation execution (used by the generic form cards)
// ------------------------------------------------------------------

export interface OpRun {
  exchange: Exchange;
  pass: boolean;
  reason: string; // why it failed (empty when pass)
  captured: Record<string, string>;
}

function coerce(f: Field, raw: string): { ok: true; v: unknown } | { ok: false; err: string } {
  switch (f.type) {
    case "number":
      return { ok: true, v: Number(raw) };
    case "bool":
      return { ok: true, v: raw === "true" };
    case "list":
      return { ok: true, v: raw.split(",").map((s) => s.trim()).filter(Boolean) };
    case "datetime":
      return { ok: true, v: /^\d{4}-\d{2}-\d{2}$/.test(raw) ? `${raw}T00:00:00Z` : raw };
    case "json":
      try {
        return { ok: true, v: JSON.parse(raw) };
      } catch {
        return { ok: false, err: `field ${f.name}: invalid JSON` };
      }
    default:
      return { ok: true, v: raw };
  }
}

export interface BuildOpts {
  slug?: string;
  token?: string | null;
  rawBody?: string;
}

export function buildCall(spec: OpSpec, values: Record<string, string>, o: BuildOpts): { call?: CallSpec; err?: string } {
  let path = spec.path;
  const query: Record<string, string> = {};
  const body: Record<string, unknown> = {};
  for (const f of spec.fields) {
    const raw = (values[f.name] ?? "").trim();
    if (f.where === "path") {
      path = path.replace(`:${f.name}`, encodeURIComponent(raw));
    } else if (f.where === "query") {
      if (raw !== "") query[f.name] = raw;
    } else if (raw !== "") {
      const c = coerce(f, raw);
      if (!c.ok) return { err: c.err };
      body[f.name] = c.v;
    }
  }
  const hasBody = spec.fields.some((f) => f.where === "body") || o.rawBody !== undefined;
  return {
    call: {
      method: spec.method,
      scope: spec.scope,
      slug: o.slug,
      path,
      query,
      body: o.rawBody === undefined && hasBody ? body : undefined,
      rawBody: o.rawBody,
      token: spec.auth ? o.token : null,
      opKey: spec.key,
    },
  };
}

/** Executes an op, evaluates PASS/FAIL honestly, captures ids and records the result. */
export async function runOp(spec: OpSpec, call: CallSpec, ep: Endpoint, opts?: { record?: boolean }): Promise<OpRun> {
  const ex = await callApi(call, ep);
  pushExchange(ex);
  const reasons: string[] = [];
  if (ex.error) reasons.push(`no HTTP response (${ex.error})`);
  else if (!spec.expect.includes(ex.status)) reasons.push(`expected HTTP ${spec.expect.join("/")}, got ${ex.status}`);
  const captured: Record<string, string> = {};
  if (!reasons.length) {
    const body = ex.resBody;
    if (ex.status !== 204 && body && typeof body === "object" && !("data" in (body as object))) {
      reasons.push("2xx response without a `data` envelope");
    }
    for (const c of spec.capture ?? []) {
      const v = getPath(body, c.from);
      if (typeof v === "string" && v) captured[c.idKey] = v;
      else reasons.push(`response has no ${c.from}`);
    }
  }
  const pass = reasons.length === 0;
  if (pass) for (const [k, v] of Object.entries(captured)) setId(k, v);
  if (opts?.record !== false) {
    setResult(spec.key, {
      state: pass ? "pass" : "fail",
      detail: pass ? `HTTP ${ex.status}` : reasons.join("; ") + (errorOf(ex) ? ` — ${errorOf(ex)}` : ""),
    });
  }
  return { exchange: ex, pass, reason: reasons.join("; "), captured };
}

// ------------------------------------------------------------------
// Scenario engine: scripted multi-step flows with explicit expectations
// ------------------------------------------------------------------

export interface StepResult {
  name: string;
  method: string;
  url: string;
  expected: string;
  actual: string;
  state: "pass" | "fail" | "skip";
  note: string;
  ex?: Exchange;
}

export interface ScenarioCtx {
  A: Session; // guaranteed when `needs` is A/AB (runScenario refuses to start otherwise)
  B: Session;
  ep: Endpoint;
  uid: string; // unique suffix for this run
}

export interface Scenario {
  key: string;
  module: 0 | 1 | 2;
  title: string;
  description: string;
  needs: "none" | "A" | "AB";
  run: (r: Runner, c: ScenarioCtx) => Promise<void>;
}

class Abort extends Error {}

export class Runner {
  steps: StepResult[] = [];
  vars: Record<string, string> = {};
  constructor(
    private ep: Endpoint,
    private onUpdate: (steps: StepResult[]) => void,
  ) {}

  /** Execute a request and assert status (+ optional body check). Returns the exchange. */
  async call(name: string, spec: CallSpec, expect: number | number[], check?: (ex: Exchange) => string | null): Promise<Exchange> {
    const exp = Array.isArray(expect) ? expect : [expect];
    const ex = await callApi(spec, this.ep);
    pushExchange(ex);
    let note = "";
    let ok = true;
    if (ex.error) {
      ok = false;
      note = `no HTTP response: ${ex.error}`;
    } else if (!exp.includes(ex.status)) {
      ok = false;
      note = errorOf(ex);
    } else if (check) {
      const m = check(ex);
      if (m) {
        ok = false;
        note = m;
      }
    }
    if (ok && !note) note = errorOf(ex);
    this.steps.push({
      name,
      method: spec.method,
      url: ex.url,
      expected: exp.join(" / "),
      actual: ex.error ? "network error" : String(ex.status),
      state: ok ? "pass" : "fail",
      note,
      ex,
    });
    this.onUpdate([...this.steps]);
    return ex;
  }

  skip(name: string, reason: string) {
    this.steps.push({ name, method: "-", url: "-", expected: "-", actual: "-", state: "skip", note: reason });
    this.onUpdate([...this.steps]);
  }

  /** Stop the scenario (a later step cannot run without this value). */
  need(cond: unknown, why: string): void {
    if (!cond) {
      this.steps.push({ name: "ABORTED", method: "-", url: "-", expected: "-", actual: "-", state: "fail", note: why });
      this.onUpdate([...this.steps]);
      throw new Abort(why);
    }
  }
}

export function str(ex: Exchange, path: string): string {
  const v = getPath(ex.resBody, path);
  return typeof v === "string" ? v : "";
}

/** body check helper: every [path, expected] pair must match */
export function expectFields(...pairs: [string, unknown][]): (ex: Exchange) => string | null {
  return (ex) => {
    for (const [p, want] of pairs) {
      const got = getPath(ex.resBody, p);
      if (JSON.stringify(got) !== JSON.stringify(want)) return `${p}: expected ${JSON.stringify(want)}, got ${JSON.stringify(got)}`;
    }
    return null;
  };
}

/** Access tokens live 15 min; transparently rotate an (almost) expired session before a scenario starts. */
async function refreshIfExpiring(slot: "A" | "B", ep: Endpoint) {
  const s = getState().sessions[slot];
  if (!s) return;
  const exp = decodeJwt(s.accessToken)?.exp ?? 0;
  if (exp - Date.now() / 1000 > 60) return;
  const ex = await callApi({ method: "POST", scope: "global", path: "/auth/refresh", body: { refresh_token: s.refreshToken } }, ep);
  pushExchange(ex);
  const d = (ex.resBody as { data?: { access_token?: string; refresh_token?: string; expires_at?: string } } | null)?.data;
  if (ex.status === 200 && d?.access_token && d.refresh_token) patchSession(slot, { accessToken: d.access_token, refreshToken: d.refresh_token, expiresAt: d.expires_at, invalid: false });
}

export async function runScenario(sc: Scenario, ep: Endpoint, onUpdate: (s: StepResult[]) => void): Promise<StepResult[]> {
  if (sc.needs !== "none") await refreshIfExpiring("A", ep);
  if (sc.needs === "AB") await refreshIfExpiring("B", ep);
  const st = getState();
  if ((sc.needs !== "none" && !st.sessions.A) || (sc.needs === "AB" && !st.sessions.B)) return [];
  const r = new Runner(ep, onUpdate);
  const ctx: ScenarioCtx = {
    A: st.sessions.A as Session,
    B: st.sessions.B as Session,
    ep,
    uid: Date.now().toString(36).slice(-6) + Math.floor(Math.random() * 36).toString(36),
  };
  try {
    await sc.run(r, ctx);
  } catch (e) {
    if (!(e instanceof Abort)) {
      r.steps.push({
        name: "UNEXPECTED ERROR",
        method: "-",
        url: "-",
        expected: "-",
        actual: "-",
        state: "fail",
        note: e instanceof Error ? e.message : String(e),
      });
      onUpdate([...r.steps]);
    }
  }
  const fails = r.steps.filter((s) => s.state === "fail").length;
  const skips = r.steps.filter((s) => s.state === "skip").length;
  const passes = r.steps.filter((s) => s.state === "pass").length;
  setResult(sc.key, {
    state: fails ? "fail" : skips || !passes ? "partial" : "pass",
    detail: `${passes} pass / ${fails} fail / ${skips} skipped`,
  });
  return r.steps;
}

export const dataOf = (ex: Exchange, path = "data") => getPath(ex.resBody, path);
