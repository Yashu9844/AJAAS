// Checks for bulk actions, Gantt maths, saved views and the automation forms (imported by run.mjs).
import assert from "node:assert/strict";
import { check, load } from "./harness.mjs";

const bulk = await load("lib/bulk.mjs");
const gantt = await load("lib/gantt.mjs");
const views = await load("lib/saved-views.mjs");
const auto = await load("lib/automation-form.mjs");
const draft = await load("lib/rule-draft.mjs");
const reports = await load("lib/reports.mjs");

await check("bulk: selection toggling, page select, pruning and the 100 cap", () => {
  const s1 = bulk.toggleSelection(new Set(["a"]), "b");
  assert.deepEqual([...s1].sort(), ["a", "b"]);
  assert.deepEqual([...bulk.toggleSelection(s1, "a")], ["b"]);
  assert.deepEqual([...bulk.togglePage(new Set(), ["a", "b"])], ["a", "b"]);
  assert.deepEqual([...bulk.togglePage(new Set(["a", "b", "z"]), ["a", "b"])], ["z"]);
  assert.deepEqual([...bulk.pruneSelection(new Set(["a", "z"]), ["a", "b"])], ["a"]);
  const many = Array.from({ length: 150 }, (_, i) => `t${i}`);
  assert.equal(bulk.togglePage(new Set(), many).size, 100);
});
await check("bulk: per-task results list failures first with readable messages", () => {
  const out = { results: [
    { task_id: "t1", ok: true, version: 3 },
    { task_id: "t2", ok: false, error: { code: "NOT_FOUND", message: "task not found" } },
    { task_id: "t3", ok: false, error: { code: "WIP_LIMIT" } },
  ] };
  const sum = bulk.summarizeBulk(out, (id) => id.toUpperCase());
  assert.equal(sum.succeeded, 1);
  assert.equal(sum.failed, 2);
  assert.deepEqual(sum.lines.map((l) => l.label), ["T2", "T3", "T1"]);
  assert.equal(sum.lines[0].message, "task not found");
  assert.equal(sum.lines[1].message, "WIP_LIMIT");
});
await check("gantt: scale, bar and marker positions, today line, window moves", () => {
  const scale = gantt.makeScale("2026-10-01", "2026-10-31", 10);
  assert.equal(scale.days, 31);
  assert.equal(gantt.scaleWidth(scale), 310);
  assert.equal(gantt.dayOffset(scale, "2026-10-11"), 100);
  assert.equal(gantt.dayOffset(scale, "2026-09-30"), -10);
  const bar = gantt.barBox(scale, "2026-10-05", "2026-10-09T12:00:00");
  assert.deepEqual(bar, { kind: "bar", left: 40, width: 50 });
  assert.equal(gantt.barBox(scale, null, "2026-10-09T12:00:00").kind, "marker");
  assert.equal(gantt.barBox(scale, "2026-10-09", null).kind, "marker");
  assert.equal(gantt.barBox(scale, null, null), null);
  assert.equal(gantt.todayOffset(scale, "2026-10-02"), 15);
  assert.equal(gantt.todayOffset(scale, "2026-11-20"), null);
  assert.deepEqual(gantt.windowAround("2026-10-15", 14, 90), { from: "2026-10-01", to: "2026-12-29" });
  assert.deepEqual(gantt.shiftWindow({ from: "2026-10-01", to: "2026-10-31" }, 30), { from: "2026-10-31", to: "2026-11-30" });
  assert.equal(gantt.weekTicks(scale)[0].key, "2026-10-01");
});
await check("gantt: arrows connect blocking end to blocked start; pages merge without duplicates", () => {
  const boxes = new Map([["a", { box: { kind: "bar", left: 10, width: 30 }, row: 0 }], ["b", { box: { kind: "bar", left: 60, width: 20 }, row: 2 }]]);
  assert.deepEqual(gantt.arrowLine("a", "b", boxes, 32), { fromId: "a", toId: "b", x1: 40, y1: 16, x2: 60, y2: 80 });
  assert.equal(gantt.arrowLine("a", "zz", boxes, 32), null);
  const merged = gantt.mergeGanttPage(
    { tasks: [{ id: "a" }], milestones: [{ id: "m" }], dependencies: [{ source_task_id: "a", target_task_id: "b", type: "blocks" }], next_cursor: "c1" },
    { tasks: [{ id: "a" }, { id: "b" }], milestones: [], dependencies: [{ source_task_id: "a", target_task_id: "b", type: "blocks" }], next_cursor: null },
  );
  assert.deepEqual(merged.tasks.map((t) => t.id), ["a", "b"]);
  assert.equal(merged.milestones.length, 1);
  assert.equal(merged.dependencies.length, 1);
  assert.equal(merged.next_cursor, null);
});
await check("saved views: filters map to the server allow-list and back", () => {
  const f = { q: " fix ", column_id: "c1", priority: "high", assignee: "u1", overdue: true, blocked: true, sort: "due", dir: "desc" };
  const v = views.filtersToView(f);
  assert.deepEqual(v.filters, { q: "fix", column_ids: ["c1"], priorities: ["high"], assignee_ids: ["u1"], overdue: true });
  assert.ok(!("blocked" in v.filters));
  assert.deepEqual(v.sort, [{ field: "due", dir: "desc" }]);
  const back = views.viewToFilters(v);
  assert.deepEqual({ ...back, blocked: true, q: "fix" }, { ...f, q: "fix" });
  assert.deepEqual(views.filtersToView(views.EMPTY_FILTERS).filters, {});
  assert.deepEqual(views.viewToFilters({ filters: {}, sort: [] }), views.EMPTY_FILTERS);
  assert.ok(views.viewHasUnshownParts({ filters: { priorities: ["low", "high"] } }));
});
await check("automation forms: rule, recurrence, webhook and id validation", () => {
  assert.equal(draft.buildRuleFromDraft({ name: "", trigger: "task.moved", conditions: [], actions: [{ type: "set_priority", value: "low" }] }, true).ok, false);
  const rule = draft.buildRuleFromDraft({ name: "R", trigger: "task.moved", conditions: [{ field: "to_column_id", value: "c1" }], actions: [{ type: "set_priority", value: "low" }] }, true);
  assert.deepEqual(rule.input, { name: "R", trigger: "task.moved", conditions: [{ field: "to_column_id", value: "c1" }], actions: [{ type: "set_priority", priority: "low" }], active: true });
  assert.deepEqual(auto.buildAction("set_due_in_days", "3"), { type: "set_due_in_days", days: 3 });
  assert.equal(auto.buildAction("set_due_in_days", "-1"), null);
  assert.equal(auto.buildAction("nope", "x"), null);
  assert.ok(auto.conditionChoices("task.moved").some((c) => c.value === "to_column_id"));
  assert.ok(!auto.conditionChoices("task.created").some((c) => c.value === "to_column_id"));
  assert.equal(auto.buildRecurrenceInput({ templateId: "", frequency: "daily", interval: "1", weekday: "1", dayOfMonth: "1" }).ok, false);
  assert.deepEqual(auto.buildRecurrenceInput({ templateId: "t", frequency: "weekly", interval: "2", weekday: "3", dayOfMonth: "1" }).input, { template_task_id: "t", frequency: "weekly", interval: 2, weekday: 3 });
  assert.equal(auto.buildRecurrenceInput({ templateId: "t", frequency: "monthly", interval: "1", weekday: "1", dayOfMonth: "40" }).ok, false);
  assert.deepEqual(auto.parseEventTypes("").input, ["*"]);
  assert.deepEqual(auto.parseEventTypes("task.created, task.moved task.created").input, ["task.created", "task.moved"]);
  assert.equal(auto.parseEventTypes("Task Created").ok, false);
  assert.equal(auto.validateWebhookUrl("https://example.com/h"), null);
  assert.equal(auto.validateWebhookUrl("http://example.com/h"), "Only https addresses are accepted.");
  assert.match(auto.validateWebhookUrl("https://example.com:8080/h"), /ports/);
  assert.match(auto.validateWebhookUrl("https://u:p@example.com/h"), /credentials/);
  assert.ok(auto.isUuid("11111111-1111-1111-1111-111111111111"));
  assert.ok(!auto.isUuid("team-1"));
  assert.deepEqual(auto.parseOptions("a, b" + String.fromCharCode(10) + "b,, c"), ["a", "b", "c"]);
});


const roles = await load("lib/role-picker.mjs");
const links = await load("lib/links.mjs");
const paths2 = await load("lib/paths.mjs");

await check("role picker: built-in and custom roles encode and decode", () => {
  const id = "22af515b-fa4a-49f6-8987-ca8c73acf349";
  const opts = roles.roleOptions([{ id, name: "Reviewer" }]);
  assert.deepEqual(opts.map((o) => o.value), ["owner", "manager", "member", "viewer", `custom:${id}`]);
  assert.equal(opts[4].label, "Reviewer (custom)");
  assert.deepEqual(roles.decodeRole("manager"), { role: "manager" });
  assert.deepEqual(roles.decodeRole(`custom:${id}`), { custom_role_id: id });
  assert.equal(roles.decodeRole("custom:../x"), null);
  assert.equal(roles.decodeRole("root"), null);
  assert.equal(roles.currentRoleValue({ role_key: "viewer", custom_role_id: null }), "viewer");
  assert.equal(roles.currentRoleValue({ role_key: null, custom_role_id: id }), `custom:${id}`);
  assert.deepEqual(roles.togglePermission(["b"], "a", true), ["a", "b"]);
  assert.deepEqual(roles.togglePermission(["a", "b"], "a", false), ["b"]);
});
await check("links: only plain https URLs become links", () => {
  assert.equal(links.safeHttpsUrl("https://github.com/o/r/pull/1"), "https://github.com/o/r/pull/1");
  for (const bad of ["http://github.com/x", "javascript:alert(1)", "https://u:p@github.com/x", "data:text/html,x", "not a url"]) assert.equal(links.safeHttpsUrl(bad), null, bad);
});
await check("paths: integration payload URL and the public page address", () => {
  const id = "cfd715b8-44cd-4847-9462-bd13ab10a3e9";
  assert.equal(paths2.integrationPayloadUrl("http://acme.localhost:8091", "github", id), `http://acme.localhost:8091/api/v1/integrations/github/${id}`);
  assert.equal(paths2.integrationPayloadUrl("http://acme.localhost:8091", "svn", id), null);
  assert.equal(paths2.integrationPayloadUrl("http://acme.localhost:8091", "github", "../x"), null);
  assert.equal(paths2.integrationPayloadUrl(null, "github", id), null);
  const token = "bnWwVYxqnz7Um5uFrR_onRijqABdvxRgm5d7H49VSXk";
  assert.equal(paths2.publicPageUrl("http://localhost:3000", token, "uitest", "localhost", "8091"), `http://localhost:3000/public/projects/${token}?t=uitest&h=localhost&p=8091`);
  assert.equal(paths2.publicPageUrl("http://localhost:3000", "short", "uitest", "localhost", "8091"), null);
  assert.equal(paths2.publicPageUrl("http://localhost:3000", token, "Bad Slug", "localhost", "8091"), null);
  const get = (m) => ({ get: (k) => m[k] ?? null });
  assert.deepEqual(paths2.parsePublicTarget(get({ t: "uitest", h: "localhost", p: "8091" })), { slug: "uitest", host: "localhost", port: "8091" });
  assert.equal(paths2.parsePublicTarget(get({ t: "uitest" })), null);
  assert.equal(paths2.parsePublicTarget(get({ t: "x", h: "evil.com/path", p: "" })), null);
  assert.equal(paths2.parsePublicTarget(get({ t: "x", h: "h.com", p: "99999999" })), null);
});
await check("rule editor: multi-condition / multi-action rules round-trip without losing parts; recurrences fill the edit form", () => {
  const rule = { name: "R", trigger: "task.moved", conditions: [{ field: "to_column_id", value: "c1" }, { field: "priority", value: "high" }], actions: [{ type: "set_priority", priority: "low" }, { type: "add_comment", text: "moved" }, { type: "future_action", foo: 1 }] };
  const d = draft.ruleToDraft(rule);
  assert.equal(d.conditions.length, 2);
  assert.deepEqual(d.actions.map((a) => a.type), ["set_priority", "add_comment", "future_action"]);
  assert.deepEqual(d.actions[2].raw, { type: "future_action", foo: 1 });
  const back = draft.buildRuleFromDraft(d, false);
  assert.deepEqual(back.input.conditions, rule.conditions);
  assert.deepEqual(back.input.actions, rule.actions);
  assert.equal(back.input.active, false);
  assert.equal(draft.buildRuleFromDraft({ ...d, conditions: [{ field: "priority", value: "" }] }, true).ok, false);
  assert.equal(draft.buildRuleFromDraft({ ...d, actions: [] }, true).ok, false);
  assert.equal(draft.buildRuleFromDraft({ ...d, actions: [{ type: "set_due_in_days", value: "x" }] }, true).ok, false);
  const edited = draft.ruleToDraft(rule);
  edited.conditions.push({ field: "label_id", value: "l1" });
  edited.actions.push({ type: "assign_user", value: "u1" });
  assert.equal(draft.buildRuleFromDraft(edited, true).input.conditions.length, 3);
  assert.deepEqual(draft.buildRuleFromDraft(edited, true).input.actions[3], { type: "assign_user", user_id: "u1" });
  const many = { ...d, conditions: Array.from({ length: 11 }, () => ({ field: "priority", value: "low" })) };
  assert.equal(draft.buildRuleFromDraft(many, true).ok, false);
  assert.deepEqual(draft.conditionsForTrigger("task.created", [{ field: "to_column_id", value: "c" }, { field: "priority", value: "low" }, { field: "custom_x", value: "1" }]).map((c) => c.field), ["priority", "custom_x"]);
  assert.equal(draft.conditionKind("task.moved", "to_column_id"), "column");
  assert.equal(draft.conditionKind("task.moved", "unknown"), "text");
  assert.equal(draft.actionKind("add_label"), "label");
  assert.equal(auto.actionValueOf({ type: "set_due_in_days", days: 0 }), "0");
  assert.equal(auto.actionValueOf({ type: "add_comment", text: "hi" }), "hi");
  const rec = { template_task_id: "t", frequency: "weekly", interval: 2, weekday: 3, day_of_month: null };
  assert.deepEqual(auto.recurrenceToForm(rec), { templateId: "t", frequency: "weekly", interval: "2", weekday: "3", dayOfMonth: "1" });
  assert.deepEqual(auto.buildRecurrenceEdit(auto.recurrenceToForm(rec)).input, { frequency: "weekly", interval: 2, weekday: 3 });
  assert.equal(auto.buildRecurrenceEdit({ ...auto.recurrenceToForm(rec), interval: "99" }).ok, false);
});

await check("reports: logged vs estimate, period validation, sprint summary and feed address", () => {
  assert.deepEqual(reports.loggedVsEstimate(90, 60), { percent: 150, over: true });
  assert.deepEqual(reports.loggedVsEstimate(30, 60), { percent: 50, over: false });
  assert.deepEqual(reports.loggedVsEstimate(30, 0), { percent: null, over: false });
  assert.ok(reports.isValidReportRange("2026-10-01", "2026-10-31"));
  assert.ok(!reports.isValidReportRange("2026-10-31", "2026-10-01"));
  assert.ok(!reports.isValidReportRange("2026-10-01", "2028-10-01"));
  assert.ok(!reports.isValidReportRange("", "2026-10-01"));
  const s = reports.summarizeSprintReport({ committed_count: 8, completed_count: 6, committed_estimate_minutes: 480, completed_estimate_minutes: 360, moved_unfinished_count: 2 });
  assert.equal(s.completionPercent, 75);
  assert.equal(reports.summarizeSprintReport({ committed_count: 0, completed_count: 0, committed_estimate_minutes: 0, completed_estimate_minutes: 0, moved_unfinished_count: 0 }).completionPercent, null);
  const tok = "1k7DwnGU1lPR626dFq3x9h4zfriJKylzagGFUNO4FTc";
  assert.equal(reports.feedUrl("http://acme.localhost:8091", `/api/v1/calendar/feed.ics?token=${tok}`), `http://acme.localhost:8091/api/v1/calendar/feed.ics?token=${tok}`);
  assert.equal(reports.feedUrl("http://acme.localhost:8091", "/api/v1/other?token=abc"), null);
  assert.equal(reports.feedUrl(null, `/api/v1/calendar/feed.ics?token=${tok}`), null);
});
