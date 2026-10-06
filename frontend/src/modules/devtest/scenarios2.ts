import { getPath } from "./api";
import { expectFields, str, type Runner, type Scenario } from "./runner";
import { PASSWORD, ZERO, listHas, login, mkUser, permId, roleId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

const JOIN = "2026-01-15T00:00:00Z";
const empBody = (userId: string, code: string, extra: Record<string, unknown> = {}) => ({
  user_id: userId,
  employee_code: code,
  first_name: "Dev",
  last_name: "Employee",
  employment_type: "full_time",
  joining_date: JOIN,
  ...extra,
});

async function mkEmployee(r: Runner, A: Session, uid: string, tag: string, roleIds: string[] = [], extra: Record<string, unknown> = {}) {
  const u = await mkUser(r, A, uid, tag, roleIds);
  r.need(u.id, "user id missing");
  const code = `E${tag.toUpperCase()}-${uid.toUpperCase()}`;
  const e = await r.call(`Create employee ${tag}`, tn(A, "POST", "/employees", empBody(u.id, code, extra)), 201, (x) => (str(x, "data.id") ? null : "no data.id"));
  return { user: u, id: str(e, "data.id"), code };
}

const timelineHas =
  (needle: string) =>
  (ex: Exchange): string | null => {
    const arr = (getPath(ex.resBody, "data") as { event_type: string }[] | undefined) ?? [];
    return arr.some((t) => t.event_type.includes(needle)) ? null : `timeline has no event containing "${needle}" (has: ${arr.map((t) => t.event_type).join(", ") || "none"})`;
  };

export const S2: Scenario[] = [
  {
    key: "s2.emp",
    module: 2,
    title: "Employee profile: create → read → update → read, uniqueness & validation",
    description: "Creates a Module 0 user then an employee profile for it. Verifies the response, a GET read-back, list/search, update, duplicate code/user rules and input validation.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const u = await mkUser(r, A, uid, "emp1");
      r.need(u.id, "user id missing");
      const code = `EMP-${uid.toUpperCase()}`;
      const c = await r.call(
        "Create employee (lower-case code is normalised)",
        tn(A, "POST", "/employees", empBody(u.id, code.toLowerCase(), { display_name: "Dev E", gender: "female", date_of_birth: "1992-04-10T00:00:00Z", personal_email: "dev@example.com", work_phone: "+911111111111", current_address: "Bengaluru", notice_period_days: 30 })),
        201,
        expectFields(["data.employee_code", code], ["data.user_id", u.id], ["data.status", "active"], ["data.employment.employment_type", "full_time"], ["data.contact.work_phone", "+911111111111"]),
      );
      const id = str(c, "data.id");
      r.need(id, "employee id missing");
      await r.call("GET employee (read-back)", tn(A, "GET", `/employees/${id}`), 200, expectFields(["data.id", id], ["data.first_name", "Dev"], ["data.display_name", "Dev E"], ["data.employment.notice_period_days", 30], ["data.contact.personal_email", "dev@example.com"]));
      await r.call("List employees contains it", tn(A, "GET", "/employees", undefined, { per_page: "100" }), 200, listHas(id));
      await r.call("Search by employee code finds it", tn(A, "GET", "/employees", undefined, { search: code }), 200, listHas(id));
      await r.call("Filter status=active contains it", tn(A, "GET", "/employees", undefined, { status: "active", per_page: "100" }), 200, listHas(id));
      await r.call("Filter status=terminated does not contain it", tn(A, "GET", "/employees", undefined, { status: "terminated", per_page: "100" }), 200, (e) => (listHas(id)(e) === null ? "active employee listed under status=terminated" : null));
      await r.call("Update first_name / gender / blood_group", tn(A, "PATCH", `/employees/${id}`, { first_name: "Changed", gender: "male", blood_group: "O+" }), 200);
      await r.call("GET after update", tn(A, "GET", `/employees/${id}`), 200, expectFields(["data.first_name", "Changed"], ["data.gender", "male"], ["data.blood_group", "O+"]));
      await r.call("Timeline has a 'hired' event after creation (FR-TL001)", tn(A, "GET", `/employees/${id}/timeline`), 200, timelineHas("hired"));
      await r.call("Duplicate employee_code → 409", tn(A, "POST", "/employees", empBody((await mkUser(r, A, uid, "emp1b")).id, code)), 409);
      await r.call("Second profile for the same user → 409", tn(A, "POST", "/employees", empBody(u.id, `X-${uid.toUpperCase()}`)), 409);
      await r.call("Employee for non-existent user → 404", tn(A, "POST", "/employees", empBody(ZERO, `Y-${uid.toUpperCase()}`)), 404);
      await r.call("Invalid employee_code format → 400", tn(A, "POST", "/employees", empBody(u.id, "bad code!")), 400);
      await r.call("Invalid employment_type → 400", tn(A, "POST", "/employees", empBody(u.id, `Z-${uid.toUpperCase()}`, { employment_type: "slave" })), 400);
      await r.call("Missing required fields → 400", tn(A, "POST", "/employees", {}), 400);
      await r.call("List meta uses total_items/total_pages like every other list", tn(A, "GET", "/employees", undefined, { per_page: "5" }), 200, (x) =>
        getPath(x.resBody, "meta.total_items") !== undefined && getPath(x.resBody, "meta.total_pages") !== undefined ? null : "meta lacks total_items/total_pages",
      );
      await r.call("Invalid joining_date → 400", { ...tn(A, "POST", "/employees"), rawBody: JSON.stringify({ ...empBody(u.id, `W-${uid.toUpperCase()}`), joining_date: "not-a-date" }) }, 400);
      await r.call("GET non-existent employee → 404", tn(A, "GET", `/employees/${ZERO}`), 404);
      await r.call("GET /employees/not-a-uuid → 400", tn(A, "GET", "/employees/not-a-uuid"), 400);
      const pr = await mkEmployee(r, A, uid, "prob", [], { probation_end_date: "2099-01-01T00:00:00Z" });
      if (pr.id) await r.call("Future probation_end_date → status 'probation'", tn(A, "GET", `/employees/${pr.id}`), 200, expectFields(["data.status", "probation"]));
    },
  },
  {
    key: "s2.status",
    module: 2,
    title: "Status transitions, timeline, deactivation & cross-module effects",
    description:
      "active → notice → resigned (+exit data) → deactivate, each followed by a GET. Also checks spec rules FR-ED002 (transition matrix), FR-ED005 (exit deactivates the Module 0 user) and FR-TL001 (timeline). Event-driven rule FR-EV001 needs RabbitMQ and is reported as skipped.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const e = await mkEmployee(r, A, uid, "stat");
      r.need(e.id, "employee id missing");
      await r.call("Transition active → notice (+notes)", tn(A, "POST", `/employees/${e.id}/status`, { status: "notice", notes: "gave notice" }), 200, expectFields(["data.status", "notice"]));
      await r.call("GET after transition → notice", tn(A, "GET", `/employees/${e.id}`), 200, expectFields(["data.status", "notice"]));
      await r.call("Timeline contains status_changed:notice", tn(A, "GET", `/employees/${e.id}/timeline`), 200, timelineHas("notice"));
      await r.call("Invalid status 'bogus' → 400", tn(A, "POST", `/employees/${e.id}/status`, { status: "bogus" }), 400);
      await r.call("Missing status → 400", tn(A, "POST", `/employees/${e.id}/status`, {}), 400);
      await r.call("Transition notice → resigned with exit date + reason", tn(A, "POST", `/employees/${e.id}/status`, { status: "resigned", resignation_date: "2026-09-01T00:00:00Z", exit_date: "2026-10-01T00:00:00Z", exit_reason: "better offer" }), 200);
      await r.call("GET after resign: status + employment.exit_reason persisted", tn(A, "GET", `/employees/${e.id}`), 200, expectFields(["data.status", "resigned"], ["data.employment.exit_reason", "better offer"]));
      await r.call("FR-ED004: exit dates were derived (resignation/exit date present)", tn(A, "GET", `/employees/${e.id}`), 200, (x) => (str(x, "data.employment.exit_date") && str(x, "data.employment.resignation_date") ? null : "exit_date / resignation_date missing"));
      await r.call("Timeline contains the 'resigned' milestone (FR-TL001)", tn(A, "GET", `/employees/${e.id}/timeline`), 200, timelineHas("resigned"));
      await r.call("FR-ED005: exit finalisation deactivates Module 0 user", tn(A, "GET", `/users/${e.user.id}`), 200, expectFields(["data.status", "inactive"]));
      await r.call("FR-ED002: illegal transition resigned → active is rejected", tn(A, "POST", `/employees/${e.id}/status`, { status: "active" }), [400, 409, 422]);
      await r.call("Transition to same status is a no-op (200)", tn(A, "POST", `/employees/${e.id}/status`, { status: "resigned" }), 200);
      await r.call("Status change on non-existent employee → 404", tn(A, "POST", `/employees/${ZERO}/status`, { status: "notice" }), 404);
      const p = await mkEmployee(r, A, uid, "conf", [], { probation_end_date: "2099-01-01T00:00:00Z" });
      if (p.id) {
        await r.call("FR-ED003: probation → active confirms the employee", tn(A, "POST", `/employees/${p.id}/status`, { status: "active" }), 200, expectFields(["data.status", "active"]));
        await r.call("confirmation_date is recorded", tn(A, "GET", `/employees/${p.id}`), 200, (x) => (str(x, "data.employment.confirmation_date") ? null : "confirmation_date missing"));
        await r.call("Timeline contains 'confirmed'", tn(A, "GET", `/employees/${p.id}/timeline`), 200, timelineHas("confirmed"));
        await r.call("Terminal state: terminated is final", tn(A, "POST", `/employees/${p.id}/status`, { status: "terminated", exit_reason: "dev test" }), 200);
        await r.call("terminated → probation rejected (409)", tn(A, "POST", `/employees/${p.id}/status`, { status: "probation" }), 409);
      }
      const e2 = await mkEmployee(r, A, uid, "stat2");
      if (e2.id) {
        await r.call("Deactivate employee", tn(A, "POST", `/employees/${e2.id}/deactivate`), [200, 204]);
        await r.call("GET after deactivation → inactive", tn(A, "GET", `/employees/${e2.id}`), 200, expectFields(["data.status", "inactive"]));
        await r.call("Deactivate non-existent employee → 404", tn(A, "POST", `/employees/${ZERO}/deactivate`), 404);
      }
      const cv = await mkEmployee(r, A, uid, "conv");
      if (cv.id) {
        await r.call("FR-EV001: deactivating a Module 0 user", tn(A, "POST", `/users/${cv.user.id}/deactivate`), 200);
        await r.call("…marks the employee inactive (synchronously, no broker needed)", tn(A, "GET", `/employees/${cv.id}`), 200, expectFields(["data.status", "inactive"]));
        await r.call("…and appends a timeline entry", tn(A, "GET", `/employees/${cv.id}/timeline`), 200, timelineHas("inactive"));
      }
    },
  },
  {
    key: "s2.stat",
    module: 2,
    title: "Statutory & bank details: upsert, masking",
    description: "PUT stores values, GET masks by default (NFR-SEC002), ?unmasked=true reveals, upsert overwrites, validation of max length.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const e = await mkEmployee(r, A, uid, "bank");
      r.need(e.id, "employee id missing");
      await r.call("GET statutory before any PUT (200 empty or 404)", tn(A, "GET", `/employees/${e.id}/statutory`), [200, 404]);
      await r.call("PUT statutory (admin)", tn(A, "PUT", `/employees/${e.id}/statutory`, { tax_id: "ABCDE1234F", national_id: "9999888877776666", bank_name: "Test Bank", bank_account_number: "123456789012", bank_routing_swift: "TESTINBB" }), 200, expectFields(["data.bank_name", "Test Bank"]));
      await r.call("GET statutory is masked by default", tn(A, "GET", `/employees/${e.id}/statutory`), 200, expectFields(["data.tax_id", "****234F"], ["data.bank_account_number", "****9012"], ["data.bank_name", "Test Bank"]));
      await r.call("GET ?unmasked=true as tenant_admin reveals raw values", tn(A, "GET", `/employees/${e.id}/statutory`, undefined, { unmasked: "true" }), 200, expectFields(["data.tax_id", "ABCDE1234F"], ["data.bank_account_number", "123456789012"]));
      await r.call("GET ?unmasked=false stays masked", tn(A, "GET", `/employees/${e.id}/statutory`, undefined, { unmasked: "false" }), 200, expectFields(["data.tax_id", "****234F"]));
      await r.call("PUT again (upsert) changes bank name", tn(A, "PUT", `/employees/${e.id}/statutory`, { bank_name: "Other Bank", tax_id: "ABCDE1234F", national_id: "9999888877776666", bank_account_number: "123456789012", bank_routing_swift: "TESTINBB" }), 200);
      await r.call("GET after upsert shows new bank name", tn(A, "GET", `/employees/${e.id}/statutory`), 200, expectFields(["data.bank_name", "Other Bank"]));
      await r.call("PUT with tax_id > 100 chars → 400", tn(A, "PUT", `/employees/${e.id}/statutory`, { tax_id: "x".repeat(101) }), 400);
      await r.call("PUT for non-existent employee → 404", tn(A, "PUT", `/employees/${ZERO}/statutory`, { bank_name: "x" }), 404);
      await r.call("GET statutory of non-existent employee → 404", tn(A, "GET", `/employees/${ZERO}/statutory`), 404);
    },
  },
  {
    key: "s2.doc",
    module: 2,
    title: "Documents: register → list → verify, validation",
    description: "The backend stores document metadata (name, URL, size, mime) — there is no binary upload endpoint.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const e = await mkEmployee(r, A, uid, "doc");
      r.need(e.id, "employee id missing");
      const d = await r.call(
        "Register document",
        tn(A, "POST", `/employees/${e.id}/documents`, { document_type: "passport", file_name: "passport.pdf", file_url: "https://example.com/passport.pdf", file_size: 2048, mime_type: "application/pdf" }),
        201,
        expectFields(["data.document_type", "passport"], ["data.employee_profile_id", e.id]),
      );
      const did = str(d, "data.id");
      r.need(did, "document id missing");
      await r.call("List documents contains it, unverified", tn(A, "GET", `/employees/${e.id}/documents`), 200, (x) => {
        const m = listHas(did)(x);
        if (m) return m;
        const doc = (getPath(x.resBody, "data") as { id: string; verified_at?: string }[]).find((y) => y.id === did);
        return doc?.verified_at ? "document already verified before verification" : null;
      });
      await r.call("Verify document (employee:admin)", tn(A, "POST", `/employees/${e.id}/documents/${did}/verify`), 200);
      await r.call("List shows verified_at + verified_by", tn(A, "GET", `/employees/${e.id}/documents`), 200, (x) => {
        const doc = (getPath(x.resBody, "data") as { id: string; verified_at?: string; verified_by?: string }[]).find((y) => y.id === did);
        if (!doc?.verified_at) return "verified_at not set";
        return doc.verified_by === A.user?.id ? null : `verified_by ${doc.verified_by} != verifier ${A.user?.id}`;
      });
      await r.call("Verify non-existent document → 404", tn(A, "POST", `/employees/${e.id}/documents/${ZERO}/verify`), 404);
      await r.call("Invalid file_url → 400", tn(A, "POST", `/employees/${e.id}/documents`, { document_type: "x1", file_name: "a", file_url: "not a url", file_size: 1, mime_type: "a/b" }), 400);
      await r.call("file_size 0 → 400", tn(A, "POST", `/employees/${e.id}/documents`, { document_type: "x1", file_name: "a", file_url: "https://e.com/a", file_size: 0, mime_type: "a/b" }), 400);
      await r.call("Missing fields → 400", tn(A, "POST", `/employees/${e.id}/documents`, {}), 400);
      await r.call("Document for non-existent employee → 404", tn(A, "POST", `/employees/${ZERO}/documents`, { document_type: "x1", file_name: "a", file_url: "https://e.com/a", file_size: 1, mime_type: "a/b" }), 404);
      await r.call("List documents of non-existent employee → 404", tn(A, "GET", `/employees/${ZERO}/documents`), [404, 200]);
    },
  },
  {
    key: "s2.me",
    module: 2,
    title: "Self-service (/employees/me) as the employee",
    description: "Logs in as the employee's own user (member role, no employee:* permissions) and uses GET/PATCH /employees/me; then confirms that user cannot read others. Uses 1 login attempt.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const member = await roleId(r, A, "member");
      const e = await mkEmployee(r, A, uid, "self", member ? [member] : []);
      r.need(e.id, "employee id missing");
      const other = await mkEmployee(r, A, uid, "other");
      await r.call("Admin without a profile: GET /employees/me → 404", tn(A, "GET", "/employees/me"), 404);
      const L = await login(r, "Login as the employee", A.slug, e.user.email, PASSWORD, 200);
      r.need(L.at, "no token");
      const S = { ...A, accessToken: L.at };
      await r.call("GET /employees/me returns own profile", tn(S, "GET", "/employees/me"), 200, expectFields(["data.id", e.id], ["data.user_id", e.user.id]));
      await r.call("PATCH /employees/me updates phone + address", tn(S, "PATCH", "/employees/me", { personal_phone: "+910000000001", current_address: "Self Street 1" }), 200);
      await r.call("GET /employees/me shows updated contact", tn(S, "GET", "/employees/me"), 200, expectFields(["data.contact.personal_phone", "+910000000001"], ["data.contact.current_address", "Self Street 1"]));
      await r.call("emergency_contacts as free text is rejected (must be a JSON array, FR-EC001) → 400", tn(S, "PATCH", "/employees/me", { emergency_contacts: "Mom +910000000002" }), 400);
      await r.call("emergency_contacts entry without phone → 400", tn(S, "PATCH", "/employees/me", { emergency_contacts: JSON.stringify([{ name: "Mom" }]) }), 400);
      await r.call("emergency_contacts as JSON array of {name,relation,phone}", tn(S, "PATCH", "/employees/me", { emergency_contacts: JSON.stringify([{ name: "Mom", relation: "mother", phone: "+910000000002" }]) }), 200);
      await r.call("GET /employees/me returns the stored emergency contacts", tn(S, "GET", "/employees/me"), 200, (e) => (str(e, "data.contact.emergency_contacts").includes("Mom") ? null : "emergency contacts not persisted"));
      await r.call("Employee cannot list the directory (no employee:read) → 403", tn(S, "GET", "/employees"), 403);
      if (other.id) {
        await r.call("Employee cannot read someone else → 403", tn(S, "GET", `/employees/${other.id}`), 403);
        await r.call("Employee cannot read someone else's statutory → 403", tn(S, "GET", `/employees/${other.id}/statutory`), 403);
        await r.call("Employee cannot edit someone else → 403", tn(S, "PATCH", `/employees/${other.id}`, { first_name: "Hacked" }), 403);
      }
      await r.call("Employee cannot edit own core profile via /:id (no employee:update) → 403", tn(S, "PATCH", `/employees/${e.id}`, { first_name: "Self" }), 403);
    },
  },
  {
    key: "s2.rbac",
    module: 2,
    title: "Employee RBAC: read / update / update_sensitive / admin + PII masking",
    description: "Limited user with employee:read only. Checks every write is denied, that statutory data stays masked for them (including ?unmasked=true), then grants permissions one by one. Uses 1 login attempt.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const names = ["read", "create", "update", "update_sensitive", "admin"];
      const ids: Record<string, string> = {};
      for (const n of names) ids[n] = await permId(r, A, "employee", n);
      r.need(ids.read && ids.update_sensitive && ids.admin && ids.update && ids.create, "employee:* permissions not seeded");
      const target = await mkEmployee(r, A, uid, "rbacT");
      r.need(target.id, "target employee missing");
      await r.call("admin stores statutory data", tn(A, "PUT", `/employees/${target.id}/statutory`, { tax_id: "ABCDE1234F", bank_account_number: "123456789012", bank_name: "B" }), 200);
      const doc = await r.call("admin registers a document", tn(A, "POST", `/employees/${target.id}/documents`, { document_type: "id", file_name: "a.pdf", file_url: "https://e.com/a.pdf", file_size: 5, mime_type: "application/pdf" }), 201);
      const did = str(doc, "data.id");
      const role = await r.call("Create role {employee:read}", tn(A, "POST", "/roles", { name: `dt-emp-${uid}`, permission_ids: [ids.read] }), 201);
      const rid = str(role, "data.id");
      r.need(rid, "role id missing");
      const u = await mkUser(r, A, uid, "rbacL", [rid]);
      const L = await login(r, "Login as limited user", A.slug, u.email, PASSWORD, 200);
      r.need(L.at, "no token");
      const S = { ...A, accessToken: L.at };
      await r.call("read-only: GET /employees → 200", tn(S, "GET", "/employees"), 200);
      await r.call("read-only: GET /employees/:id → 200", tn(S, "GET", `/employees/${target.id}`), 200);
      await r.call("read-only: GET timeline → 200", tn(S, "GET", `/employees/${target.id}/timeline`), 200);
      await r.call("read-only: GET documents → 200", tn(S, "GET", `/employees/${target.id}/documents`), 200);
      await r.call("read-only: statutory is MASKED", tn(S, "GET", `/employees/${target.id}/statutory`), 200, expectFields(["data.tax_id", "****234F"], ["data.bank_account_number", "****9012"]));
      await r.call("SECURITY: employee:read user adds ?unmasked=true → 403 (never raw PII)", tn(S, "GET", `/employees/${target.id}/statutory`, undefined, { unmasked: "true" }), 403, (x) =>
        JSON.stringify(x.resBody).includes("ABCDE1234F") ? "PII LEAK in an error response" : null,
      );
      await r.call("read-only: POST /employees → 403", tn(S, "POST", "/employees", empBody(u.id, `Q-${uid.toUpperCase()}`)), 403);
      await r.call("read-only: PATCH /employees/:id → 403", tn(S, "PATCH", `/employees/${target.id}`, { first_name: "X" }), 403);
      await r.call("read-only: POST status → 403", tn(S, "POST", `/employees/${target.id}/status`, { status: "notice" }), 403);
      await r.call("read-only: deactivate → 403", tn(S, "POST", `/employees/${target.id}/deactivate`), 403);
      await r.call("read-only: PUT statutory → 403", tn(S, "PUT", `/employees/${target.id}/statutory`, { bank_name: "hack" }), 403);
      await r.call("read-only: POST document → 403", tn(S, "POST", `/employees/${target.id}/documents`, { document_type: "id", file_name: "a", file_url: "https://e.com/a", file_size: 1, mime_type: "a/b" }), 403);
      if (did) await r.call("read-only: verify document → 403", tn(S, "POST", `/employees/${target.id}/documents/${did}/verify`), 403);
      await r.call("grant employee:update", tn(A, "POST", `/roles/${rid}/permissions`, { permission_ids: [ids.update] }), 200);
      await r.call("now PATCH /employees/:id → 200", tn(S, "PATCH", `/employees/${target.id}`, { first_name: "ByLimited" }), 200);
      await r.call("still: PUT statutory (needs update_sensitive) → 403", tn(S, "PUT", `/employees/${target.id}/statutory`, { bank_name: "hack" }), 403);
      await r.call("grant employee:update_sensitive", tn(A, "POST", `/roles/${rid}/permissions`, { permission_ids: [ids.update_sensitive] }), 200);
      await r.call("now PUT statutory → 200", tn(S, "PUT", `/employees/${target.id}/statutory`, { tax_id: "ABCDE1234F", bank_account_number: "123456789012", bank_name: "ByLimited" }), 200);
      if (did) await r.call("still: verify document (needs employee:admin) → 403", tn(S, "POST", `/employees/${target.id}/documents/${did}/verify`), 403);
      await r.call("still: unmasked read (needs employee:read_sensitive) → 403", tn(S, "GET", `/employees/${target.id}/statutory`, undefined, { unmasked: "true" }), 403);
      const pSens = await permId(r, A, "employee", "read_sensitive");
      r.need(pSens, "employee:read_sensitive permission missing (seed)");
      await r.call("grant employee:read_sensitive", tn(A, "POST", `/roles/${rid}/permissions`, { permission_ids: [pSens] }), 200);
      await r.call("now ?unmasked=true reveals the raw values", tn(S, "GET", `/employees/${target.id}/statutory`, undefined, { unmasked: "true" }), 200, expectFields(["data.tax_id", "ABCDE1234F"], ["data.bank_account_number", "123456789012"]));
      await r.call("default read stays masked even for read_sensitive holders", tn(S, "GET", `/employees/${target.id}/statutory`), 200, expectFields(["data.tax_id", "****234F"]));
      await r.call("unmasked access is audited (audit trail has employee.statutory_unmasked)", tn(A, "GET", "/audit-logs", undefined, { per_page: "100" }), 200, (x) =>
        ((getPath(x.resBody, "data") as { action: string }[]) ?? []).some((y) => y.action === "employee.statutory_unmasked") ? null : "unmasked read not audited",
      );
      await r.call("grant employee:admin", tn(A, "POST", `/roles/${rid}/permissions`, { permission_ids: [ids.admin] }), 200);
      if (did) await r.call("now verify document → 200", tn(S, "POST", `/employees/${target.id}/documents/${did}/verify`), 200);
      await r.call("cleanup: deactivate limited user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
    },
  },
  {
    key: "s2.errors",
    module: 2,
    title: "Employee validation & error handling",
    description: "No token, malformed JSON, invalid ids, pagination edge cases, filter edge cases.",
    needs: "A",
    run: async (r, { A, uid }) => {
      await r.call("No token → 401", tn(A, "GET", "/employees", undefined, undefined, null), 401);
      await r.call("Garbage token → 401", tn(A, "GET", "/employees", undefined, undefined, "x.y.z"), 401);
      await r.call("GET /employees/me without token → 401", tn(A, "GET", "/employees/me", undefined, undefined, null), 401);
      await r.call("Malformed JSON → 400", { ...tn(A, "POST", "/employees"), rawBody: "{oops" }, 400);
      await r.call("GET /employees/:id/timeline of non-existent employee → 404", tn(A, "GET", `/employees/${ZERO}/timeline`), [404, 200]);
      await r.call("PATCH non-existent employee → 404", tn(A, "PATCH", `/employees/${ZERO}`, { first_name: "x" }), 404);
      await r.call("Unknown status filter handled (not 500)", tn(A, "GET", "/employees", undefined, { status: "nonsense" }), [200, 400]);
      await r.call("Invalid department_id filter handled (not 500)", tn(A, "GET", "/employees", undefined, { department_id: "nope" }), [200, 400]);
      await r.call("per_page=1000 handled (not 500)", tn(A, "GET", "/employees", undefined, { per_page: "1000" }), [200, 400]);
      await r.call("page=-1 handled (not 500)", tn(A, "GET", "/employees", undefined, { page: "-1" }), [200, 400]);
      await r.call("per_page=0 handled (not 500)", tn(A, "GET", "/employees", undefined, { per_page: "0" }), [200, 400]);
      await r.call("Search with SQL metacharacters is safe (not 500)", tn(A, "GET", "/employees", undefined, { search: `'; DROP TABLE users;-- ${uid}` }), 200);
    },
  },
];
