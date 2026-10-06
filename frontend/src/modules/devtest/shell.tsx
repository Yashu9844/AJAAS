"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { AuthPanel, Badge, Checklist, ExchangeView } from "./components";
import { clearHistory, clearResults, setConfig, setPlatformKey, useDevStore } from "./store";
import { CHECKLIST } from "./specs";

const NAV = [
  { href: "/dev-test", label: "Overview / Checklist" },
  { href: "/dev-test/module-0", label: "Module 0 — Tenant / Identity / RBAC" },
  { href: "/dev-test/module-1", label: "Module 1 — Organization" },
  { href: "/dev-test/module-2", label: "Module 2 — Employee" },
];

export function Shell({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  const st = useDevStore();
  const [showHist, setShowHist] = useState(false);
  return (
    <div className="dt-root">
      <aside className="dt-side">
        <h1>JAAS MODULE VERIFICATION</h1>
        <p className="dt-sub">Temporary testing console — NOT the product UI. Talks to the real backend.</p>
        <nav>
          {NAV.map((n) => (
            <Link key={n.href} href={n.href} className={path === n.href ? "on" : ""}>
              {n.label}
            </Link>
          ))}
        </nav>
        <div className="dt-cfg">
          <label>
            API host
            <input value={st.host} onChange={(e) => setConfig(e.target.value, st.port)} />
          </label>
          <label>
            API port
            <input value={st.port} onChange={(e) => setConfig(st.host, e.target.value)} />
          </label>
          <label>
            Platform key (X-Platform-Key for /tenants)
            <input type="password" value={st.platformKey} onChange={(e) => setPlatformKey(e.target.value)} />
          </label>
          <small>
            Tenant calls use <code>http://&lt;slug&gt;.{st.host}:{st.port}</code>
          </small>
        </div>
        <button className="dt-ghost" onClick={() => setShowHist(!showHist)}>
          {showHist ? "Hide" : "Show"} request log ({st.history.length})
        </button>
        <button className="dt-ghost" onClick={clearResults}>
          Reset all PASS/FAIL results
        </button>
      </aside>
      <main className="dt-main">
        <AuthPanel />
        {showHist && <RequestLog />}
        {children}
      </main>
    </div>
  );
}

function RequestLog() {
  const st = useDevStore();
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div className="dt-card">
      <div className="dt-chead">
        <b>Request log (latest first)</b>
        <button className="dt-mini" onClick={clearHistory}>
          clear
        </button>
      </div>
      <table className="dt-table">
        <tbody>
          {st.history.map((h) => (
            <>
              <tr key={h.id} className="dt-click" onClick={() => setOpen(open === h.id ? null : h.id)}>
                <td>{new Date(h.ts).toLocaleTimeString()}</td>
                <td>{h.method}</td>
                <td className="dt-sub">{h.url.replace(/^https?:\/\//, "")}</td>
                <td>
                  <Badge kind={h.status >= 200 && h.status < 300 ? "pass" : "fail"}>{h.status || "ERR"}</Badge>
                </td>
                <td>{h.ms} ms</td>
              </tr>
              {open === h.id && (
                <tr key={h.id + "x"}>
                  <td colSpan={5}>
                    <ExchangeView ex={h} />
                  </td>
                </tr>
              )}
            </>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Overview() {
  return (
    <div>
      <h2>Verification checklist</h2>
      <p className="dt-note">
        🟢 PASS only when every mapped op/scenario actually produced the expected status and body. 🔴 FAIL if any did not. ⚪ NOT TESTED until run. A 200 alone is never enough.
      </p>
      <div className="dt-grid">
        <Checklist title="MODULE 0 — Tenant / Identity / RBAC" items={CHECKLIST[0]} />
        <Checklist title="MODULE 1 — Organization" items={CHECKLIST[1]} />
        <Checklist title="MODULE 2 — Employee" items={CHECKLIST[2]} />
      </div>
      <h3>How to start</h3>
      <ol className="dt-note">
        <li>
          Start Postgres + Redis (<code>docker compose up -d postgres redis</code>) and the backend (<code>go run cmd/main.go</code> with DB env set).
        </li>
        <li>
          Module 0 → Tenants: create tenant(s) (or use scenario “Tenant lifecycle”). Seed an admin: <code>scripts/dev-test-bootstrap-admin.sh &lt;slug&gt;</code>.
        </li>
        <li>Module 0 → Auth: log in as session A (and session B for a second tenant).</li>
        <li>Run scenarios in each module, or drive single endpoints with the form cards.</li>
      </ol>
    </div>
  );
}

export function Tabs({ tabs }: { tabs: { id: string; label: string; body: React.ReactNode }[] }) {
  const [active, setActive] = useState(tabs[0].id);
  return (
    <div>
      <div className="dt-tabs">
        {tabs.map((t) => (
          <button key={t.id} className={active === t.id ? "on" : ""} onClick={() => setActive(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      {tabs.map((t) => (
        <div key={t.id} hidden={active !== t.id}>
          {t.body}
        </div>
      ))}
    </div>
  );
}
