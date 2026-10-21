"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { AuthPanel, Badge, Checklist, ExchangeView } from "./components";
import { clearHistory, clearResults, setConfig, setPlatformKey, useDevStore } from "./store";
import { HR_CHECKLISTS } from "./hrPages";
import { CHECKLIST } from "./specs";

const NAV = [
  { href: "/dev-test", label: "Overview / Checklist" },
  { href: "/dev-test/identity", label: "M0 — Identity & Access" },
  { href: "/dev-test/hr/core", label: "M1 HR — Core HCM" },
  { href: "/dev-test/hr/organization", label: "M1 HR — Organization" },
  { href: "/dev-test/hr/employee", label: "M1 HR — Employee" },
  { href: "/dev-test/hr/attendance", label: "M1 HR — Attendance & Time" },
  { href: "/dev-test/hr/leave", label: "M1 HR — Leave & Absence" },
  { href: "/dev-test/hr/payroll", label: "M1 HR — Payroll" },
  { href: "/dev-test/hr/recruitment", label: "M1 HR — Recruitment" },
  { href: "/dev-test/hr/performance", label: "M1 HR — Performance & OKRs" },
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
        <Checklist title="M0 — Identity & Access" items={CHECKLIST[0]} />
        <Checklist title="M1 HR — Core HCM" items={HR_CHECKLISTS.core} />
        <Checklist title="M1 HR — Organization" items={CHECKLIST[1]} />
        <Checklist title="M1 HR — Employee" items={CHECKLIST[2]} />
        <Checklist title="M1 HR — Attendance & Time" items={HR_CHECKLISTS.attendance} />
        <Checklist title="M1 HR — Leave & Absence" items={HR_CHECKLISTS.leave} />
        <Checklist title="M1 HR — Payroll" items={HR_CHECKLISTS.payroll} />
        <Checklist title="M1 HR — Recruitment" items={HR_CHECKLISTS.recruitment} />
        <Checklist title="M1 HR — Performance & OKRs" items={HR_CHECKLISTS.performance} />
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
