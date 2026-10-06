"use client";
import { AuthSection } from "./auth";
import { Checklist, OpGroup, ScenarioCard, SessionBox } from "./components";
import { S0 } from "./scenarios0";
import { S1 } from "./scenarios1";
import { S2 } from "./scenarios2";
import { Tabs } from "./shell";
import { useDevStore } from "./store";
import {
  CHECKLIST,
  M0_AUTH,
  M0_ROLES,
  M0_TENANTS,
  M0_USERS,
  M1_CHART,
  M1_DEPT,
  M1_DESIG,
  M1_MAP,
  M1_TEAM,
  M2_DOC,
  M2_EMP,
  M2_STAT,
} from "./specs";

function Scenarios({ list }: { list: typeof S0 }) {
  return (
    <div>
      <p className="dt-note">
        Scenarios are scripted flows that assert the expected status AND body at every step (CREATE → READ → UPDATE → READ AGAIN → DEACTIVATE → READ AGAIN, plus negatives). They create their own throwaway data.
        Click a step row to see its full request/response.
      </p>
      {list.map((s) => (
        <ScenarioCard key={s.key} sc={s} />
      ))}
    </div>
  );
}

function RbacViewer() {
  const st = useDevStore();
  return (
    <div>
      <h3>Current user → roles → permissions</h3>
      <p className="dt-note">
        Permissions are resolved by calling GET /roles/:id for each role of the logged-in user (the backend has no “my permissions” endpoint). tenant_admin bypasses permission checks in RequirePermission.
      </p>
      <div className="dt-auth">
        <SessionBox slot="A" s={st.sessions.A} />
        <SessionBox slot="B" s={st.sessions.B} />
      </div>
    </div>
  );
}

export function Module0() {
  return (
    <div>
      <h2>Module 0 — Tenant / Identity / RBAC</h2>
      <Tabs
        tabs={[
          {
            id: "auth",
            label: "Authentication",
            body: (
              <>
                <AuthSection />
                <OpGroup title="Password recovery" ops={M0_AUTH} />
              </>
            ),
          },
          { id: "tenants", label: "Tenants", body: <OpGroup title="Tenants (global routes, no host subdomain)" ops={M0_TENANTS} /> },
          { id: "users", label: "Users", body: <OpGroup title="Users (tenant host + JWT + RBAC users:*)" ops={M0_USERS} /> },
          {
            id: "roles",
            label: "Roles / Permissions / RBAC",
            body: (
              <>
                <RbacViewer />
                <OpGroup title="Roles & permissions" ops={M0_ROLES} />
              </>
            ),
          },
          { id: "sc", label: "Scenarios (automated)", body: <Scenarios list={S0} /> },
          { id: "ck", label: "Checklist", body: <Checklist title="MODULE 0" items={CHECKLIST[0]} /> },
        ]}
      />
    </div>
  );
}

export function Module1() {
  return (
    <div>
      <h2>Module 1 — Organization</h2>
      <p className="dt-note">All routes are tenant-host + JWT + RBAC (organization:read for GET, organization:update for writes).</p>
      <Tabs
        tabs={[
          { id: "dept", label: "Departments", body: <OpGroup title="Departments" ops={M1_DEPT} /> },
          { id: "team", label: "Teams", body: <OpGroup title="Teams" ops={M1_TEAM} /> },
          { id: "desig", label: "Designations", body: <OpGroup title="Designations" ops={M1_DESIG} /> },
          { id: "map", label: "Mappings", body: <OpGroup title="User ↔ org mappings" ops={M1_MAP} /> },
          { id: "chart", label: "Org chart", body: <OpGroup title="Org chart" ops={M1_CHART} /> },
          { id: "sc", label: "Scenarios (automated)", body: <Scenarios list={S1} /> },
          { id: "ck", label: "Checklist", body: <Checklist title="MODULE 1" items={CHECKLIST[1]} /> },
        ]}
      />
    </div>
  );
}

export function Module2() {
  return (
    <div>
      <h2>Module 2 — Employee</h2>
      <p className="dt-note">An employee profile is attached to an existing Module 0 user (user_id). Create a user first (Module 0 → Users) — its id is captured automatically.</p>
      <Tabs
        tabs={[
          { id: "emp", label: "Employees", body: <OpGroup title="Employee profiles" ops={M2_EMP} /> },
          { id: "stat", label: "Statutory & bank", body: <OpGroup title="Statutory / bank" ops={M2_STAT} /> },
          { id: "doc", label: "Documents & timeline", body: <OpGroup title="Documents & timeline" ops={M2_DOC} /> },
          { id: "sc", label: "Scenarios (automated)", body: <Scenarios list={S2} /> },
          { id: "ck", label: "Checklist", body: <Checklist title="MODULE 2" items={CHECKLIST[2]} /> },
        ]}
      />
    </div>
  );
}
