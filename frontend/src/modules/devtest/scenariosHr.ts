import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// HR context scenarios (docs/TEST_CONSOLE.md): the context goldens, scripted against the live backend. Each run creates
// its own throwaway users, employees and records in session A's tenant (A must be a tenant_admin).

const ymd = (d: Date) => d.toISOString().slice(0, 10);
const addDays = (d: Date, n: number) => new Date(d.getTime() + n * 86400000);
/** The Monday k weeks after the next Monday (UTC) — always a future working day. */
function monday(k: number): Date {
  const t = new Date();
  const today = new Date(Date.UTC(t.getUTCFullYear(), t.getUTCMonth(), t.getUTCDate()));
  const ahead = ((8 - today.getUTCDay()) % 7) || 7;
  return addDays(today, ahead + 7 * k);
}
const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];
const num = (ex: Exchange, path: string) => Number(getPath(ex.resBody, path));

function empBody(userId: string, code: string) {
  return { user_id: userId, employee_code: code, first_name: "Dt", last_name: code, employment_type: "full_time", joining_date: "2026-01-15T00:00:00Z" };
}

/** A fresh user with an employee profile and a logged-in session. */
async function member(r: Runner, A: Session, uid: string, tag: string, roleIds: string[] = []): Promise<{ S: Session; userId: string; empId: string }> {
  const u = await mkUser(r, A, uid, tag, roleIds);
  const e = await r.call(`Employee profile for ${tag}`, tn(A, "POST", "/hr/employees", empBody(u.id, `${tag}-${uid}`.toUpperCase().slice(0, 30))), 201);
  const L = await login(r, `Login ${tag}`, A.slug, u.email, PASSWORD);
  r.need(L.at && str(e, "data.id"), `${tag} setup failed`);
  return { S: { ...A, accessToken: L.at }, userId: u.id, empId: str(e, "data.id") };
}

/** A user holding exactly the given permissions (tenant scope). */
async function limited(r: Runner, A: Session, uid: string, tag: string, perms: [string, string][]): Promise<Session> {
  const ids: string[] = [];
  for (const [res, act] of perms) ids.push(await permId(r, A, res, act));
  const role = await r.call(`Role {${perms.map((p) => p.join(":")).join(", ")}}`, tn(A, "POST", "/roles", { name: `dt-${tag}-${uid}`, permission_ids: ids }), 201);
  const u = await mkUser(r, A, uid, tag, [str(role, "data.id")]);
  const L = await login(r, `Login ${tag}`, A.slug, u.email, PASSWORD);
  return { ...A, accessToken: L.at };
}

export const S_ATTENDANCE: Scenario[] = [
  {
    key: "hr.att.flow",
    module: "attendance",
    title: "Shift, assignment, punches and records (G2, AT-004)",
    description: "Creates a shift, assigns it to a fresh employee, punches IN (with location), checks the duplicate-punch guard, punches OUT, then reads today, records and the summary.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "att");
      const s = await r.call("Create shift 00:00-23:59", tn(A, "POST", "/hr/shifts", { name: `Day ${uid}`, start_time: "00:00", end_time: "23:59", grace_period_mins: 0, break_duration_mins: 0 }), 201);
      await r.call("Assign shift to the employee", tn(A, "POST", `/hr/shifts/${str(s, "data.id")}/assignments`, { employee_id: m.empId, effective_from: ymd(addDays(new Date(), -30)) }), 201);
      const pin = await r.call("Punch IN (with location)", tn(m.S, "POST", "/hr/attendance/punch", { type: "in", latitude: 12.9716, longitude: 77.5946, device_id: "console" }), 201, (e) =>
        str(e, "data.record.status") === "present" && !JSON.stringify(e.resBody).includes("ip_address") ? null : "record must be present and never expose the IP",
      );
      await r.call("Second IN within 60 s → 409 DUPLICATE_PUNCH", tn(m.S, "POST", "/hr/attendance/punch", { type: "in" }), 409);
      await r.call("Today shows an open session", tn(m.S, "GET", "/hr/attendance/me/today"), 200, (e) => (getPath(e.resBody, "data.open_session") === true ? null : "open_session expected"));
      await r.call("Punch OUT", tn(m.S, "POST", "/hr/attendance/punch", { type: "out" }), [201, 409]);
      await r.call("Record detail lists the punches", tn(A, "GET", `/hr/attendance/records/${str(pin, "data.record.id")}`), 200, (e) => (list(e, "data.punches").length >= 1 ? null : "punches missing"));
      await r.call("Employee's own records", tn(m.S, "GET", "/hr/attendance/me"), 200);
      await r.call("Tenant summary", tn(A, "GET", "/hr/attendance/summary"), 200, (e) => (num(e, "data.total_employees") >= 1 ? null : "summary empty"));
    },
  },
  {
    key: "hr.att.reg",
    module: "attendance",
    title: "Regularization lifecycle and self-approval guard (G6–G8)",
    description: "An employee requests a correction for yesterday; a second request is refused while one is pending; an approver cannot approve their own; another approver approves and the record becomes regularized with 9 h worked.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "reg");
      const y = ymd(addDays(new Date(), -1));
      const body = { attendance_date: y, requested_punch_in: `${y}T09:00:00Z`, requested_punch_out: `${y}T18:00:00Z`, reason: "forgot to punch" };
      const req = await r.call("Employee requests regularization", tn(m.S, "POST", "/hr/attendance/regularizations", body), 201);
      await r.call("Second pending request → 409 REGULARIZATION_PENDING", tn(m.S, "POST", "/hr/attendance/regularizations", body), 409);
      const approver = await member(r, A, uid, "apr");
      const role = await r.call("Role {attendance:approve}", tn(A, "POST", "/roles", { name: `dt-aa-${uid}`, permission_ids: [await permId(r, A, "attendance", "approve"), await permId(r, A, "attendance", "read")] }), 201);
      await r.call("Give the approver the role", tn(A, "POST", `/users/${approver.userId}/roles`, { role_ids: [str(role, "data.id")] }), 200);
      const own = await r.call("Approver files their own request", tn(approver.S, "POST", "/hr/attendance/regularizations", body), 201);
      await r.call("Approving own request → 403 SELF_APPROVAL_FORBIDDEN", tn(approver.S, "POST", `/hr/attendance/regularizations/${str(own, "data.id")}/approve`), 403);
      await r.call("Approver approves the employee's request", tn(approver.S, "POST", `/hr/attendance/regularizations/${str(req, "data.id")}/approve`, { comment: "ok" }), 200);
      await r.call("Record is regularized with 540 minutes", tn(A, "GET", "/hr/attendance/records", undefined, { employee_id: m.empId, from: y, to: y }), 200, (e) =>
        getPath(e.resBody, "data.0.is_regularized") === true && num(e, "data.0.total_work_minutes") === 540 ? null : "record not regularized",
      );
      await r.call("Reviewing again → 409 REGULARIZATION_NOT_PENDING", tn(approver.S, "POST", `/hr/attendance/regularizations/${str(req, "data.id")}/reject`, { comment: "x" }), 409);
    },
  },
  {
    key: "hr.att.rbac",
    module: "attendance",
    title: "Exact-grant RBAC (G9)",
    description: "A plain employee can punch and read their own records but not tenant data; attendance:read reads but cannot manage shifts.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "rb");
      await r.call("Employee: GET /hr/attendance/records → 403", tn(m.S, "GET", "/hr/attendance/records"), 403);
      await r.call("Employee: POST /hr/shifts → 403", tn(m.S, "POST", "/hr/shifts", {}), 403);
      await r.call("Employee: self records → 200", tn(m.S, "GET", "/hr/attendance/me"), 200);
      const reader = await limited(r, A, uid, "rd", [["attendance", "read"]]);
      await r.call("attendance:read lists records", tn(reader, "GET", "/hr/attendance/records"), 200);
      await r.call("attendance:read cannot create shifts → 403", tn(reader, "POST", "/hr/shifts", { name: "X", start_time: "09:00", end_time: "18:00" }), 403);
    },
  },
];

export const S_LEAVE: Scenario[] = [
  {
    key: "hr.lv.flow",
    module: "leave",
    title: "Preview, apply, approve, cancel, adjust (G5, G8, G13)",
    description: "Creates a 12-day leave type, previews and applies for 2 days (balance reserves 2), approves (attendance marks on_leave), cancels the approved future leave, then adjusts the balance.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "lv");
      const t = await r.call("Create leave type EL (12/yr)", tn(A, "POST", "/hr/leave/types", { name: `EL ${uid}`, code: `E${uid}`.slice(0, 10).toUpperCase(), annual_allowance: 12, carry_forward_limit: 5 }), 201);
      const typ = str(t, "data.id");
      const mon = monday(0);
      const tue = addDays(mon, 1);
      const range = { leave_type_id: typ, start_date: ymd(mon), end_date: ymd(tue), reason: "family wedding" };
      await r.call("Preview: 2 days, 10 left after", tn(m.S, "POST", "/hr/leave/requests/preview", range), 200, (e) =>
        num(e, "data.total_days") === 2 && num(e, "data.available_after") === 10 ? null : "preview mismatch",
      );
      const req = await r.call("Apply", tn(m.S, "POST", "/hr/leave/requests", range), 201);
      await r.call("Balance: reserved 2, available 10", tn(m.S, "GET", "/hr/leave/balances/me"), 200, (e) => {
        const b = list(e).find((x) => x.leave_type_id === typ);
        return b && b.reserved === 2 && b.available === 10 ? null : "balance mismatch";
      });
      await r.call("Overlapping request → 409", tn(m.S, "POST", "/hr/leave/requests", range), 409);
      await r.call("Approve", tn(A, "POST", `/hr/leave/requests/${str(req, "data.id")}/approve`, { comment: "enjoy" }), 200);
      await r.call("Attendance shows on_leave for Monday", tn(A, "GET", "/hr/attendance/records", undefined, { employee_id: m.empId, from: ymd(mon), to: ymd(mon) }), 200, (e) =>
        getPath(e.resBody, "data.0.status") === "on_leave" ? null : "attendance not marked on_leave",
      );
      await r.call("Employee cancels the approved future leave", tn(m.S, "POST", `/hr/leave/requests/${str(req, "data.id")}/cancel`), 200);
      await r.call("Adjust balance −1.5 with a reason", tn(A, "POST", "/hr/leave/balances/adjust", { employee_id: m.empId, leave_type_id: typ, days: -1.5, reason: "correction" }), 200, (e) =>
        num(e, "data.available") === 10.5 ? null : "available should be 10.5",
      );
      await r.call("Ledger lists every movement", tn(A, "GET", "/hr/leave/ledger", undefined, { employee_id: m.empId, leave_type_id: typ }), 200, (e) => (list(e).length >= 4 ? null : "ledger too short"));
    },
  },
  {
    key: "hr.lv.rbac",
    module: "leave",
    title: "Self-approval guard and RBAC (G9, G10)",
    description: "An approver cannot approve their own leave; a plain employee cannot read others' requests or manage types.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const t = await r.call("Create leave type CL", tn(A, "POST", "/hr/leave/types", { name: `CL ${uid}`, code: `C${uid}`.slice(0, 10).toUpperCase(), annual_allowance: 6 }), 201);
      const approver = await member(r, A, uid, "lap");
      const role = await r.call("Role {leave:approve, leave:read}", tn(A, "POST", "/roles", { name: `dt-la-${uid}`, permission_ids: [await permId(r, A, "leave", "approve"), await permId(r, A, "leave", "read")] }), 201);
      await r.call("Give the approver the role", tn(A, "POST", `/users/${approver.userId}/roles`, { role_ids: [str(role, "data.id")] }), 200);
      const mon = monday(1);
      const own = await r.call("Approver applies for leave", tn(approver.S, "POST", "/hr/leave/requests", { leave_type_id: str(t, "data.id"), start_date: ymd(mon), end_date: ymd(mon), reason: "x" }), 201);
      await r.call("Approving own leave → 403", tn(approver.S, "POST", `/hr/leave/requests/${str(own, "data.id")}/approve`), 403);
      const m = await member(r, A, uid, "lpl");
      await r.call("Employee: GET /hr/leave/requests → 403", tn(m.S, "GET", "/hr/leave/requests"), 403);
      await r.call("Employee: POST /hr/leave/types → 403", tn(m.S, "POST", "/hr/leave/types", {}), 403);
      await r.call("Employee: own requests → 200", tn(m.S, "GET", "/hr/leave/requests/me"), 200);
    },
  },
];

export const S_PAYROLL: Scenario[] = [
  {
    key: "hr.pay.flow",
    module: "payroll",
    title: "Structure, assignment, run with maker-checker, payslip visibility, payout CSV (G11–G13)",
    description: "Creates a structure and assigns ₹12 L CTC to a fresh employee, runs last month's payroll, proves the maker cannot approve, a checker approves and finalizes, the employee sees exactly one payslip and the CSV downloads.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "pay");
      const s = await r.call("Create salary structure", tn(A, "POST", "/hr/payroll/structures", {
        name: `Std ${uid}`,
        components: [
          { code: "BASIC", name: "Basic", kind: "earning", calc: "percent_of_ctc", value: 40 },
          { code: "HRA", name: "HRA", kind: "earning", calc: "percent_of_basic", value: 50 },
          { code: "SPECIAL", name: "Special", kind: "earning", calc: "balance" },
        ],
      }), 201);
      await r.call("Assign CTC ₹12,00,000", tn(A, "POST", "/hr/payroll/assignments", { employee_id: m.empId, structure_id: str(s, "data.id"), annual_ctc: 1200000, effective_from: "2026-01-15" }), 201);
      // The most recent past month without a run (re-runs in the same tenant walk backwards).
      const runs = await r.call("Existing runs", tn(A, "GET", "/hr/payroll/runs", undefined, { per_page: "100" }), 200);
      const taken = new Set(list(runs).map((x) => `${x.year}-${x.month}`));
      const prev = new Date();
      prev.setUTCDate(1);
      do prev.setUTCMonth(prev.getUTCMonth() - 1);
      while (taken.has(`${prev.getUTCFullYear()}-${prev.getUTCMonth() + 1}`));
      const run = await r.call(`Create run for ${prev.getUTCFullYear()}-${prev.getUTCMonth() + 1}`, tn(A, "POST", "/hr/payroll/runs", { month: prev.getUTCMonth() + 1, year: prev.getUTCFullYear() }), 201);
      const rid = str(run, "data.id");
      await r.call("Calculate", tn(A, "POST", `/hr/payroll/runs/${rid}/calculate`), 200);
      await r.call("Nothing visible to the employee before finalize", tn(m.S, "GET", "/hr/payroll/payslips/me"), 200, (e) => (list(e).length === 0 ? null : "payslip leaked before finalize"));
      await r.call("Maker approves own run → 403 SELF_APPROVAL_FORBIDDEN", tn(A, "POST", `/hr/payroll/runs/${rid}/approve`), 403);
      const checker = await limited(r, A, uid, "chk", [["payroll", "approve"]]);
      await r.call("Finalize before approve → 409", tn(checker, "POST", `/hr/payroll/runs/${rid}/finalize`), 409);
      await r.call("Checker approves", tn(checker, "POST", `/hr/payroll/runs/${rid}/approve`), 200);
      await r.call("Checker finalizes", tn(checker, "POST", `/hr/payroll/runs/${rid}/finalize`), 200);
      await r.call("Employee sees exactly their payslip", tn(m.S, "GET", "/hr/payroll/payslips/me"), 200, (e) => (list(e).length === 1 ? null : "expected one payslip"));
      await r.call("Payout CSV", tn(A, "GET", `/hr/payroll/runs/${rid}/payout.csv`), 200);
    },
  },
  {
    key: "hr.pay.rbac",
    module: "payroll",
    title: "Exact-grant RBAC and privacy",
    description: "A plain employee cannot read structures or runs; payroll:read can read but not calculate.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "prb");
      await r.call("Employee: GET /hr/payroll/structures → 403", tn(m.S, "GET", "/hr/payroll/structures"), 403);
      await r.call("Employee: GET /hr/payroll/runs → 403", tn(m.S, "GET", "/hr/payroll/runs"), 403);
      const reader = await limited(r, A, uid, "prd", [["payroll", "read"]]);
      await r.call("payroll:read lists runs", tn(reader, "GET", "/hr/payroll/runs"), 200);
      await r.call("payroll:read cannot create a run → 403", tn(reader, "POST", "/hr/payroll/runs", { month: 1, year: 2026 }), 403);
    },
  },
];

export const S_RECRUITMENT: Scenario[] = [
  {
    key: "hr.rec.flow",
    module: "recruitment",
    title: "Job → candidate → interview → offer → hire (G5–G10)",
    description: "Opens a job, adds a candidate (duplicate email refused), walks the pipeline, schedules an interview whose interviewer gives feedback, declines then accepts an offer and hires: an invited user and an employee profile are created and the job is filled.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const interviewer = await member(r, A, uid, "ivw");
      const job = await r.call("Create job (headcount 1)", tn(A, "POST", "/hr/recruitment/jobs", { title: `Engineer ${uid}`, headcount: 1, employment_type: "full_time", description: "Build things" }), 201);
      const jid = str(job, "data.id");
      await r.call("Open the job", tn(A, "POST", `/hr/recruitment/jobs/${jid}/status`, { status: "open" }), 200);
      const email = `cand.${uid}@example.com`;
      const c = await r.call("Add candidate", tn(A, "POST", "/hr/recruitment/candidates", { job_id: jid, first_name: "Priya", last_name: "Nair", email, source: "referral", expected_ctc: 1500000 }), 201);
      const cid = str(c, "data.id");
      await r.call("Same email again (any case) → 409", tn(A, "POST", "/hr/recruitment/candidates", { job_id: jid, first_name: "P", last_name: "N", email: email.toUpperCase(), source: "referral" }), 409);
      await r.call("Skipping screening → 409 INVALID_STAGE", tn(A, "POST", `/hr/recruitment/candidates/${cid}/stage`, { stage: "interview" }), 409);
      await r.call("Stage → screening", tn(A, "POST", `/hr/recruitment/candidates/${cid}/stage`, { stage: "screening" }), 200);
      await r.call("Stage → interview", tn(A, "POST", `/hr/recruitment/candidates/${cid}/stage`, { stage: "interview" }), 200);
      const when = new Date(Date.now() + 86400000).toISOString();
      const iv = await r.call("Schedule interview", tn(A, "POST", "/hr/recruitment/interviews", { candidate_id: cid, round_name: "System design", interviewer_employee_id: interviewer.empId, scheduled_at: when, duration_mins: 60 }), 201);
      await r.call("Interviewer sees their interview", tn(interviewer.S, "GET", "/hr/recruitment/interviews/me"), 200, (e) => (list(e).length === 1 ? null : "interview not visible"));
      const fb = { rating: 5, recommendation: "strong_hire", notes: "excellent" };
      await r.call("Non-interviewer feedback → 404", tn(A, "POST", `/hr/recruitment/interviews/${str(iv, "data.id")}/feedback`, fb), 404);
      await r.call("Interviewer feedback", tn(interviewer.S, "POST", `/hr/recruitment/interviews/${str(iv, "data.id")}/feedback`, fb), 200);
      const join = ymd(addDays(new Date(), 30));
      const o1 = await r.call("Offer", tn(A, "POST", "/hr/recruitment/offers", { candidate_id: cid, offered_ctc: 1800000, joining_date: join }), 201);
      await r.call("Second open offer → 409 OFFER_EXISTS", tn(A, "POST", "/hr/recruitment/offers", { candidate_id: cid, offered_ctc: 1800000, joining_date: join }), 409);
      await r.call("Hire before acceptance → 409 NOT_HIREABLE", tn(A, "POST", `/hr/recruitment/candidates/${cid}/hire`, { employee_code: `H-${uid}`.toUpperCase() }), 409);
      await r.call("Decline the offer", tn(A, "POST", `/hr/recruitment/offers/${str(o1, "data.id")}/decision`, { decision: "declined" }), 200);
      const o2 = await r.call("Second offer", tn(A, "POST", "/hr/recruitment/offers", { candidate_id: cid, offered_ctc: 1800000, joining_date: join }), 201);
      await r.call("Accept", tn(A, "POST", `/hr/recruitment/offers/${str(o2, "data.id")}/decision`, { decision: "accepted" }), 200);
      const hire = await r.call("Hire", tn(A, "POST", `/hr/recruitment/candidates/${cid}/hire`, { employee_code: `H-${uid}`.toUpperCase() }), 200, (e) =>
        str(e, "data.job_status") === "filled" ? null : "job should be filled",
      );
      await r.call("Hired employee profile exists", tn(A, "GET", `/hr/employees/${str(hire, "data.employee_id")}`), 200);
      await r.call("The hire created a pending invitation", tn(A, "GET", "/invitations", undefined, { status: "pending" }), 200, (e) =>
        list(e).some((x) => x.user_id === str(hire, "data.user_id")) ? null : "invitation missing",
      );
      await r.call("Filled job takes no more candidates → 409", tn(A, "POST", "/hr/recruitment/candidates", { job_id: jid, first_name: "L", last_name: "L", email: `late.${uid}@example.com`, source: "referral" }), 409);
    },
  },
];
