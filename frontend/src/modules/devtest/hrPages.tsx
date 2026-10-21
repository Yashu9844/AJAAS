"use client";
import { Checklist, OpGroup, ScenarioCard } from "./components";
import { HR_ATTENDANCE, HR_COMPENSATION, HR_CORE, HR_LEAVE, HR_PAYROLL, HR_PERFORMANCE, HR_RECRUITMENT } from "./generated/hrSpecs";
import type { Scenario } from "./runner";
import { S_ATTENDANCE, S_COMPENSATION, S_CORE, S_LEAVE, S_PAYROLL, S_PERFORMANCE, S_RECRUITMENT } from "./scenariosHr";
import { Tabs } from "./shell";
import type { ChecklistItem, OpSpec } from "./types";

// HR context pages of the test console (docs/TEST_CONSOLE.md). Operation forms are generated from the swagger contract
// (scripts/gen_console_specs.py); scenarios are the context goldens in scenariosHr.ts.

/** Groups operations by resource: the path segment after /hr/<context>/ (or the context itself). */
function byResource(ops: OpSpec[]): [string, OpSpec[]][] {
  const groups = new Map<string, OpSpec[]>();
  for (const op of ops) {
    const segs = op.path.split("/").filter((s) => s && s !== "hr" && !s.startsWith(":"));
    const key = segs.length > 1 ? `${segs[0]} / ${segs[1]}` : segs[0];
    groups.set(key, [...(groups.get(key) ?? []), op]);
  }
  return [...groups.entries()];
}

interface ContextPageProps {
  title: string;
  intro: string;
  ops: OpSpec[];
  scenarios: Scenario[];
  checklist: ChecklistItem[];
}

function ContextPage({ title, intro, ops, scenarios, checklist }: ContextPageProps) {
  return (
    <div>
      <h2>{title}</h2>
      <p className="dt-note">{intro}</p>
      <Tabs
        tabs={[
          {
            id: "ops",
            label: `Endpoints (${ops.length})`,
            body: (
              <>
                {byResource(ops).map(([name, group]) => (
                  <OpGroup key={name} title={name} ops={group} />
                ))}
              </>
            ),
          },
          {
            id: "sc",
            label: "Scenarios (automated)",
            body: (
              <div>
                <p className="dt-note">Each scenario creates its own users, employees and records in session A&apos;s tenant (session A must be a tenant_admin).</p>
                {scenarios.map((s) => (
                  <ScenarioCard key={s.key} sc={s} />
                ))}
              </div>
            ),
          },
          { id: "ck", label: "Checklist", body: <Checklist title={title.toUpperCase()} items={checklist} /> },
        ]}
      />
    </div>
  );
}

const items = (list: Scenario[]): ChecklistItem[] => list.map((s) => ({ label: s.title, keys: [s.key] }));

export const HR_CHECKLISTS: Record<string, ChecklistItem[]> = {
  attendance: items(S_ATTENDANCE),
  leave: items(S_LEAVE),
  payroll: items(S_PAYROLL),
  recruitment: items(S_RECRUITMENT),
  performance: items(S_PERFORMANCE),
  core: items(S_CORE),
  compensation: items(S_COMPENSATION),
};

export function HrPerformance() {
  return (
    <ContextPage
      title="HR / Performance & OKRs"
      intro="Appraisal cycles, goals with key results and check-ins, self/manager/peer reviews and the 9-box. Owners and their direct managers act on goals without a permission; performance:read | manage cover everyone."
      ops={HR_PERFORMANCE}
      scenarios={S_PERFORMANCE}
      checklist={HR_CHECKLISTS.performance}
    />
  );
}

export function HrAttendance() {
  return (
    <ContextPage
      title="HR / Attendance & Time"
      intro="Shifts, assignments, punches (server clock), daily records, regularizations and summaries. Self endpoints need only a login with an employee profile; others need attendance:read | manage | approve."
      ops={HR_ATTENDANCE}
      scenarios={S_ATTENDANCE}
      checklist={HR_CHECKLISTS.attendance}
    />
  );
}

export function HrLeave() {
  return (
    <ContextPage
      title="HR / Leave & Absence"
      intro="Leave types, holidays, ledger-backed balances, requests and approvals; approved leave marks attendance on_leave and unpaid leave feeds payroll loss of pay."
      ops={HR_LEAVE}
      scenarios={S_LEAVE}
      checklist={HR_CHECKLISTS.leave}
    />
  );
}

export function HrPayroll() {
  return (
    <ContextPage
      title="HR / Payroll & Statutory"
      intro="Salary structures, CTC assignments, maker-checker payroll runs with Indian statutory deductions, payslips and the payout CSV. Money is exact (2 decimals)."
      ops={HR_PAYROLL}
      scenarios={S_PAYROLL}
      checklist={HR_CHECKLISTS.payroll}
    />
  );
}

export function HrRecruitment() {
  return (
    <ContextPage
      title="HR / Recruitment (ATS)"
      intro="Jobs, candidates and their stage history, interviews and feedback, offers and the hire saga (invites the user, creates the employee profile)."
      ops={HR_RECRUITMENT}
      scenarios={S_RECRUITMENT}
      checklist={HR_CHECKLISTS.recruitment}
    />
  );
}

export function HrCore() {
  return (
    <ContextPage
      title="HR / Core HCM"
      intro="Job architecture, positions with effective-dated occupancy, HR actions (hire … rehire) with maker-checker approval, worker history as of any date, headcount and back-fill. core:read | manage | approve; /workers/me for everyone."
      ops={HR_CORE}
      scenarios={S_CORE}
      checklist={HR_CHECKLISTS.core}
    />
  );
}

export function HrCompensation() {
  return (
    <ContextPage
      title="HR / Compensation"
      intro="Pay ranges per grade, effective-dated base pay synced to payroll, off-cycle changes with maker-checker (raises above 10% need compensation:approve, never your own) and merit cycles with guidelines, budgets and manager proposals. /me for everyone."
      ops={HR_COMPENSATION}
      scenarios={S_COMPENSATION}
      checklist={HR_CHECKLISTS.compensation}
    />
  );
}
