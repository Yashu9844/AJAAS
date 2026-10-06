"use client";
import { useState } from "react";
import { decodeJwt, maskToken, type Endpoint } from "./api";
import { buildCall, runOp, runScenario, type OpRun, type Scenario, type StepResult } from "./runner";
import { getState, patchSession, useDevStore } from "./store";
import { callApi } from "./api";
import { OP_BY_KEY } from "./specs";
import type { ChecklistItem, Exchange, OpSpec, Session, Slot, StoredResult } from "./types";

export const ep = (s: { host: string; port: string }): Endpoint => ({ host: s.host, port: s.port });

// ------------------------------------------------------------------ primitives
export function Badge({ kind, children }: { kind: "pass" | "fail" | "idle" | "warn"; children: React.ReactNode }) {
  return <span className={`dt-badge dt-${kind}`}>{children}</span>;
}

export function Json({ value, max = 420 }: { value: unknown; max?: number }) {
  const text = typeof value === "string" ? value : JSON.stringify(value, null, 2);
  return (
    <pre className="dt-json" style={{ maxHeight: max }}>
      {text === undefined ? "(empty)" : text}
    </pre>
  );
}

const FLOW = ["UI", "API", "Controller", "Service", "Repository", "Database", "Response", "UI"];
export function Flow({ ex, mutation, pass }: { ex?: Exchange; mutation: boolean; pass?: boolean }) {
  const reached = ex ? (ex.status === 0 ? 1 : 7) : 0;
  return (
    <div className="dt-flow" title="Expected architecture; only the API response is actual evidence">
      {FLOW.map((n, i) => (
        <span key={i} className={i < reached ? "on" : ""}>
          {n}
          {i < FLOW.length - 1 ? " → " : ""}
        </span>
      ))}
      <em>
        {!ex
          ? " (not run)"
          : ex.status === 0
            ? " — no HTTP response, request never reached the API"
            : pass && mutation
              ? " — API confirmed the write (2xx); DB persistence is only proven by a read-back"
              : pass
                ? " — API returned the expected response"
                : " — API responded, but not as expected"}
      </em>
    </div>
  );
}

export function ExchangeView({ ex, pass, reason, mutation, expect }: { ex: Exchange; pass?: boolean; reason?: string; mutation?: boolean; expect?: number[] }) {
  const err = (() => {
    const b = ex.resBody as { error?: unknown } | null;
    if (!b || typeof b !== "object" || !("error" in b)) return ex.error ?? "";
    const e = b.error;
    if (typeof e === "string") return e;
    const o = e as { code?: string; message?: string };
    return `${o.code ?? ""}: ${o.message ?? ""}`;
  })();
  return (
    <div className="dt-result">
      <div className="dt-rhead">
        <b>API RESULT</b>
        {pass !== undefined && <Badge kind={pass ? "pass" : "fail"}>{pass ? "PASS" : "FAIL"}</Badge>}
      </div>
      <div className="dt-kv">
        <span>Method</span>
        <code>{ex.method}</code>
        <span>Endpoint</span>
        <code>{ex.url}</code>
        <span>Status</span>
        <code>{ex.status === 0 ? "network error" : `${ex.status} ${ex.statusText}`}</code>
        {expect && (
          <>
            <span>Expected</span>
            <code>HTTP {expect.join(" / ")}</code>
          </>
        )}
        <span>Response time</span>
        <code>{ex.ms} ms</code>
        {(err || reason) && (
          <>
            <span>Error</span>
            <code className="dt-errtext">{[err, reason && !err ? reason : reason && reason !== err ? `(${reason})` : ""].filter(Boolean).join(" ")}</code>
          </>
        )}
      </div>
      <Flow ex={ex} mutation={!!mutation} pass={pass} />
      <details>
        <summary>Request inspector (headers + body)</summary>
        <div className="dt-cols">
          <div>
            <h5>Request headers</h5>
            <Json value={ex.reqHeaders} max={160} />
          </div>
          <div>
            <h5>Request body</h5>
            <Json value={ex.reqBody ?? "(none)"} max={160} />
          </div>
        </div>
      </details>
      <h5>Response body</h5>
      <Json value={ex.resBody ?? ex.error ?? "(empty body)"} />
    </div>
  );
}

// ------------------------------------------------------------------ operation card
export function OpCard({ spec }: { spec: OpSpec }) {
  const st = useDevStore();
  const [values, setValues] = useState<Record<string, string>>({});
  const [slot, setSlot] = useState<Slot>("A");
  const [slugOverride, setSlugOverride] = useState("");
  const [raw, setRaw] = useState<string | null>(null);
  const [run, setRun] = useState<OpRun | null>(null);
  const [back, setBack] = useState<OpRun | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const session = st.sessions[slot];
  const defOf = (name: string) => {
    const f = spec.fields.find((x) => x.name === name)!;
    return (f.ref ? st.ids[f.ref] : undefined) ?? f.def ?? "";
  };
  const val = (name: string) => values[name] ?? defOf(name);
  const mutation = spec.method !== "GET";
  const stored = st.results[spec.key];

  async function exec() {
    setBusy(true);
    setMsg("");
    setBack(null);
    const vals = Object.fromEntries(spec.fields.map((f) => [f.name, val(f.name)]));
    const b = buildCall(spec, vals, {
      slug: slugOverride.trim() || session?.slug || "",
      token: session?.accessToken ?? null,
      rawBody: raw ?? undefined,
    });
    if (!b.call) {
      setMsg(b.err ?? "invalid input");
      setBusy(false);
      return;
    }
    setRun(await runOp(spec, b.call, ep(st)));
    setBusy(false);
  }

  async function fetchCreated() {
    const fs = OP_BY_KEY[spec.fetchAfter ?? ""];
    if (!fs) return;
    setBusy(true);
    const ids = getState().ids;
    const vals = Object.fromEntries(fs.fields.map((f) => [f.name, (f.ref ? ids[f.ref] : "") ?? ""]));
    const b = buildCall(fs, vals, { slug: slugOverride.trim() || session?.slug || "", token: session?.accessToken ?? null });
    if (b.call) setBack(await runOp(fs, b.call, ep(st)));
    setBusy(false);
  }

  const createdId = run?.pass ? Object.values(run.captured)[0] : undefined;
  const backId = back ? (back.exchange.resBody as { data?: { id?: string } } | null)?.data?.id : undefined;

  return (
    <div className="dt-card">
      <div className="dt-chead">
        <div>
          <b>{spec.label}</b> <code className="dt-path">{spec.method} /api/v1{spec.path}</code>
          <span className="dt-tag">{spec.scope === "tenant" ? "tenant host" : "global"}{spec.auth ? " · JWT" : " · public"}</span>
        </div>
        {stored ? <Badge kind={stored.state === "pass" ? "pass" : stored.state === "fail" ? "fail" : "idle"}>{stored.state.toUpperCase()}</Badge> : <Badge kind="idle">NOT TESTED</Badge>}
      </div>
      {spec.note && <p className="dt-note">{spec.note}</p>}
      {raw === null ? (
        <div className="dt-form">
          {spec.fields.map((f) => (
            <label key={f.name} className={f.where === "path" ? "dt-path-f" : ""}>
              <span>
                {f.label ?? f.name}
                <small>
                  {" "}
                  {f.where}
                  {f.required ? " *" : ""}
                </small>
              </span>
              {f.type === "select" ? (
                <select value={val(f.name)} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}>
                  {(f.options ?? []).map((o) => (
                    <option key={o} value={o}>
                      {o || "(unset)"}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type={f.type === "password" ? "password" : "text"}
                  value={val(f.name)}
                  placeholder={f.placeholder ?? ""}
                  onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}
                />
              )}
            </label>
          ))}
        </div>
      ) : (
        <label className="dt-rawlabel">
          Raw request body (sent verbatim — use for invalid / malformed input tests)
          <textarea value={raw} onChange={(e) => setRaw(e.target.value)} rows={5} />
        </label>
      )}
      <div className="dt-actions">
        <button onClick={exec} disabled={busy}>
          {busy ? "Running…" : `Send ${spec.method}`}
        </button>
        {spec.fields.some((f) => f.where === "body") && (
          <button className="dt-ghost" onClick={() => setRaw(raw === null ? "{}" : null)}>
            {raw === null ? "Edit raw body" : "Back to form"}
          </button>
        )}
        {spec.fetchAfter && createdId && (
          <button className="dt-ghost" onClick={fetchCreated} disabled={busy}>
            Fetch created record
          </button>
        )}
        <details className="dt-adv">
          <summary>advanced</summary>
          <label>
            token slot
            <select value={slot} onChange={(e) => setSlot(e.target.value as Slot)}>
              <option value="A">Session A{st.sessions.A ? ` (${st.sessions.A.slug})` : " (none)"}</option>
              <option value="B">Session B{st.sessions.B ? ` (${st.sessions.B.slug})` : " (none)"}</option>
            </select>
          </label>
          {spec.scope === "tenant" && (
            <label>
              host tenant slug override
              <input value={slugOverride} placeholder={session?.slug ?? "slug"} onChange={(e) => setSlugOverride(e.target.value)} />
            </label>
          )}
        </details>
        {msg && <span className="dt-errtext">{msg}</span>}
      </div>
      {spec.auth && !session && <p className="dt-warn">No session in slot {slot}: the request will be sent WITHOUT a token (expect 401).</p>}
      {run && (
        <>
          <ExchangeView ex={run.exchange} pass={run.pass} reason={run.reason} mutation={mutation} expect={spec.expect} />
          {run.pass && Object.keys(run.captured).length > 0 && (
            <div className="dt-ids">
              Database-generated IDs captured:{" "}
              {Object.entries(run.captured).map(([k, v]) => (
                <code key={k}>
                  {k}={v}
                </code>
              ))}
            </div>
          )}
        </>
      )}
      {back && (
        <div className="dt-readback">
          <b>READ-BACK (CREATE → DATABASE → READ)</b>{" "}
          {back.pass && backId === createdId ? <Badge kind="pass">same id returned by GET</Badge> : <Badge kind="fail">GET did not return the created record</Badge>}
          <ExchangeView ex={back.exchange} pass={back.pass && backId === createdId} reason={back.reason} expect={[200]} />
        </div>
      )}
    </div>
  );
}

export function OpGroup({ title, ops }: { title: string; ops: OpSpec[] }) {
  return (
    <section>
      <h3>{title}</h3>
      {ops.map((o) => (
        <OpCard key={o.key} spec={o} />
      ))}
    </section>
  );
}

// ------------------------------------------------------------------ scenarios
export function ScenarioCard({ sc }: { sc: Scenario }) {
  const st = useDevStore();
  const [steps, setSteps] = useState<StepResult[]>([]);
  const [busy, setBusy] = useState(false);
  const stored = st.results[sc.key];
  const missing = sc.needs !== "none" && !st.sessions.A ? "Log in as Session A first (Module 0 → Auth)." : sc.needs === "AB" && !st.sessions.B ? "Log in as Session B (a second tenant) first (Module 0 → Auth)." : "";

  async function go() {
    setBusy(true);
    setSteps([]);
    await runScenario(sc, ep(getState()), setSteps);
    setBusy(false);
  }
  return (
    <div className="dt-card">
      <div className="dt-chead">
        <div>
          <b>{sc.title}</b>
          <span className="dt-tag">scenario · {sc.needs === "none" ? "no login" : sc.needs === "A" ? "needs session A" : "needs sessions A + B"}</span>
        </div>
        {stored ? (
          <Badge kind={stored.state === "pass" ? "pass" : stored.state === "fail" ? "fail" : "warn"}>
            {stored.state === "partial" ? "PARTIAL" : stored.state.toUpperCase()} · {stored.detail}
          </Badge>
        ) : (
          <Badge kind="idle">NOT TESTED</Badge>
        )}
      </div>
      <p className="dt-note">{sc.description}</p>
      <div className="dt-actions">
        <button onClick={go} disabled={busy || !!missing}>
          {busy ? "Running…" : "Run scenario"}
        </button>
        {missing && <span className="dt-warn">{missing}</span>}
      </div>
      {steps.length > 0 && (
        <table className="dt-table">
          <thead>
            <tr>
              <th>#</th>
              <th>Step</th>
              <th>Expected</th>
              <th>Actual</th>
              <th>Result</th>
              <th>Note</th>
            </tr>
          </thead>
          <tbody>
            {steps.map((s, i) => (
              <StepRow key={i} s={s} i={i} />
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function StepRow({ s, i }: { s: StepResult; i: number }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <tr onClick={() => s.ex && setOpen(!open)} className={s.ex ? "dt-click" : ""}>
        <td>{i + 1}</td>
        <td>
          {s.name}
          <div className="dt-sub">
            {s.method} {s.url.replace(/^https?:\/\//, "")}
          </div>
        </td>
        <td>{s.expected}</td>
        <td>{s.actual}</td>
        <td>
          <Badge kind={s.state === "pass" ? "pass" : s.state === "fail" ? "fail" : "warn"}>{s.state === "skip" ? "SKIPPED" : s.state.toUpperCase()}</Badge>
        </td>
        <td>{s.note}</td>
      </tr>
      {open && s.ex && (
        <tr>
          <td colSpan={6}>
            <ExchangeView ex={s.ex} pass={s.state === "pass"} />
          </td>
        </tr>
      )}
    </>
  );
}

// ------------------------------------------------------------------ checklist
export function resultFor(item: ChecklistItem, results: Record<string, StoredResult>): { label: string; kind: "pass" | "fail" | "idle" | "warn" } {
  if (item.unavailable) return { label: "⚪ NOT TESTABLE (no API)", kind: "idle" };
  const rs = item.keys.map((k) => results[k]);
  if (rs.some((r) => r?.state === "fail")) return { label: "🔴 FAIL", kind: "fail" };
  if (rs.every((r) => r?.state === "pass")) return { label: "🟢 PASS", kind: "pass" };
  if (rs.some((r) => r?.state === "partial")) return { label: "⚪ PARTIAL (skipped steps)", kind: "warn" };
  return { label: "⚪ NOT TESTED", kind: "idle" };
}

export function Checklist({ title, items }: { title: string; items: ChecklistItem[] }) {
  const st = useDevStore();
  return (
    <div className="dt-card">
      <div className="dt-chead">
        <b>{title}</b>
      </div>
      <table className="dt-table">
        <tbody>
          {items.map((it) => {
            const r = resultFor(it, st.results);
            return (
              <tr key={it.label}>
                <td>{it.label}</td>
                <td>
                  <Badge kind={r.kind}>{r.label}</Badge>
                </td>
                <td className="dt-sub">{it.unavailable ?? it.keys.map((k) => `${k}: ${st.results[k]?.state ?? "-"}`).join(" · ")}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// ------------------------------------------------------------------ auth state panel
function tokenInfo(s: Session): string {
  if (s.invalid) return "rejected / revoked";
  const c = decodeJwt(s.accessToken);
  if (!c?.exp) return "unknown";
  const left = Math.round(c.exp - Date.now() / 1000);
  return left > 0 ? `valid · expires in ${left}s` : `expired ${-left}s ago`;
}

export function SessionBox({ slot, s }: { slot: Slot; s: Session | null }) {
  const st = useDevStore();
  const [busy, setBusy] = useState(false);
  async function loadPerms() {
    if (!s?.user) return;
    setBusy(true);
    const perms = new Set<string>();
    for (const r of s.user.roles) {
      const ex = await callApi({ method: "GET", scope: "tenant", slug: s.slug, path: `/roles/${r.id}`, token: s.accessToken }, ep(st));
      const list = (ex.resBody as { data?: { permissions?: { resource: string; action: string }[] } } | null)?.data?.permissions ?? [];
      list.forEach((p) => perms.add(`${p.resource}:${p.action}`));
      if (r.name === "tenant_admin") perms.add("* (tenant_admin bypasses permission checks)");
      if (ex.status === 401) patchSession(slot, { invalid: true });
    }
    patchSession(slot, { permissions: [...perms] });
    setBusy(false);
  }
  if (!s) {
    return (
      <div className="dt-sess">
        <b>Session {slot}</b> — not logged in
      </div>
    );
  }
  return (
    <div className="dt-sess">
      <b>Session {slot}</b>
      <div className="dt-kv">
        <span>Tenant</span>
        <code>
          {s.slug} {s.tenantId ? `(${s.tenantId.slice(0, 8)}…)` : ""}
        </code>
        <span>User</span>
        <code>{s.user ? `${s.user.first_name} ${s.user.last_name} <${s.user.email}>` : "?"}</code>
        <span>User ID</span>
        <code>{s.user?.id ?? "?"}</code>
        <span>Roles</span>
        <code>{s.user?.roles.map((r) => r.name).join(", ") || "-"}</code>
        <span>Permissions</span>
        <code>
          {s.permissions ? s.permissions.join(", ") || "(none)" : "not loaded"}{" "}
          <button className="dt-mini" onClick={loadPerms} disabled={busy}>
            load
          </button>
        </code>
        <span>Access token</span>
        <code>
          {maskToken(s.accessToken)} · {tokenInfo(s)}
        </code>
        <span>Refresh token</span>
        <code>{s.refreshToken ? "••••••••" : "none"}</code>
        <span>Session</span>
        <code>{s.invalid ? "inactive (API rejected token)" : tokenInfo(s).startsWith("expired") ? "access token expired (refresh needed)" : "active (token valid; server-side check on next call)"}</code>
      </div>
    </div>
  );
}

export function AuthPanel() {
  const st = useDevStore();
  return (
    <div className="dt-auth">
      <SessionBox slot="A" s={st.sessions.A} />
      <SessionBox slot="B" s={st.sessions.B} />
    </div>
  );
}
