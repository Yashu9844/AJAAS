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
  {
    key: "hr.att.ops",
    module: "attendance",
    title: "Manager inbox, policy, overtime, roster, auto-absent and bulk import (AG1–AG5)",
    description:
      "A manager (no HR permission) sees and decides a report's regularization while a stranger cannot; HR saves the policy and reads the overtime report; a two-segment roster is laid over a worker and an overlapping apply changes nothing; the auto-absent job marks only workers without a record and is idempotent; a bulk import with one bad row writes nothing, a clean one applies, a repeat is refused until overwrite.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const w = await member(r, A, uid, "ao");
      const boss = await member(r, A, uid, "ab");
      const peer = await member(r, A, uid, "ap");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Att ${uid}`, code: `AT${uid}`.toUpperCase() }), 201);
      await r.call("Worker reports to the manager", tn(A, "POST", "/hr/mappings", { user_id: w.userId, department_id: str(d, "data.id"), manager_user_id: boss.userId, is_primary: true }), 201);
      const day = ymd(addDays(new Date(), -1));
      const reg = await r.call("Worker requests a regularization", tn(w.S, "POST", "/hr/attendance/regularizations", {
        attendance_date: day, requested_punch_in: `${day}T09:00:00Z`, requested_punch_out: `${day}T18:00:00Z`, reason: "forgot" }), 201);
      const rid = str(reg, "data.id");
      await r.call("Manager inbox shows it", tn(boss.S, "GET", "/hr/attendance/inbox"), 200, (e) => (list(e).length === 1 ? null : "expected 1 item"));
      await r.call("Stranger inbox is empty", tn(peer.S, "GET", "/hr/attendance/inbox"), 200, (e) => (list(e).length === 0 ? null : "expected none"));
      await r.call("scope=all needs attendance:approve → 403", tn(boss.S, "GET", "/hr/attendance/inbox", undefined, { scope: "all" }), 403);
      await r.call("Stranger cannot decide → 404", tn(peer.S, "POST", `/hr/attendance/inbox/${rid}/approve`), 404);
      await r.call("Manager approves", tn(boss.S, "POST", `/hr/attendance/inbox/${rid}/approve`, { comment: "ok" }), 200);
      await r.call("Deciding again → 409", tn(boss.S, "POST", `/hr/attendance/inbox/${rid}/reject`, { comment: "late" }), 409);

      await r.call("Save the policy", tn(A, "PUT", "/hr/attendance/policy", { week_off_days: [0, 6], overtime_min_daily: 30, overtime_max_daily: 240,
        overtime_max_monthly: 3600, overtime_rate_weekday: 150, overtime_rate_week_off: 200, auto_absent_enabled: true }), 200);
      await r.call("Overtime report for the month", tn(A, "GET", "/hr/attendance/overtime/report", undefined, { month: day.slice(0, 7), employee_id: w.empId }), 200, (e) =>
        list(e, "data.lines").length === 1 ? null : "expected one line",
      );

      const mk = async (name: string, start: string, end: string) =>
        str(await r.call(`Shift ${name}`, tn(A, "POST", "/hr/shifts", { name: `${name} ${uid}`, start_time: start, end_time: end, grace_period_mins: 0, break_duration_mins: 0 }), 201), "data.id");
      const [early, late] = [await mk("Early", "06:00", "14:00"), await mk("Late", "14:00", "22:00")];
      const ro = await r.call("Roster: 7 early, 7 late", tn(A, "POST", "/hr/attendance/rosters", { name: `Rotation ${uid}`, pattern: [{ shift_id: early, days: 7 }, { shift_id: late, days: 7 }] }), 201);
      const from = ymd(addDays(new Date(), 60));
      await r.call("Apply for 2 cycles → 4 assignments", tn(A, "POST", `/hr/attendance/rosters/${str(ro, "data.id")}/apply`, { employee_ids: [w.empId], from, cycles: 2 }), 200, (e) =>
        num(e, "data.assignments") === 4 ? null : "expected 4",
      );
      await r.call("Overlapping apply → 409 ROSTER_OVERLAP", tn(A, "POST", `/hr/attendance/rosters/${str(ro, "data.id")}/apply`, { employee_ids: [boss.empId, w.empId], from, cycles: 1 }), 409);

      await r.call("Auto-absent for yesterday", tn(A, "POST", "/hr/attendance/jobs/auto-absent", { date: day }), 200);
      await r.call("Again → nothing new", tn(A, "POST", "/hr/attendance/jobs/auto-absent", { date: day }), 200, (e) =>
        num(e, "data.absent") === 0 && num(e, "data.week_off") === 0 ? null : "expected idempotent run",
      );
      await r.call("Today is outside the window → 400", tn(A, "POST", "/hr/attendance/jobs/auto-absent", { date: ymd(new Date()) }), 400);

      const d5 = ymd(addDays(new Date(), -5));
      const good = { employee_code: `ao-${uid}`.toUpperCase().slice(0, 30), date: d5, punch_in: `${d5}T09:00:00Z`, punch_out: `${d5}T18:00:00Z` };
      await r.call("Import with an unknown code writes nothing", tn(A, "POST", "/hr/attendance/imports", { rows: [good, { ...good, employee_code: "NOPE-0" }] }), 200, (e) =>
        num(e, "data.applied") === 0 && list(e, "data.errors").length === 1 ? null : "expected one row error and nothing applied",
      );
      await r.call("Dry run is clean", tn(A, "POST", "/hr/attendance/imports", { dry_run: true, rows: [good] }), 200, (e) => (list(e, "data.errors").length === 0 ? null : "expected no errors"));
      await r.call("Import applies", tn(A, "POST", "/hr/attendance/imports", { rows: [good] }), 200, (e) => (num(e, "data.applied") === 1 ? null : "expected 1 applied"));
      await r.call("Repeat is refused (EXISTS)", tn(A, "POST", "/hr/attendance/imports", { rows: [good] }), 200, (e) =>
        str(e, "data.errors.0.code") === "EXISTS" ? null : "expected EXISTS",
      );
      await r.call("Overwrite replaces", tn(A, "POST", "/hr/attendance/imports", { overwrite: true, rows: [good] }), 200, (e) => (num(e, "data.applied") === 1 ? null : "expected 1 applied"));
      await r.call("Import log", tn(A, "GET", "/hr/attendance/imports"), 200, (e) => (list(e).length === 2 ? null : "expected 2 imports"));
      await r.call("Worker cannot import → 403", tn(w.S, "POST", "/hr/attendance/imports", { rows: [good] }), 403);
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
  {
    key: "hr.leave.plus",
    module: "leave",
    title: "Location calendar, manager inbox, comp-off and encashment (LG2–LG5)",
    description:
      "A worker hired at a location whose weekly off is Sunday only can take a Saturday that the tenant calendar would refuse; a local holiday removes a day only there. The worker's manager (no HR permission) decides the request from the inbox while a stranger cannot. Real work on a weekly off earns comp-off that the manager credits once and the expiry run takes back without going negative. Encashment respects the yearly cap and the minimum balance and debits exactly once.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const coreApprover = await limited(r, A, uid, "lca", [["core", "approve"], ["core", "read"]]);
      const w = await member(r, A, uid, "lw");
      const boss = await member(r, A, uid, "lb");
      const peer = await member(r, A, uid, "lp");
      const L = "/hr/leave";
      const pol = await r.call("Current leave policy", tn(A, "GET", `${L}/policy`), 200);
      const month = num(pol, "data.year_start_month") || 1;
      const el = str(await r.call("Leave type EL (12)", tn(A, "POST", `${L}/types`, { name: `EL ${uid}`, code: `EL${uid}`.slice(0, 10).toUpperCase(), annual_allowance: 12, carry_forward_limit: 5 }), 201), "data.id");
      const co = str(await r.call("Leave type CO (0)", tn(A, "POST", `${L}/types`, { name: `CO ${uid}`, code: `CO${uid}`.slice(0, 10).toUpperCase(), annual_allowance: 0, carry_forward_limit: 0 }), 201), "data.id");
      await r.call("Policy with the comp-off type (same leave year)", tn(A, "PUT", `${L}/policy`, { year_start_month: month, comp_off_leave_type_id: co, comp_off_expiry_days: 1 }), 200);
      const loc = str(await r.call("Location", tn(A, "POST", "/hr/locations", { code: `lp${uid}`.toLowerCase(), name: `Leave site ${uid}`, city: "Pune", state_code: "mh" }), 201), "data.id");

      const d = str(await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Lv ${uid}`, code: `LV${uid}`.toUpperCase() }), 201), "data.id");
      const fam = str(await r.call("Job family", tn(A, "POST", "/hr/core/job-families", { code: `LF${uid}`, name: `Leave family ${uid}` }), 201), "data.id");
      const gr = await r.call("Grade", tn(A, "POST", "/hr/core/grades", { code: `LG${uid}`, name: `Leave grade ${uid}`, rank: 70 }), [201, 409]);
      const grades = await r.call("Grades", tn(A, "GET", "/hr/core/grades"), 200);
      const grade = str(gr, "data.id") || String(list(grades).find((x) => x.code === `LG${uid}`)?.id ?? "");
      const prof = str(await r.call("Job profile", tn(A, "POST", "/hr/core/job-profiles", { code: `LP${uid}`, title: "Associate", family_id: fam, default_grade_id: grade, category: "staff", probation_days: 0 }), 201), "data.id");
      const pos = str(await r.call("Position", tn(A, "POST", "/hr/core/positions", { code: `LS${uid}`, title: "Leave seat", job_profile_id: prof, department_id: d }), 201), "data.id");
      const h = await r.call("Hire at the location: draft", tn(A, "POST", "/hr/core/actions", { type: "hire", employee_id: w.empId, effective_date: ymd(addDays(new Date(), -60)), reason_code: "console",
        changes: { position_id: pos, manager_employee_id: boss.empId, worker_type: "employee", employment_type: "full_time", location_id: loc } }), 201);
      await r.call("Hire: submit", tn(A, "POST", `/hr/core/actions/${str(h, "data.id")}/submit`), 200);
      await r.call("Hire: approve", tn(coreApprover, "POST", `/hr/core/actions/${str(h, "data.id")}/approve`), 200);

      const sat = addDays(monday(0), 5);
      const preview = (S: Session, from: Date, to: Date) => tn(S, "POST", `${L}/requests/preview`, { leave_type_id: el, start_date: ymd(from), end_date: ymd(to), reason: "x" });
      await r.call("Saturday is off by default → 400", preview(w.S, sat, sat), 400);
      await r.call("Location weekly off: Sunday only", tn(A, "PUT", `${L}/week-offs/${loc}`, { week_off_days: [0] }), 200);
      await r.call("Saturday now works at the location (1 day)", preview(w.S, sat, sat), 200, (e) => (num(e, "data.total_days") === 1 ? null : "expected 1 day"));
      await r.call("Other locations keep Saturday off → 400", preview(peer.S, sat, sat), 400);
      const lh = await r.call("Local holiday on Monday", tn(A, "POST", `${L}/location-holidays`, { location_id: loc, date: ymd(monday(0)), name: "Local festival" }), 201);
      await r.call("Mon+Tue at the location = 1 day", preview(w.S, monday(0), addDays(monday(0), 1)), 200, (e) => (num(e, "data.total_days") === 1 ? null : "expected 1 day"));
      await r.call("Mon+Tue elsewhere = 2 days", preview(peer.S, monday(0), addDays(monday(0), 1)), 200, (e) => (num(e, "data.total_days") === 2 ? null : "expected 2 days"));

      const req = await r.call("Worker applies for Mon+Tue next week", tn(w.S, "POST", `${L}/requests`, { leave_type_id: el, start_date: ymd(monday(1)), end_date: ymd(addDays(monday(1), 1)), reason: "family wedding" }), 201);
      const rid = str(req, "data.id");
      await r.call("Manager inbox shows it", tn(boss.S, "GET", `${L}/inbox`), 200, (e) => (list(e).length === 1 ? null : "expected 1 item"));
      await r.call("Stranger inbox is empty", tn(peer.S, "GET", `${L}/inbox`), 200, (e) => (list(e).length === 0 ? null : "expected none"));
      await r.call("Stranger cannot decide → 404", tn(peer.S, "POST", `${L}/inbox/${rid}/approve`), 404);
      await r.call("Team calendar shows the leave without its reason", tn(boss.S, "GET", `${L}/team-calendar`, undefined, { from: ymd(monday(1)), to: ymd(addDays(monday(1), 6)) }), 200, (e) =>
        list(e, "data.leaves").length === 1 && !JSON.stringify(e.resBody).includes("family wedding") ? null : "expected one leave, no reason",
      );
      await r.call("Manager approves", tn(boss.S, "POST", `${L}/inbox/${rid}/approve`, { comment: "enjoy" }), 200);

      let sun = addDays(new Date(), -3);
      while (sun.getUTCDay() !== 0) sun = addDays(sun, -1);
      const code = String(getPath((await r.call("Worker profile", tn(A, "GET", `/hr/employees/${w.empId}`), 200)).resBody, "data.employee_code"));
      const cReq = { worked_date: ymd(sun), days: 1, reason: "release support" };
      await r.call("Comp-off on a working day → 409", tn(w.S, "POST", `${L}/comp-off`, { worked_date: ymd(addDays(sun, 3)), days: 1, reason: "x" }), 409);
      await r.call("Comp-off without attendance → 409", tn(w.S, "POST", `${L}/comp-off`, cReq), 409);
      await r.call("Import the Sunday's work", tn(A, "POST", "/hr/attendance/imports", { rows: [{ employee_code: code, date: ymd(sun), punch_in: `${ymd(sun)}T09:00:00Z`, punch_out: `${ymd(sun)}T15:00:00Z` }] }), 200, (e) =>
        num(e, "data.applied") === 1 ? null : "import not applied",
      );
      const cr = await r.call("Comp-off request", tn(w.S, "POST", `${L}/comp-off`, cReq), 201);
      await r.call("Own approval → 403", tn(w.S, "POST", `${L}/comp-off/${str(cr, "data.id")}/approve`), 403);
      await r.call("Manager credits it", tn(boss.S, "POST", `${L}/comp-off/${str(cr, "data.id")}/approve`, { comment: "thanks" }), 200);
      await r.call("Credited twice → 409", tn(boss.S, "POST", `${L}/comp-off/${str(cr, "data.id")}/approve`), 409);
      await r.call("Expiry run takes it back", tn(A, "POST", `${L}/comp-off/expire`), 200, (e) => (num(e, "data.days_debited") >= 1 ? null : "expected a debit"));

      await r.call("Encashment rule: 3 days a year, keep 2", tn(A, "PUT", `${L}/encashment-rules/${el}`, { max_days_per_year: 3, min_balance_after: 2 }), 200);
      await r.call("Not encashable without a rule → 409", tn(w.S, "POST", `${L}/encashments`, { leave_type_id: co, days: 1 }), 409);
      await r.call("Over the yearly cap → 409", tn(w.S, "POST", `${L}/encashments`, { leave_type_id: el, days: 4 }), 409);
      const en = await r.call("Encash 2 days", tn(w.S, "POST", `${L}/encashments`, { leave_type_id: el, days: 2, reason: "cash needed" }), 201);
      await r.call("Own approval → 403", tn(w.S, "POST", `${L}/encashments/${str(en, "data.id")}/approve`), 403);
      await r.call("Manager approves (balance debited)", tn(boss.S, "POST", `${L}/encashments/${str(en, "data.id")}/approve`, { comment: "ok" }), 200);
      await r.call("Debited once → 409", tn(boss.S, "POST", `${L}/encashments/${str(en, "data.id")}/approve`), 409);
      await r.call("2 + 2 exceeds the cap → 409", tn(w.S, "POST", `${L}/encashments`, { leave_type_id: el, days: 2 }), 409);
      await r.call("Delete the local holiday", tn(A, "DELETE", `${L}/location-holidays/${str(lh, "data.id")}`), 204);
      await r.call("Delete the weekly-off pattern", tn(A, "DELETE", `${L}/week-offs/${loc}`), 204);
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

const isoDay = (offset: number) => ymd(addDays(new Date(), offset));

export const S_EMPLOYEE_RECORDS: Scenario[] = [
  {
    key: "hr.emp.records",
    module: 2,
    title: "Records and approval-gated change requests (FR-ER, FR-CR)",
    description:
      "A fresh worker maintains emergency contacts (single primary), education and prior employment (no overlaps), then asks for a bank change: the number is masked, a second request is refused, the worker cannot approve, an approver with employee:approve_changes approves and the bank details are applied.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const w = await member(r, A, uid, "rec");
      const me = (method: "GET" | "POST" | "PUT" | "DELETE", path: string, b?: unknown) => tn(w.S, method, `/hr/employees/me${path}`, b);
      await r.call("First emergency contact is primary", me("POST", "/emergency-contacts", { name: "Asha", relationship: "Mother", phone: "+919800000001" }), 201, (e) =>
        getPath(e.resBody, "data.is_primary") === true ? null : "not primary",
      );
      await r.call("New primary replaces it", me("POST", "/emergency-contacts", { name: "Ravi", relationship: "Brother", phone: "+919800000002", is_primary: true }), 201);
      await r.call("Exactly one primary, listed first", me("GET", "/emergency-contacts"), 200, (e) => {
        const rows = list(e);
        return rows.length === 2 && rows[0].name === "Ravi" && rows.filter((x) => x.is_primary).length === 1 ? null : "primary mismatch";
      });
      await r.call("Education", me("POST", "/education", { institution: "IIT Madras", qualification: "B.Tech", start_year: 2012, end_year: 2016 }), 201);
      await r.call("Prior employment", me("POST", "/prior-employment", { employer: "Acme", title: "Engineer", start_date: "2016-07-01", end_date: "2020-06-30" }), 201);
      await r.call("Overlapping period → 409 OVERLAP", me("POST", "/prior-employment", { employer: "Beta", title: "SE", start_date: "2020-01-01", end_date: "2021-01-01" }), 409);
      await r.call("HR reads the worker's education", tn(A, "GET", `/hr/employees/${w.empId}/education`), 200, (e) => (list(e).length === 1 ? null : "expected 1"));
      const cr = await r.call("Bank change request (number masked)", me("POST", "/change-requests", { type: "bank", reason: "salary account moved",
        payload: { bank_name: "HDFC", bank_account_number: "50100123456789", bank_routing_swift: "HDFC0001234" } }), 201, (e) =>
        str(e, "data.payload.bank_account_number") === "****6789" ? null : "not masked",
      );
      await r.call("Second open bank request → 409", me("POST", "/change-requests", { type: "bank",
        payload: { bank_name: "SBI", bank_account_number: "12345678", bank_routing_swift: "SBIN0000001" } }), 409);
      await r.call("Worker cannot open the approval queue → 403", tn(w.S, "GET", "/hr/employee-change-requests"), 403);
      const approver = await limited(r, A, uid, "rap", [["employee", "approve_changes"]]);
      await r.call("Approver approves", tn(approver, "POST", `/hr/employee-change-requests/${str(cr, "data.id")}/approve`, { comment: "verified" }), 200, (e) =>
        str(e, "data.status") === "approved" ? null : "not approved",
      );
      await r.call("Deciding again → 409 REQUEST_STATE", tn(approver, "POST", `/hr/employee-change-requests/${str(cr, "data.id")}/reject`), 409);
      await r.call("Bank details applied (statutory, masked read)", tn(A, "GET", `/hr/employees/${w.empId}/statutory`), 200, (e) =>
        str(e, "data.bank_name") === "HDFC" ? null : "bank not applied",
      );
    },
  },
];

export const S_CORE: Scenario[] = [
  {
    key: "hr.core.flow",
    module: "core",
    title: "Job architecture → position → hire → promote → terminate → rescind (CG4–CG9)",
    description:
      "Builds a family, two grades, a job profile and a single seat; hires a fresh worker into it (the maker cannot approve their own action, an approver holding core:approve does), refuses a second incumbent, promotes, shows the as-of history, terminates (status, login and seat follow) and rescinds the termination (everything is restored).",
    needs: "A",
    run: async (r, { A, uid }) => {
      const approver = await limited(r, A, uid, "cap", [["core", "approve"], ["core", "read"]]);
      const w = await member(r, A, uid, "cw");
      const other = await member(r, A, uid, "co");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Core ${uid}`, code: `CO${uid}`.toUpperCase() }), 201);
      const dept = str(d, "data.id");
      const fam = await r.call("Job family", tn(A, "POST", "/hr/core/job-families", { code: `F${uid}`, name: `Engineering ${uid}` }), 201);
      const g1 = await r.call("Grade L1 (rank 1)", tn(A, "POST", "/hr/core/grades", { code: `L1${uid}`, name: "Level 1", rank: 1 }), [201, 409]);
      const g2 = await r.call("Grade L2 (rank 2)", tn(A, "POST", "/hr/core/grades", { code: `L2${uid}`, name: "Level 2", rank: 2 }), [201, 409]);
      const grades = await r.call("Grades by rank", tn(A, "GET", "/hr/core/grades"), 200);
      const byRank = (n: number) => String(list(grades).find((x) => x.rank === n)?.id ?? "");
      const [l1, l2] = [str(g1, "data.id") || byRank(1), str(g2, "data.id") || byRank(2)];
      const prof = await r.call("Job profile (default grade L1, 90-day probation)", tn(A, "POST", "/hr/core/job-profiles", {
        code: `SWE${uid}`, title: "Software Engineer", family_id: str(fam, "data.id"), default_grade_id: l1, category: "staff", probation_days: 90 }), 201);
      const pos = await r.call("Position (single seat)", tn(A, "POST", "/hr/core/positions", { code: `P${uid}`, title: "Backend seat",
        job_profile_id: str(prof, "data.id"), department_id: dept }), 201);
      const pid = str(pos, "data.id");
      const act = async (name: string, type: string, emp: string, date: string, changes: Record<string, unknown>, expectSubmit: number | number[] = 200) => {
        const c = await r.call(`${name}: draft`, tn(A, "POST", "/hr/core/actions", { type, employee_id: emp, effective_date: date, reason_code: "console", changes }), 201);
        await r.call(`${name}: submit`, tn(A, "POST", `/hr/core/actions/${str(c, "data.id")}/submit`), expectSubmit);
        return str(c, "data.id");
      };
      const hire = await act("Hire", "hire", w.empId, isoDay(-120), { position_id: pid, worker_type: "employee", employment_type: "full_time" });
      await r.call("Maker approves own action → 403 CORE_SELF_APPROVAL", tn(A, "POST", `/hr/core/actions/${hire}/approve`), 403);
      await r.call("Approver approves → completed", tn(approver, "POST", `/hr/core/actions/${hire}/approve`), 200, (e) =>
        str(e, "data.status") === "completed" && getPath(e.resBody, "data.effects_applied") === true ? null : "not completed",
      );
      await act("Second incumbent → 409 POSITION_FILLED", "hire", other.empId, isoDay(-60), { position_id: pid, worker_type: "employee", employment_type: "full_time" }, 409);
      const promo = await act("Promote to L2", "promote", w.empId, isoDay(-30), { grade_id: l2 });
      await r.call("Approve promotion", tn(approver, "POST", `/hr/core/actions/${promo}/approve`), 200);
      await r.call("As of 31 days ago: L1", tn(A, "GET", `/hr/core/workers/${w.empId}`, undefined, { as_of: isoDay(-31) }), 200, (e) =>
        str(e, "data.current.grade_id") === l1 ? null : "expected L1",
      );
      await r.call("Today: L2, history of 2", tn(A, "GET", `/hr/core/workers/${w.empId}`), 200, (e) =>
        str(e, "data.current.grade_id") === l2 && list(e, "data.history").length === 2 ? null : "expected L2",
      );
      const term = await act("Terminate (last day yesterday)", "terminate", w.empId, isoDay(-2), { termination_type: "voluntary", last_working_day: isoDay(-1) });
      await r.call("Approve termination", tn(approver, "POST", `/hr/core/actions/${term}/approve`), 200);
      await r.call("Employee is terminated", tn(A, "GET", `/hr/employees/${w.empId}`), 200, (e) => (str(e, "data.status") === "terminated" ? null : "status"));
      await r.call("Seat is vacant", tn(A, "GET", `/hr/core/positions/${pid}`), 200, (e) => (getPath(e.resBody, "data.vacant") === true ? null : "occupied"));
      await r.call("Rescind needs a reason → 400", tn(approver, "POST", `/hr/core/actions/${term}/rescind`, {}), 400);
      await r.call("Rescind the termination", tn(approver, "POST", `/hr/core/actions/${term}/rescind`, { reason: "resignation withdrawn" }), 200);
      await r.call("Employee is active again", tn(A, "GET", `/hr/employees/${w.empId}`), 200, (e) => (str(e, "data.status") === "active" ? null : "status"));
      await r.call("Seat is occupied again", tn(A, "GET", `/hr/core/positions/${pid}`), 200, (e) => (getPath(e.resBody, "data.vacant") === false ? null : "vacant"));
      await r.call("Headcount for the department", tn(A, "GET", "/hr/core/headcount", undefined, { department_id: dept }), 200, (e) =>
        getPath(e.resBody, "data.departments.0.workers") === 1 ? null : "expected 1 worker",
      );
    },
  },
  {
    key: "hr.core.rbac",
    module: "core",
    title: "RBAC: workers see themselves; core:read reads; only core:manage initiates",
    description: "A plain employee reads /workers/me but nothing else; core:read lists positions but cannot create actions.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "crb");
      await r.call("Employee: /workers/me → 200", tn(m.S, "GET", "/hr/core/workers/me"), 200);
      await r.call("Employee: positions → 403", tn(m.S, "GET", "/hr/core/positions"), 403);
      const reader = await limited(r, A, uid, "crd", [["core", "read"]]);
      await r.call("core:read lists positions", tn(reader, "GET", "/hr/core/positions"), 200);
      await r.call("core:read cannot create actions → 403", tn(reader, "POST", "/hr/core/actions", {}), 403);
    },
  },
];

const perfRatings =(...s: number[]) => ["Quality", "Delivery", "Collaboration", "Ownership"].map((criterion, i) => ({ criterion, score: s[i] }));

export const S_PERFORMANCE: Scenario[] = [
  {
    key: "hr.perf.flow",
    module: "performance",
    title: "Cycle → goals/OKRs → reviews → 9-box → completion (G4–G11)",
    description:
      "Alice reports to a fresh manager through an org mapping; Bob is a colleague. Runs a cycle: weight cap, key-result progress, manager access and colleague denial, review launch (idempotent), peer assignment, submissions, the 9-box, completion and anonymous peer visibility.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const mgr = await member(r, A, uid, "pmg");
      const alice = await member(r, A, uid, "pal");
      const bob = await member(r, A, uid, "pbo");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Perf ${uid}`, code: `PF${uid}`.toUpperCase() }), 201);
      await r.call("Alice reports to the manager", tn(A, "POST", "/hr/mappings", { user_id: alice.userId, department_id: str(d, "data.id"), is_primary: true, manager_user_id: mgr.userId }), 201);
      const cy = await r.call("Create cycle (default criteria)", tn(A, "POST", "/hr/performance/cycles", { title: `FY26 ${uid}`, start_date: "2026-01-01", end_date: "2026-06-30" }), 201, (e) =>
        list(e, "data.criteria").length === 4 ? null : "4 default criteria expected",
      );
      const cid = str(cy, "data.id");
      await r.call("Activate", tn(A, "POST", `/hr/performance/cycles/${cid}/activate`), 200);
      const goal = (S: Session, weight: number | string) => tn(S, "POST", "/hr/performance/goals", { cycle_id: cid, title: `Goal ${weight}`, weight });
      const g1 = await r.call("Alice: goal weight 60", goal(alice.S, 60), 201);
      await r.call("Alice: goal weight 40", goal(alice.S, 40), 201);
      await r.call("Weight 0.01 more → 409 WEIGHT_EXCEEDED", goal(alice.S, "0.01"), 409);
      const gid = str(g1, "data.id");
      const kr = await r.call("Key result 0 → 10", tn(alice.S, "POST", `/hr/performance/goals/${gid}/key-results`, { title: "Ship features", start_value: 0, target_value: 10 }), 201);
      const krID = String(list(kr, "data.key_results")[0]?.id ?? "");
      await r.call("Key-result check-in 5 → goal 50%", tn(alice.S, "POST", `/hr/performance/goals/${gid}/check-ins`, { key_result_id: krID, value: 5, note: "half way" }), 201, (e) =>
        num(e, "data.progress") === 50 ? null : "progress should be 50",
      );
      await r.call("Direct progress on a KR goal → 409 HAS_KEY_RESULTS", tn(alice.S, "POST", `/hr/performance/goals/${gid}/check-ins`, { progress: 90 }), 409);
      await r.call("Manager edits the report's goal", tn(mgr.S, "PATCH", `/hr/performance/goals/${gid}`, { title: "Ship v2" }), 200);
      await r.call("Colleague reads the goal → 404", tn(bob.S, "GET", `/hr/performance/goals/${gid}`), 404);
      await r.call("Manager lists the report's goals", tn(mgr.S, "GET", "/hr/performance/goals", undefined, { employee_id: alice.empId }), 200, (e) => (list(e).length === 2 ? null : "two goals expected"));
      await r.call("Launch reviews: 1 self + 1 manager", tn(A, "POST", `/hr/performance/cycles/${cid}/launch-reviews`), 200, (e) =>
        num(e, "data.self_reviews_created") === 1 && num(e, "data.manager_reviews_created") === 1 ? null : "counts mismatch",
      );
      await r.call("Launch again creates nothing", tn(A, "POST", `/hr/performance/cycles/${cid}/launch-reviews`), 200, (e) =>
        num(e, "data.self_reviews_created") + num(e, "data.manager_reviews_created") === 0 ? null : "not idempotent",
      );
      const peer = await r.call("Assign Bob as peer", tn(A, "POST", "/hr/performance/reviews", { cycle_id: cid, employee_id: alice.empId, reviewer_employee_id: bob.empId, type: "peer" }), 201);
      const assigned = async (S: Session, who: string) => {
        const e = await r.call(`${who}: reviews to write`, tn(S, "GET", "/hr/performance/reviews/assigned", undefined, { status: "pending" }), 200, (x) => (list(x).length === 1 ? null : "one review expected"));
        return String(list(e)[0]?.id ?? "");
      };
      const mrev = await assigned(mgr.S, "Manager");
      const srev = await assigned(alice.S, "Alice");
      await r.call("Manager review without potential → 400", tn(mgr.S, "POST", `/hr/performance/reviews/${mrev}/submit`, { ratings: perfRatings(4, 4, 3, 3) }), 400);
      await r.call("Manager submits (overall 3.5, potential high)", tn(mgr.S, "POST", `/hr/performance/reviews/${mrev}/submit`, { ratings: perfRatings(4, 4, 3, 3), potential: "high" }), 200, (e) =>
        num(e, "data.overall_score") === 3.5 ? null : "overall should be 3.5",
      );
      await r.call("Submitting again → 409 REVIEW_STATE", tn(mgr.S, "POST", `/hr/performance/reviews/${mrev}/submit`, { ratings: perfRatings(4, 4, 3, 3), potential: "high" }), 409);
      await r.call("Alice submits her self review", tn(alice.S, "POST", `/hr/performance/reviews/${srev}/submit`, { ratings: perfRatings(5, 5, 5, 5) }), 200);
      await r.call("Bob submits the peer review", tn(bob.S, "POST", `/hr/performance/reviews/${str(peer, "data.id")}/submit`, { ratings: perfRatings(3, 3, 3, 3) }), 200);
      await r.call("Alice sees nothing before completion", tn(alice.S, "GET", "/hr/performance/reviews/me"), 200, (e) => (list(e).length === 0 ? null : "reviews visible too early"));
      await r.call("9-box: Alice high/high, goal score 30", tn(A, "GET", `/hr/performance/cycles/${cid}/nine-box`), 200, (e) => {
        const en = list(e, "data.entries").find((x) => x.employee_id === alice.empId);
        return en && en.box === "high/high" && en.goal_score === 30 ? null : "9-box entry mismatch";
      });
      await r.call("Complete the cycle", tn(A, "POST", `/hr/performance/cycles/${cid}/complete`), 200);
      await r.call("Alice sees 3 reviews; the peer is anonymous", tn(alice.S, "GET", "/hr/performance/reviews/me"), 200, (e) => {
        const rows = list(e);
        return rows.length === 3 && rows.every((x) => x.type !== "peer" || x.reviewer_employee_id === undefined) ? null : "visibility mismatch";
      });
      await r.call("Check-in after completion → 409 CYCLE_STATE", tn(alice.S, "POST", `/hr/performance/goals/${gid}/check-ins`, { key_result_id: krID, value: 6 }), 409);
    },
  },
  {
    key: "hr.perf.calibration",
    module: "performance",
    title: "Calibration: overrides in review drive the 9-box and the final rating (G16, G17)",
    description:
      "Runs a cycle to review with a submitted manager review (3.5, medium), checks NOT_RATED and permission rules, calibrates twice (score, then potential), reads the history, sees the calibrated 9-box, completes and checks the subject's final rating.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const mgr = await member(r, A, uid, "cmg");
      const alice = await member(r, A, uid, "cal");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Cal ${uid}`, code: `CL${uid}`.toUpperCase() }), 201);
      await r.call("Alice reports to the manager", tn(A, "POST", "/hr/mappings", { user_id: alice.userId, department_id: str(d, "data.id"), is_primary: true, manager_user_id: mgr.userId }), 201);
      const cy = await r.call("Create cycle", tn(A, "POST", "/hr/performance/cycles", { title: `Cal ${uid}`, start_date: "2026-01-01", end_date: "2026-06-30" }), 201);
      const cid = str(cy, "data.id");
      await r.call("Activate", tn(A, "POST", `/hr/performance/cycles/${cid}/activate`), 200);
      await r.call("Alice sets a goal", tn(alice.S, "POST", "/hr/performance/goals", { cycle_id: cid, title: "Goal", weight: 50 }), 201);
      const calib = (S: Session, b: Record<string, unknown>) => tn(S, "POST", `/hr/performance/cycles/${cid}/calibrations`, { employee_id: alice.empId, ...b });
      await r.call("Calibrate while active → 409 CYCLE_STATE", calib(A, { overall_score: 4.25, reason: "calibration session" }), 409);
      await r.call("Launch reviews", tn(A, "POST", `/hr/performance/cycles/${cid}/launch-reviews`), 200);
      await r.call("Calibrate before the manager review → 409 NOT_RATED", calib(A, { overall_score: 4.25, reason: "calibration session" }), 409);
      const mr = await r.call("Manager: reviews to write", tn(mgr.S, "GET", "/hr/performance/reviews/assigned"), 200);
      await r.call("Manager submits (3.5, medium)", tn(mgr.S, "POST", `/hr/performance/reviews/${String(list(mr)[0]?.id ?? "")}/submit`, { ratings: perfRatings(4, 4, 3, 3), potential: "medium" }), 200);
      await r.call("Employee cannot calibrate → 403", calib(alice.S, { overall_score: 5, reason: "I deserve more" }), 403);
      await r.call("No-op calibration → 400", calib(A, { overall_score: 3.5, potential: "medium", reason: "nothing changes" }), 400);
      await r.call("Calibrate score 3.5 → 4.25", calib(A, { overall_score: 4.25, reason: "delivery underrated across teams" }), 201, (e) =>
        num(e, "data.before_score") === 3.5 && num(e, "data.after_score") === 4.25 ? null : "before/after mismatch",
      );
      await r.call("Calibrate potential → high", calib(A, { potential: "high", reason: "leadership in incident review" }), 201);
      await r.call("History has two rows", tn(A, "GET", `/hr/performance/cycles/${cid}/calibrations`, undefined, { employee_id: alice.empId }), 200, (e) => (list(e).length === 2 ? null : "expected 2"));
      await r.call("9-box: high/high, calibrated", tn(A, "GET", `/hr/performance/cycles/${cid}/nine-box`), 200, (e) => {
        const en = list(e, "data.entries").find((x) => x.employee_id === alice.empId);
        return en && en.box === "high/high" && en.calibrated === true ? null : "9-box not calibrated";
      });
      await r.call("Complete", tn(A, "POST", `/hr/performance/cycles/${cid}/complete`), 200);
      await r.call("Alice sees the calibrated final rating", tn(alice.S, "GET", "/hr/performance/reviews/me", undefined, { cycle_id: cid }), 200, (e) => {
        const f = getPath(e.resBody, "data.0.final") as Record<string, unknown> | undefined;
        return f && f.overall_score === 4.25 && f.potential === "high" && f.calibrated === true ? null : "final rating mismatch";
      });
    },
  },
  {
    key: "hr.perf.feedback",
    module: "performance",
    title: "Continuous feedback: praise wall, requests, answers, visibility (G18, G19)",
    description:
      "Bob praises Alice publicly; public constructive feedback is refused; Alice asks Bob for feedback (duplicates refused), Bob answers through the request; the manager sees the report's feedback, a colleague does not; Bob declines a second request.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const mgr = await member(r, A, uid, "fmg");
      const alice = await member(r, A, uid, "fal");
      const bob = await member(r, A, uid, "fbo");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Fb ${uid}`, code: `FB${uid}`.toUpperCase() }), 201);
      await r.call("Alice reports to the manager", tn(A, "POST", "/hr/mappings", { user_id: alice.userId, department_id: str(d, "data.id"), is_primary: true, manager_user_id: mgr.userId }), 201);
      const give = (S: Session, b: Record<string, unknown>) => tn(S, "POST", "/hr/performance/feedback", { to_employee_id: alice.empId, ...b });
      await r.call("Bob praises Alice publicly", give(bob.S, { kind: "praise", visibility: "public", message: "Great launch!" }), 201);
      await r.call("Public constructive → 400", give(bob.S, { kind: "constructive", visibility: "public", message: "x" }), 400);
      await r.call("Feedback to yourself → 400", give(alice.S, { kind: "praise", visibility: "private", message: "me" }), 400);
      const rq = await r.call("Alice asks Bob for feedback", tn(alice.S, "POST", "/hr/performance/feedback/requests", { responder_employee_ids: [bob.empId], question: "How did the launch go?" }), 201);
      const rid = String(list(rq)[0]?.id ?? "");
      await r.call("Asking again while pending → 409", tn(alice.S, "POST", "/hr/performance/feedback/requests", { responder_employee_ids: [bob.empId], question: "again" }), 409);
      await r.call("Bob sees the request", tn(bob.S, "GET", "/hr/performance/feedback/requests/assigned", undefined, { status: "pending" }), 200, (e) => (list(e).length === 1 ? null : "one expected"));
      await r.call("Manager cannot answer Bob's request → 404", give(mgr.S, { kind: "praise", visibility: "private", message: "m", request_id: rid }), 404);
      await r.call("Bob answers", give(bob.S, { kind: "constructive", visibility: "private", message: "Add a rollback plan", request_id: rid }), 201);
      await r.call("The request is answered", tn(alice.S, "GET", "/hr/performance/feedback/requests/mine", undefined, { status: "answered" }), 200, (e) => (list(e).length === 1 ? null : "not answered"));
      await r.call("Alice received 2", tn(alice.S, "GET", "/hr/performance/feedback/received"), 200, (e) => (list(e).length === 2 ? null : "expected 2"));
      await r.call("Manager sees the report's feedback", tn(mgr.S, "GET", "/hr/performance/feedback", undefined, { employee_id: alice.empId }), 200, (e) => (list(e).length === 2 ? null : "expected 2"));
      await r.call("Colleague cannot → 404", tn(bob.S, "GET", "/hr/performance/feedback", undefined, { employee_id: alice.empId }), 404);
      await r.call("Praise wall shows the public praise", tn(mgr.S, "GET", "/hr/performance/feedback/public"), 200, (e) =>
        list(e).some((x) => x.to_employee_id === alice.empId && x.kind === "praise") ? null : "praise missing",
      );
      const rq2 = await r.call("Manager asks Bob about Alice", tn(mgr.S, "POST", "/hr/performance/feedback/requests", { responder_employee_ids: [bob.empId], about_employee_id: alice.empId, question: "Strengths?" }), 201);
      const rid2 = String(list(rq2)[0]?.id ?? "");
      await r.call("Bob declines", tn(bob.S, "POST", `/hr/performance/feedback/requests/${rid2}/decline`), 200);
      await r.call("Declining twice → 409 REQUEST_STATE", tn(bob.S, "POST", `/hr/performance/feedback/requests/${rid2}/decline`), 409);
    },
  },
  {
    key: "hr.perf.rbac",
    module: "performance",
    title: "RBAC: cycles readable by all, managed by performance:manage (G13)",
    description: "A plain employee reads cycles but cannot create one or list all reviews; performance:read lists reviews but cannot create cycles.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const m = await member(r, A, uid, "prb");
      await r.call("Employee lists cycles → 200", tn(m.S, "GET", "/hr/performance/cycles"), 200);
      await r.call("Employee creates a cycle → 403", tn(m.S, "POST", "/hr/performance/cycles", { title: "x", start_date: "2026-01-01", end_date: "2026-02-01" }), 403);
      await r.call("Employee lists all reviews → 403", tn(m.S, "GET", "/hr/performance/reviews"), 403);
      await r.call("Employee: own goals → 200", tn(m.S, "GET", "/hr/performance/goals/me"), 200);
      const reader = await limited(r, A, uid, "prr", [["performance", "read"]]);
      await r.call("performance:read lists reviews", tn(reader, "GET", "/hr/performance/reviews"), 200);
      await r.call("performance:read cannot create a cycle → 403", tn(reader, "POST", "/hr/performance/cycles", { title: "x", start_date: "2026-01-01", end_date: "2026-02-01" }), 403);
    },
  },
];

export const S_COMPENSATION: Scenario[] = [
  {
    key: "hr.comp",
    module: "compensation",
    title: "Pay range → auto and approved changes → merit cycle with guideline, budget and maker-checker (CO-G2–G8)",
    description:
      "Hires a fresh worker into a graded seat, sets a pay range, records the first base (applies at once), requests a 30% promotion raise (the requester cannot approve; an approver holding compensation:approve does), checks compa-ratio and the worker's own view, then runs a merit cycle: budgets, open (snapshot), a proposal outside the guideline is refused, one over budget is refused, close, approve by someone other than the opener, apply, apply again is refused.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const coreApprover = await limited(r, A, uid, "kca", [["core", "approve"], ["core", "read"]]);
      const approver = await limited(r, A, uid, "kap", [["compensation", "approve"], ["compensation", "read"]]);
      const w = await member(r, A, uid, "kw");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Comp ${uid}`, code: `CP${uid}`.toUpperCase() }), 201);
      const dept = str(d, "data.id");
      const fam = await r.call("Job family", tn(A, "POST", "/hr/core/job-families", { code: `KF${uid}`, name: `Comp family ${uid}` }), 201);
      const g = await r.call("Grade", tn(A, "POST", "/hr/core/grades", { code: `K${uid}`, name: `Comp grade ${uid}`, rank: 50 }), [201, 409]);
      const grades = await r.call("Grades", tn(A, "GET", "/hr/core/grades"), 200);
      const grade = str(g, "data.id") || String(list(grades).find((x) => x.code === `K${uid}`)?.id ?? "");
      const prof = await r.call("Job profile", tn(A, "POST", "/hr/core/job-profiles", { code: `KP${uid}`, title: "Analyst", family_id: str(fam, "data.id"),
        default_grade_id: grade, category: "staff", probation_days: 0 }), 201);
      const pos = await r.call("Position", tn(A, "POST", "/hr/core/positions", { code: `KS${uid}`, title: "Analyst seat", job_profile_id: str(prof, "data.id"),
        department_id: dept }), 201);
      const h = await r.call("Hire: draft", tn(A, "POST", "/hr/core/actions", { type: "hire", employee_id: w.empId, effective_date: isoDay(-120), reason_code: "console",
        changes: { position_id: str(pos, "data.id"), worker_type: "employee", employment_type: "full_time" } }), 201);
      await r.call("Hire: submit", tn(A, "POST", `/hr/core/actions/${str(h, "data.id")}/submit`), 200);
      await r.call("Hire: approve", tn(coreApprover, "POST", `/hr/core/actions/${str(h, "data.id")}/approve`), 200);

      const B = "/hr/compensation";
      const rng = { grade_id: grade, min_amount: 800000, mid_amount: 1000000, max_amount: 1200000, effective_from: isoDay(-300) };
      await r.call("Pay range for the grade", tn(A, "POST", `${B}/pay-ranges`, rng), 201);
      await r.call("Same start again → 409 BACKDATED", tn(A, "POST", `${B}/pay-ranges`, rng), 409);
      await r.call("First base applies at once", tn(A, "POST", `${B}/changes`, { employee_id: w.empId, annual_base: 1000000, effective_from: isoDay(-120), reason: "hire" }),
        201, (e) => (str(e, "data.status") === "approved" ? null : "expected approved"));
      const big = await r.call("30% raise waits for approval", tn(A, "POST", `${B}/changes`, { employee_id: w.empId, annual_base: 1300000,
        effective_from: isoDay(-60), reason: "promotion" }), 201, (e) => (str(e, "data.status") === "pending" ? null : "expected pending"));
      await r.call("Requester approves → 403 COMP_SELF_APPROVAL", tn(A, "POST", `${B}/changes/${str(big, "data.id")}/approve`), 403);
      await r.call("Approver approves", tn(approver, "POST", `${B}/changes/${str(big, "data.id")}/approve`, { comment: "promotion" }), 200);
      await r.call("Worker view: 1.30 compa, above range, 2 records", tn(A, "GET", `${B}/workers/${w.empId}`), 200, (e) =>
        getPath(e.resBody, "data.position.compa_ratio") === 1.3 && str(e, "data.position.compa_band") === "above" && list(e, "data.history").length === 2
          ? null
          : "unexpected position",
      );
      await r.call("Worker sees own pay (/me)", tn(w.S, "GET", `${B}/me`), 200, (e) =>
        getPath(e.resBody, "data.current.annual_base") === 1300000 ? null : "expected 1300000",
      );
      await r.call("Worker cannot read others → 403", tn(w.S, "GET", `${B}/workers/${w.empId}`), 403);

      const cy = await r.call("Merit cycle (guideline: unrated/above 0–3%)", tn(A, "POST", `${B}/merit-cycles`, { name: `Merit ${uid}`, effective_date: isoDay(10),
        guidelines: [{ rating: "unrated", compa: "above", min_pct: 0, max_pct: 3 }] }), 201);
      const cid = str(cy, "data.id");
      await r.call("Budget 50,000 for the department", tn(A, "PUT", `${B}/merit-cycles/${cid}/budgets`, [{ department_id: dept, amount: 50000 }]), 200);
      await r.call("Open (snapshot proposals)", tn(A, "POST", `${B}/merit-cycles/${cid}/open`), 200);
      const props = await r.call("Proposals", tn(A, "GET", `${B}/merit-cycles/${cid}/proposals`), 200);
      const pid = String(list(props).find((x) => x.employee_id === w.empId)?.id ?? "");
      await r.call("5% is outside the guideline without justification → 400", tn(A, "PATCH", `${B}/proposals/${pid}`, { increase_pct: 5 }), 400);
      await r.call("4% with justification exceeds the budget → 409", tn(A, "PATCH", `${B}/proposals/${pid}`, { increase_pct: 4, justification: "star" }), 409);
      await r.call("3% → 39,000", tn(A, "PATCH", `${B}/proposals/${pid}`, { increase_pct: 3 }), 200, (e) =>
        getPath(e.resBody, "data.amount") === 39000 ? null : "expected 39000",
      );
      await r.call("Live budget usage", tn(A, "GET", `${B}/merit-cycles/${cid}/usage`), 200);
      await r.call("Close for review", tn(A, "POST", `${B}/merit-cycles/${cid}/close`), 200);
      await r.call("Opener approves → 403 COMP_SELF_APPROVAL", tn(A, "POST", `${B}/merit-cycles/${cid}/approve`), 403);
      await r.call("Approver approves the cycle", tn(approver, "POST", `${B}/merit-cycles/${cid}/approve`), 200);
      await r.call("Apply", tn(A, "POST", `${B}/merit-cycles/${cid}/apply`), 200);
      await r.call("Apply again → 409 CYCLE_STATE", tn(A, "POST", `${B}/merit-cycles/${cid}/apply`), 409);
      await r.call("History has the merit record", tn(A, "GET", `${B}/workers/${w.empId}`), 200, (e) =>
        list(e, "data.history").length === 3 ? null : "expected 3 records",
      );
    },
  },
];

export const S_ONBOARDING: Scenario[] = [
  {
    key: "hr.onb",
    module: "onboarding",
    title: "Hire → onboarding checklist → document task → termination → clearance → exit interview → rescind (OG2–OG8)",
    description:
      "Creates department-specific onboarding and generic offboarding templates, hires a fresh worker through core (the checklist starts by itself, once), shows the worker's own tasks, refuses a document task until the document is verified, completes the rest (the checklist closes), terminates (offboarding from the last working day), refuses completion before clearance, signs the four clearances, takes the exit interview (an onboarding:read holder cannot read it) and rescinds the termination (its checklist is cancelled).",
    needs: "A",
    run: async (r, { A, uid }) => {
      const coreApprover = await limited(r, A, uid, "oca", [["core", "approve"], ["core", "read"]]);
      const reader = await limited(r, A, uid, "ord", [["onboarding", "read"]]);
      const w = await member(r, A, uid, "ow");
      const d = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Onb ${uid}`, code: `ON${uid}`.toUpperCase() }), 201);
      const dept = str(d, "data.id");
      const fam = await r.call("Job family", tn(A, "POST", "/hr/core/job-families", { code: `OF${uid}`, name: `Onb family ${uid}` }), 201);
      const g = await r.call("Grade", tn(A, "POST", "/hr/core/grades", { code: `O${uid}`, name: `Onb grade ${uid}`, rank: 60 }), [201, 409]);
      const grades = await r.call("Grades", tn(A, "GET", "/hr/core/grades"), 200);
      const grade = str(g, "data.id") || String(list(grades).find((x) => x.code === `O${uid}`)?.id ?? "");
      const prof = await r.call("Job profile", tn(A, "POST", "/hr/core/job-profiles", { code: `OP${uid}`, title: "Associate", family_id: str(fam, "data.id"),
        default_grade_id: grade, category: "staff", probation_days: 0 }), 201);
      const pos = await r.call("Position", tn(A, "POST", "/hr/core/positions", { code: `OS${uid}`, title: "Associate seat", job_profile_id: str(prof, "data.id"),
        department_id: dept }), 201);

      const B = "/hr/onboarding";
      const t1 = await r.call("Onboarding template for the department", tn(A, "POST", `${B}/templates`, { kind: "onboarding", name: `Dept ${uid}`, department_id: dept }), 201);
      await r.call("Its tasks (document task is blocking)", tn(A, "PUT", `${B}/templates/${str(t1, "data.id")}/tasks`, [
        { title: "Upload ID proof", owner_role: "employee", document_type: `id_${uid}`, blocking: true },
        { title: "Laptop", owner_role: "it", due_offset_days: 1 },
      ]), 200);
      const t2 = await r.call("Offboarding template", tn(A, "POST", `${B}/templates`, { kind: "offboarding", name: `Exit ${uid}`, department_id: dept }), 201);
      await r.call("Its tasks", tn(A, "PUT", `${B}/templates/${str(t2, "data.id")}/tasks`, [{ title: "Return laptop", owner_role: "it" }]), 200);

      const act = async (name: string, type: string, date: string, changes: Record<string, unknown>) => {
        const c = await r.call(`${name}: draft`, tn(A, "POST", "/hr/core/actions", { type, employee_id: w.empId, effective_date: date, reason_code: "console", changes }), 201);
        await r.call(`${name}: submit`, tn(A, "POST", `/hr/core/actions/${str(c, "data.id")}/submit`), 200);
        await r.call(`${name}: approve`, tn(coreApprover, "POST", `/hr/core/actions/${str(c, "data.id")}/approve`), 200);
        return str(c, "data.id");
      };
      await act("Hire", "hire", isoDay(-3), { position_id: str(pos, "data.id"), worker_type: "employee", employment_type: "full_time" });
      const lst = await r.call("One onboarding checklist started by the hire", tn(A, "GET", `${B}/checklists`, undefined, { kind: "onboarding", employee_id: w.empId }),
        200, (e) => (list(e).length === 1 && list(e)[0].source === "core_action" ? null : "expected one core_action checklist"));
      const cid = String(list(lst)[0]?.id ?? "");
      const cl = await r.call("Checklist detail: 2 tasks", tn(A, "GET", `${B}/checklists/${cid}`), 200, (e) => (list(e, "data.tasks").length === 2 ? null : "expected 2 tasks"));
      const tasks = list(cl, "data.tasks");
      const [idTask, laptop] = [String(tasks[0]?.id ?? ""), String(tasks[1]?.id ?? "")];
      await r.call("Worker sees their task", tn(w.S, "GET", `${B}/my-tasks`), 200, (e) => (list(e).length === 1 ? null : "expected 1 task"));
      await r.call("Worker cannot do the IT task → 404", tn(w.S, "POST", `${B}/tasks/${laptop}/done`), 404);
      await r.call("Document task without a verified document → 409", tn(w.S, "POST", `${B}/tasks/${idTask}/done`), 409);
      const doc = await r.call("Register the document", tn(A, "POST", `/hr/employees/${w.empId}/documents`, { document_type: `id_${uid}`, file_name: "id.pdf",
        file_url: "https://example.com/id.pdf", file_size: 1024, mime_type: "application/pdf" }), 201);
      await r.call("Verify it", tn(A, "POST", `/hr/employees/${w.empId}/documents/${str(doc, "data.id")}/verify`), 200);
      await r.call("Worker completes the document task", tn(w.S, "POST", `${B}/tasks/${idTask}/done`, { note: "uploaded" }), 200, (e) =>
        str(e, "data.evidence_document_id") === str(doc, "data.id") ? null : "evidence missing",
      );
      await r.call("HR completes the IT task", tn(A, "POST", `${B}/tasks/${laptop}/done`), 200);
      await r.call("Checklist completed by itself", tn(A, "GET", `${B}/checklists/${cid}`), 200, (e) => (str(e, "data.status") === "completed" ? null : "not completed"));

      const term = await act("Terminate", "terminate", isoDay(0), { termination_type: "voluntary", last_working_day: isoDay(10) });
      const off = await r.call("Offboarding checklist from the last working day", tn(A, "GET", `${B}/checklists`, undefined, { kind: "offboarding", employee_id: w.empId }),
        200, (e) => (list(e).length === 1 && list(e)[0].anchor_date === isoDay(10) ? null : "expected one offboarding checklist"));
      const oid = String(list(off)[0]?.id ?? "");
      await r.call("Complete before clearance → 409 CLEARANCE_PENDING", tn(A, "POST", `${B}/checklists/${oid}/complete`), 409);
      for (const area of ["manager", "hr", "it", "finance"]) {
        await r.call(`Clear ${area}`, tn(A, "POST", `${B}/checklists/${oid}/clearances/${area}/clear`, { note: "ok" }), 200);
      }
      await r.call("Clear twice → 409", tn(A, "POST", `${B}/checklists/${oid}/clearances/hr/clear`), 409);
      await r.call("Worker submits the exit interview", tn(w.S, "PUT", `${B}/checklists/${oid}/exit-interview`, { answers: { why: "growth" } }), 201);
      await r.call("onboarding:read cannot read it → 404", tn(reader, "GET", `${B}/checklists/${oid}/exit-interview`), 404);
      await r.call("HR (exit_interview) reads it", tn(A, "GET", `${B}/checklists/${oid}/exit-interview`), 200);
      await r.call("Progress report", tn(A, "GET", `${B}/reports/progress`, undefined, { kind: "offboarding" }), 200);
      await r.call("Rescind the termination", tn(coreApprover, "POST", `/hr/core/actions/${term}/rescind`, { reason: "withdrawn" }), 200);
      await r.call("Offboarding checklist cancelled", tn(A, "GET", `${B}/checklists/${oid}`), 200, (e) => (str(e, "data.status") === "cancelled" ? null : "not cancelled"));
    },
  },
];
