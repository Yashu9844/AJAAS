"use client";
import { useState } from "react";
import { ep, ExchangeView, Badge } from "./components";
import { runOp, type OpRun } from "./runner";
import { getState, patchSession, setResult, setSession, useDevStore } from "./store";
import type { OpSpec, Session, SessionUser, Slot } from "./types";

const LOGIN: OpSpec = { key: "m0.login", label: "Login", method: "POST", path: "/auth/login", scope: "global", auth: false, fields: [], expect: [200] };
const REFRESH: OpSpec = { key: "m0.refresh", label: "Refresh", method: "POST", path: "/auth/refresh", scope: "global", auth: false, fields: [], expect: [200] };
const LOGOUT: OpSpec = { key: "m0.logout", label: "Logout", method: "POST", path: "/auth/logout", scope: "global", auth: true, fields: [], expect: [200, 204] };

interface LoginData {
  access_token?: string;
  refresh_token?: string;
  expires_at?: string;
  user?: SessionUser;
}
const dataOf = <T,>(r: OpRun): T | undefined => (r.exchange.resBody as { data?: T } | null)?.data;

export function AuthSection() {
  const st = useDevStore();
  const [slot, setSlot] = useState<Slot>("A");
  const [slug, setSlug] = useState("probeco");
  const [email, setEmail] = useState("admin@probeco.com");
  const [password, setPassword] = useState("Secret123!");
  const [last, setLast] = useState<{ title: string; run: OpRun; expect: number[] } | null>(null);
  const [busy, setBusy] = useState(false);
  const sess = st.sessions[slot];

  async function login() {
    setBusy(true);
    const run = await runOp(LOGIN, { method: "POST", scope: "global", path: "/auth/login", body: { tenant_slug: slug, email, password }, opKey: LOGIN.key }, ep(getState()), { record: false });
    // PASS only when the response actually contains both tokens and a user
    const d = dataOf<LoginData>(run);
    if (run.pass && !(d?.access_token && d.refresh_token && d.user?.id)) {
      run.pass = false;
      run.reason = "200 but response lacks access_token / refresh_token / user";
    }
    setResult(LOGIN.key, { state: run.pass ? "pass" : "fail", detail: run.pass ? "HTTP 200 + tokens + user" : run.reason });
    if (run.pass && d) {
      const claims = JSON.parse(atob(d.access_token!.split(".")[1].replace(/-/g, "+").replace(/_/g, "/"))) as { tid?: string };
      const s: Session = { slug, accessToken: d.access_token!, refreshToken: d.refresh_token!, expiresAt: d.expires_at, user: d.user, tenantId: claims.tid };
      setSession(slot, s);
    }
    setLast({ title: `Login → session ${slot}`, run, expect: [200] });
    setBusy(false);
  }

  async function refresh() {
    if (!sess) return;
    setBusy(true);
    const run = await runOp(REFRESH, { method: "POST", scope: "global", path: "/auth/refresh", body: { refresh_token: sess.refreshToken }, opKey: REFRESH.key }, ep(getState()), { record: false });
    const d = dataOf<LoginData>(run);
    if (run.pass && !(d?.access_token && d.refresh_token)) {
      run.pass = false;
      run.reason = "200 but response lacks new tokens";
    }
    setResult(REFRESH.key, { state: run.pass ? "pass" : "fail", detail: run.pass ? "HTTP 200 + rotated tokens" : run.reason });
    if (run.pass && d) patchSession(slot, { accessToken: d.access_token!, refreshToken: d.refresh_token!, expiresAt: d.expires_at, invalid: false });
    setLast({ title: `Refresh token (${slot})`, run, expect: [200] });
    setBusy(false);
  }

  async function logout() {
    if (!sess) return;
    setBusy(true);
    const run = await runOp(LOGOUT, { method: "POST", scope: "global", path: "/auth/logout", token: sess.accessToken, opKey: LOGOUT.key }, ep(getState()));
    if (run.pass) patchSession(slot, { invalid: true });
    setLast({ title: `Logout (${slot})`, run, expect: [200, 204] });
    setBusy(false);
  }

  function clearLocal() {
    setSession(slot, null);
    setLast(null);
  }

  return (
    <section>
      <h3>Authentication</h3>
      <div className="dt-card">
        <div className="dt-chead">
          <div>
            <b>Login</b> <code className="dt-path">POST /api/v1/auth/login</code>
            <span className="dt-tag">public · rate limited 10 / 15 min</span>
          </div>
          <Badge kind="idle">{st.results["m0.login"]?.state.toUpperCase() ?? "NOT TESTED"}</Badge>
        </div>
        <div className="dt-form">
          <label>
            <span>store in session</span>
            <select value={slot} onChange={(e) => setSlot(e.target.value as Slot)}>
              <option value="A">A (primary)</option>
              <option value="B">B (second tenant, for isolation tests)</option>
            </select>
          </label>
          <label>
            <span>tenant_slug *</span>
            <input value={slug} onChange={(e) => setSlug(e.target.value)} />
          </label>
          <label>
            <span>email *</span>
            <input value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label>
            <span>password *</span>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
        </div>
        <div className="dt-actions">
          <button onClick={login} disabled={busy}>
            LOGIN
          </button>
          <button className="dt-ghost" onClick={refresh} disabled={busy || !sess}>
            REFRESH TOKEN
          </button>
          <button className="dt-ghost" onClick={logout} disabled={busy || !sess}>
            LOGOUT
          </button>
          <button className="dt-ghost" onClick={clearLocal} disabled={!sess}>
            forget local session
          </button>
        </div>
        <p className="dt-note">
          The first admin of a tenant cannot be created through the API (POST /users needs auth). Seed one with <code>scripts/dev-test-bootstrap-admin.sh &lt;slug&gt;</code>
          (default admin@&lt;slug&gt;.com / Secret123!). Tenant-scoped calls go to <code>http://&lt;slug&gt;.{st.host}:{st.port}</code>.
        </p>
        {last && <ExchangeView ex={last.run.exchange} pass={last.run.pass} reason={last.run.reason} expect={last.expect} mutation />}
      </div>
    </section>
  );
}
