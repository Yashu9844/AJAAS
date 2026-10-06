import { getPath } from "./api";
import { expectFields, str, type Scenario } from "./runner";
import { PASSWORD, ZERO, listHas, login, mkUser, permId, tn } from "./scenarios0";
import type { Exchange } from "./types";

const treeHas =
  (id: string) =>
  (ex: Exchange): string | null => {
    const walk = (nodes: unknown): boolean =>
      Array.isArray(nodes) && nodes.some((n) => (n as { id?: string }).id === id || walk((n as { children?: unknown }).children));
    return walk(getPath(ex.resBody, "data")) ? null : `department ${id} not present in org-chart tree`;
  };

const FORCE = { force: "true" };

export const S1: Scenario[] = [
  {
    key: "s1.dept",
    module: 1,
    title: "Departments: CRUD, hierarchy, cycle protection, deactivation rules",
    description: "CREATE parent+child → READ → UPDATE → READ → cycle/self-parent/duplicate-code negatives → deactivate rules (blocked while active team exists) → READ AGAIN.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const code = (s: string) => `${s}${uid.toUpperCase()}`;
      const p = await r.call("Create parent department", tn(A, "POST", "/departments", { name: `DT Parent ${uid}`, code: code("P"), description: "dev test" }), 201, expectFields(["data.status", "active"], ["data.depth", 0]));
      const pid = str(p, "data.id");
      r.need(pid, "parent id missing");
      const c = await r.call("Create child department (parent_department_id)", tn(A, "POST", "/departments", { name: `DT Child ${uid}`, code: code("C"), parent_department_id: pid }), 201, expectFields(["data.depth", 1], ["data.parent_department_id", pid]));
      const cid = str(c, "data.id");
      r.need(cid, "child id missing");
      await r.call("GET child (read-back): depth 1, parent set, path = [parent]", tn(A, "GET", `/departments/${cid}`), 200, expectFields(["data.parent_department_id", pid], ["data.name", `DT Child ${uid}`], ["data.path", [pid]]));
      await r.call("Update child name + description", tn(A, "PATCH", `/departments/${cid}`, { name: `DT Child renamed ${uid}`, description: "changed" }), 200);
      await r.call("GET after update", tn(A, "GET", `/departments/${cid}`), 200, expectFields(["data.name", `DT Child renamed ${uid}`], ["data.description", "changed"]));
      await r.call("Cycle: set parent's parent = its own child → rejected", tn(A, "PATCH", `/departments/${pid}`, { parent_department_id: cid }), [400, 409, 422]);
      await r.call("Self-parent → rejected", tn(A, "PATCH", `/departments/${pid}`, { parent_department_id: pid }), [400, 409]);
      await r.call("Duplicate department code → 409", tn(A, "POST", "/departments", { name: "dup", code: code("P") }), 409);
      await r.call("Non-existent parent_department_id → rejected", tn(A, "POST", "/departments", { name: `bad ${uid}`, parent_department_id: ZERO }), [400, 404]);
      await r.call("List departments contains parent", tn(A, "GET", "/departments", undefined, { per_page: "100" }), 200, listHas(pid));
      const t = await r.call("Create team inside parent", tn(A, "POST", "/teams", { name: `DT Team ${uid}`, department_id: pid }), 201);
      const tid = str(t, "data.id");
      r.need(tid, "team id missing");
      await r.call("Deactivate parent while team active (no force) → 409", tn(A, "POST", `/departments/${pid}/deactivate`), 409);
      await r.call("Deactivate team", tn(A, "POST", `/teams/${tid}/deactivate`), 200);
      await r.call("Deactivate child", tn(A, "POST", `/departments/${cid}/deactivate`), 200);
      await r.call("Deactivate parent (its only team is already inactive) → 200", tn(A, "POST", `/departments/${pid}/deactivate`, { reason: "dev test" }), 200);
      await r.call("cleanup: force-deactivate parent (only needed if previous step failed)", tn(A, "POST", `/departments/${pid}/deactivate`, { reason: "cleanup" }, FORCE), [200, 409]);
      await r.call("GET parent after deactivation → inactive", tn(A, "GET", `/departments/${pid}`), 200, expectFields(["data.status", "inactive"]));
      await r.call("Deactivate again → 409", tn(A, "POST", `/departments/${pid}/deactivate`), 409);
    },
  },
  {
    key: "s1.team",
    module: 1,
    title: "Teams: CRUD, lead user, department filter",
    description: "CREATE → READ → UPDATE → READ → list filtered by department → lead user validation → DEACTIVATE → READ.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const d = await r.call("Create department", tn(A, "POST", "/departments", { name: `DT TeamDept ${uid}` }), 201);
      const did = str(d, "data.id");
      r.need(did, "dept id missing");
      const u = await mkUser(r, A, uid, "lead");
      r.need(u.id, "user id missing");
      const c = await r.call("Create team", tn(A, "POST", "/teams", { name: `DT Team ${uid}`, department_id: did, code: `T${uid.toUpperCase()}` }), 201, expectFields(["data.department_id", did], ["data.status", "active"], ["data.member_count", 0]));
      const tid = str(c, "data.id");
      r.need(tid, "team id missing");
      await r.call("GET team (read-back)", tn(A, "GET", `/teams/${tid}`), 200, expectFields(["data.name", `DT Team ${uid}`]));
      await r.call("Update team name + lead user", tn(A, "PATCH", `/teams/${tid}`, { name: `DT Team renamed ${uid}`, lead_user_id: u.id }), 200);
      await r.call("GET after update", tn(A, "GET", `/teams/${tid}`), 200, expectFields(["data.name", `DT Team renamed ${uid}`], ["data.lead_user_id", u.id]));
      await r.call("List teams by department contains team", tn(A, "GET", "/teams", undefined, { department_id: did }), 200, listHas(tid));
      await r.call("Duplicate team code → 409", tn(A, "POST", "/teams", { name: "dup", department_id: did, code: `T${uid.toUpperCase()}` }), 409);
      await r.call("Team with non-existent department → rejected", tn(A, "POST", "/teams", { name: "x", department_id: ZERO }), [400, 404]);
      await r.call("Team with non-existent lead user → 404", tn(A, "POST", "/teams", { name: "x", department_id: did, lead_user_id: ZERO }), [400, 404]);
      await r.call("Team without department_id → 400", tn(A, "POST", "/teams", { name: "x" }), 400);
      await r.call("Deactivate team", tn(A, "POST", `/teams/${tid}/deactivate`), 200);
      await r.call("GET after deactivation → inactive", tn(A, "GET", `/teams/${tid}`), 200, expectFields(["data.status", "inactive"]));
      await r.call("Deactivate again → 409", tn(A, "POST", `/teams/${tid}/deactivate`), 409);
      await r.call("Deactivate department whose only team is inactive → 200", tn(A, "POST", `/departments/${did}/deactivate`), 200);
      await r.call("cleanup: force-deactivate department (only needed if previous step failed)", tn(A, "POST", `/departments/${did}/deactivate`, undefined, FORCE), [200, 409]);
      await r.call("cleanup: deactivate lead user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
    },
  },
  {
    key: "s1.desig",
    module: 1,
    title: "Designations: CRUD + validation",
    description: "CREATE → READ → UPDATE → READ → duplicate/invalid negatives → DEACTIVATE → READ.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const code = `D${uid.toUpperCase()}`;
      const c = await r.call("Create designation", tn(A, "POST", "/designations", { title: `DT Engineer ${uid}`, code, level: 3, description: "dev test" }), 201, expectFields(["data.level", 3], ["data.status", "active"]));
      const id = str(c, "data.id");
      r.need(id, "designation id missing");
      await r.call("GET designation (read-back)", tn(A, "GET", `/designations/${id}`), 200, expectFields(["data.title", `DT Engineer ${uid}`], ["data.code", code]));
      await r.call("Update title + level", tn(A, "PATCH", `/designations/${id}`, { title: `DT Senior ${uid}`, level: 5 }), 200);
      await r.call("GET after update", tn(A, "GET", `/designations/${id}`), 200, expectFields(["data.title", `DT Senior ${uid}`], ["data.level", 5]));
      await r.call("Duplicate code → 409", tn(A, "POST", "/designations", { title: "dup", code }), 409);
      await r.call("level 0 → 400", tn(A, "POST", "/designations", { title: "bad", level: 0 }), 400);
      await r.call("Missing title → 400", tn(A, "POST", "/designations", {}), 400);
      await r.call("List contains designation", tn(A, "GET", "/designations", undefined, { per_page: "100" }), 200, listHas(id));
      await r.call("Deactivate designation", tn(A, "POST", `/designations/${id}/deactivate`), 200);
      await r.call("GET after deactivation → inactive", tn(A, "GET", `/designations/${id}`), 200, expectFields(["data.status", "inactive"]));
      await r.call("Deactivate again → 409", tn(A, "POST", `/designations/${id}/deactivate`), 409);
    },
  },
  {
    key: "s1.map",
    module: 1,
    title: "Mappings: user ↔ department/team/designation/manager, rules, cross-module deactivation",
    description: "Includes FR-M005: deactivating a Module 0 user must converge (deactivate) that user's org mappings.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const d = await r.call("Create department", tn(A, "POST", "/departments", { name: `DT MapDept ${uid}` }), 201);
      const d2 = await r.call("Create 2nd department", tn(A, "POST", "/departments", { name: `DT MapDept2 ${uid}` }), 201);
      const did = str(d, "data.id");
      const did2 = str(d2, "data.id");
      const t = await r.call("Create team in dept 1", tn(A, "POST", "/teams", { name: `DT MapTeam ${uid}`, department_id: did }), 201);
      const tid = str(t, "data.id");
      const g = await r.call("Create designation", tn(A, "POST", "/designations", { title: `DT MapDesig ${uid}` }), 201);
      const gid = str(g, "data.id");
      r.need(did && did2 && tid && gid, "setup ids missing");
      const u1 = await mkUser(r, A, uid, "map1");
      const u2 = await mkUser(r, A, uid, "map2");
      r.need(u1.id && u2.id, "users missing");
      const m = await r.call("Create primary mapping for user 1", tn(A, "POST", "/mappings", { user_id: u1.id, department_id: did, team_id: tid, designation_id: gid, is_primary: true }), 201, expectFields(["data.user_id", u1.id], ["data.is_primary", true], ["data.status", "active"]));
      const mid = str(m, "data.id");
      r.need(mid, "mapping id missing");
      await r.call("GET mapping (read-back)", tn(A, "GET", `/mappings/${mid}`), 200, expectFields(["data.department_id", did], ["data.team_id", tid], ["data.designation_id", gid]));
      await r.call("List mappings of user 1 contains it", tn(A, "GET", "/mappings", undefined, { user_id: u1.id }), 200, listHas(mid));
      await r.call("Update: set manager = user 2", tn(A, "PATCH", `/mappings/${mid}`, { manager_user_id: u2.id }), 200);
      await r.call("GET after update shows manager", tn(A, "GET", `/mappings/${mid}`), 200, expectFields(["data.manager_user_id", u2.id]));
      await r.call("Second PRIMARY mapping for same user → 409", tn(A, "POST", "/mappings", { user_id: u1.id, department_id: did, is_primary: true }), 409);
      await r.call("Mapping with no dept/team/designation → 400", tn(A, "POST", "/mappings", { user_id: u2.id }), 400);
      await r.call("Team not in given department → 400", tn(A, "POST", "/mappings", { user_id: u2.id, department_id: did2, team_id: tid }), 400);
      await r.call("User cannot manage themselves → 400", tn(A, "POST", "/mappings", { user_id: u2.id, department_id: did, manager_user_id: u2.id }), 400);
      await r.call("Manager cycle (u2 managed by u1, u1 managed by u2) → 409", tn(A, "POST", "/mappings", { user_id: u2.id, department_id: did, manager_user_id: u1.id }), 409);
      await r.call("Mapping for non-existent user → 404", tn(A, "POST", "/mappings", { user_id: ZERO, department_id: did }), [400, 404]);
      await r.call("Deactivate user 1 (Module 0) → converges mappings (FR-M005)", tn(A, "POST", `/users/${u1.id}/deactivate`), 200);
      await r.call("GET mapping after user deactivation → inactive", tn(A, "GET", `/mappings/${mid}`), 200, expectFields(["data.status", "inactive"]));
      const m2 = await r.call("Create mapping for user 2", tn(A, "POST", "/mappings", { user_id: u2.id, department_id: did }), 201);
      const m2id = str(m2, "data.id");
      r.need(m2id, "mapping 2 id missing");
      await r.call("Deactivate mapping WITHOUT a JSON body (other deactivate endpoints accept that)", tn(A, "POST", `/mappings/${m2id}/deactivate`), 200);
      await r.call("Deactivate mapping with {reason} (retry)", tn(A, "POST", `/mappings/${m2id}/deactivate`, { reason: "dev test" }), [200, 409]);
      await r.call("GET after deactivation → inactive", tn(A, "GET", `/mappings/${m2id}`), 200, expectFields(["data.status", "inactive"]));
      await r.call("Deactivate again → 409", tn(A, "POST", `/mappings/${m2id}/deactivate`, {}), 409);
      await r.call("cleanup: deactivate team", tn(A, "POST", `/teams/${tid}/deactivate`), 200);
      await r.call("cleanup: force-deactivate department 1", tn(A, "POST", `/departments/${did}/deactivate`, undefined, FORCE), 200);
      await r.call("cleanup: force-deactivate department 2", tn(A, "POST", `/departments/${did2}/deactivate`, undefined, FORCE), 200);
      await r.call("cleanup: deactivate designation", tn(A, "POST", `/designations/${gid}/deactivate`), [200, 409]);
      await r.call("cleanup: deactivate user 2", tn(A, "POST", `/users/${u2.id}/deactivate`), 200);
    },
  },
  {
    key: "s1.chart",
    module: 1,
    title: "Org chart tree + reporting chain",
    description: "Builds department→child + manager relation, then checks GET /org-chart and GET /org-chart/chain reflect it.",
    needs: "A",
    run: async (r, { A, uid }) => {
      // The backend caches GET /org-chart per (tenant,max_depth,include_inactive) for 60s. Use keys derived from this run's uid so
      // content checks hit a cold key, and one extra "warm key" to test read-your-writes.
      const n = parseInt(uid, 36);
      const warm = { max_depth: String(3 + (n % 7)), include_inactive: n % 2 ? "true" : "false" };
      const cold = { max_depth: String(3 + ((n + 3) % 7)), include_inactive: n % 2 ? "false" : "true" };
      const one = { max_depth: "1", include_inactive: n % 2 ? "false" : "true" };
      await r.call("Warm the org-chart cache (key A)", tn(A, "GET", "/org-chart", undefined, warm), 200);
      const p = await r.call("Create parent dept", tn(A, "POST", "/departments", { name: `DT ChartP ${uid}` }), 201);
      const pid = str(p, "data.id");
      const c = await r.call("Create child dept", tn(A, "POST", "/departments", { name: `DT ChartC ${uid}`, parent_department_id: pid }), 201);
      const cid = str(c, "data.id");
      r.need(pid && cid, "dept ids missing");
      const boss = await mkUser(r, A, uid, "boss");
      const emp = await mkUser(r, A, uid, "emp");
      r.need(boss.id && emp.id, "users missing");
      await r.call("Map boss into parent dept", tn(A, "POST", "/mappings", { user_id: boss.id, department_id: pid, is_primary: true }), 201);
      await r.call("Map employee into child dept, manager = boss", tn(A, "POST", "/mappings", { user_id: emp.id, department_id: cid, is_primary: true, manager_user_id: boss.id }), 201);
      await r.call("Org chart (cold key) contains parent dept", tn(A, "GET", "/org-chart", undefined, cold), 200, treeHas(pid));
      await r.call("Org chart (cold key) contains child dept nested under parent", tn(A, "GET", "/org-chart", undefined, cold), 200, treeHas(cid));
      await r.call("Org chart max_depth=1 contains parent but omits nested child", tn(A, "GET", "/org-chart", undefined, one), 200, (e) => treeHas(pid)(e) ?? (treeHas(cid)(e) === null ? "child still present at max_depth=1" : null));
      await r.call("Read-your-writes: the key warmed BEFORE the writes must now show the new department", tn(A, "GET", "/org-chart", undefined, warm), 200, treeHas(pid));
      await r.call("Chain of employee: department + ancestors", tn(A, "GET", "/org-chart/chain", undefined, { user_id: emp.id }), 200, (e) => {
        const anc = (getPath(e.resBody, "data.ancestors") as { id: string }[] | undefined) ?? [];
        if (str(e, "data.department.id") !== cid) return "department.id != child dept";
        return anc.some((a) => a.id === pid) ? null : "ancestors does not include parent dept";
      });
      await r.call("Chain of boss: direct_reports includes employee", tn(A, "GET", "/org-chart/chain", undefined, { user_id: boss.id }), 200, (e) =>
        ((getPath(e.resBody, "data.direct_reports") as string[] | undefined) ?? []).includes(emp.id) ? null : "employee missing from direct_reports",
      );
      await r.call("Chain without user_id → 400", tn(A, "GET", "/org-chart/chain"), 400);
      await r.call("cleanup: deactivate employee (converges mappings)", tn(A, "POST", `/users/${emp.id}/deactivate`), 200);
      await r.call("cleanup: deactivate boss", tn(A, "POST", `/users/${boss.id}/deactivate`), 200);
      await r.call("cleanup: force-deactivate child dept", tn(A, "POST", `/departments/${cid}/deactivate`, undefined, FORCE), 200);
      await r.call("cleanup: force-deactivate parent dept", tn(A, "POST", `/departments/${pid}/deactivate`, undefined, FORCE), [200, 409]);
    },
  },
  {
    key: "s1.rbac",
    module: 1,
    title: "Organization RBAC: organization:read vs organization:update",
    description: "Limited user with only organization:read can list but not write; after granting organization:update can write. Uses 1 login attempt.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const pRead = await permId(r, A, "organization", "read");
      const pUpd = await permId(r, A, "organization", "update");
      r.need(pRead && pUpd, "organization:* permissions not seeded");
      const role = await r.call("Create role {organization:read}", tn(A, "POST", "/roles", { name: `dt-org-${uid}`, permission_ids: [pRead] }), 201);
      const rid = str(role, "data.id");
      r.need(rid, "role id missing");
      const u = await mkUser(r, A, uid, "orgrbac", [rid]);
      const L = await login(r, "Login as limited user", A.slug, u.email, PASSWORD, 200);
      r.need(L.at, "no token");
      const S = { ...A, accessToken: L.at };
      await r.call("read-only: GET /departments → 200", tn(S, "GET", "/departments"), 200);
      await r.call("read-only: GET /org-chart → 200", tn(S, "GET", "/org-chart"), 200);
      await r.call("read-only: POST /departments → 403", tn(S, "POST", "/departments", { name: `forbidden ${uid}` }), 403);
      await r.call("read-only: POST /teams → 403", tn(S, "POST", "/teams", { name: "x", department_id: ZERO }), 403);
      await r.call("read-only: POST /designations → 403", tn(S, "POST", "/designations", { title: "x" }), 403);
      await r.call("read-only: POST /mappings → 403", tn(S, "POST", "/mappings", { user_id: ZERO, department_id: ZERO }), 403);
      await r.call("admin grants organization:update", tn(A, "POST", `/roles/${rid}/permissions`, { permission_ids: [pUpd] }), 200);
      const d = await r.call("now POST /departments → 201", tn(S, "POST", "/departments", { name: `allowed ${uid}` }), 201);
      if (str(d, "data.id")) await r.call("cleanup: deactivate department", tn(A, "POST", `/departments/${str(d, "data.id")}/deactivate`), 200);
      await r.call("cleanup: deactivate user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
    },
  },
  {
    key: "s1.errors",
    module: 1,
    title: "Organization validation & error handling",
    description: "Invalid input, missing fields, non-existent resources, invalid ids, no token.",
    needs: "A",
    run: async (r, { A, uid }) => {
      await r.call("No token → 401", tn(A, "GET", "/departments", undefined, undefined, null), 401);
      await r.call("Department with empty name → 400", tn(A, "POST", "/departments", { name: "" }), 400);
      await r.call("Department name > 100 chars → 400", tn(A, "POST", "/departments", { name: "x".repeat(101) }), 400);
      await r.call("Department code 1 char → 400", tn(A, "POST", "/departments", { name: `v ${uid}`, code: "A" }), 400);
      await r.call("Department parent not a uuid → 400", tn(A, "POST", "/departments", { name: `v ${uid}`, parent_department_id: "nope" }), 400);
      await r.call("GET /departments/not-a-uuid → 400", tn(A, "GET", "/departments/not-a-uuid"), 400);
      await r.call("GET non-existent department → 404", tn(A, "GET", `/departments/${ZERO}`), 404);
      await r.call("GET non-existent team → 404", tn(A, "GET", `/teams/${ZERO}`), 404);
      await r.call("GET non-existent designation → 404", tn(A, "GET", `/designations/${ZERO}`), 404);
      await r.call("GET non-existent mapping → 404", tn(A, "GET", `/mappings/${ZERO}`), 404);
      await r.call("PATCH non-existent department → 404", tn(A, "PATCH", `/departments/${ZERO}`, { name: "x" }), 404);
      await r.call("Deactivate non-existent team → 404", tn(A, "POST", `/teams/${ZERO}/deactivate`), 404);
      await r.call("List mappings without user_id → 400", tn(A, "GET", "/mappings"), 400);
      await r.call("Malformed JSON body → 400", { ...tn(A, "POST", "/departments"), rawBody: "{oops" }, 400);
      await r.call("Pagination per_page=1000 handled (clamped or 400, not 500)", tn(A, "GET", "/departments", undefined, { per_page: "1000" }), [200, 400]);
      await r.call("Pagination page=-1 handled (not 500)", tn(A, "GET", "/departments", undefined, { page: "-1" }), [200, 400]);
    },
  },
];
