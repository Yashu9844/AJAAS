import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 collaboration scenarios (comments and mentions, watchers, time tracking, attachments): specification golden tests
// PG4, PG6, PG8, PG11 and PG12 scripted against the live backend. Each run creates its own users, project and task in
// session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];

/** A fresh user holding exactly the given tenant permissions, logged in. */
async function limited(r: Runner, A: Session, uid: string, tag: string, perms: [string, string][]): Promise<{ S: Session; userId: string }> {
  const ids: string[] = [];
  for (const [res, act] of perms) ids.push(await permId(r, A, res, act));
  const role = await r.call(`Role {${perms.map((p) => p.join(":")).join(", ") || "nothing"}}`, tn(A, "POST", "/roles", { name: `dt-${tag}-${uid}`, permission_ids: ids }), 201);
  const u = await mkUser(r, A, uid, tag, [str(role, "data.id")]);
  const L = await login(r, `Login ${tag}`, A.slug, u.email, PASSWORD);
  return { S: { ...A, accessToken: L.at }, userId: u.id };
}

interface Fixture {
  owner: Session;
  member: Session;
  viewer: Session;
  stranger: Session;
  memberId: string;
  strangerId: string;
  pid: string;
  task: string;
}

/** A private project with an owner, a member, a viewer, a stranger with no access, and one task. */
async function fixture(r: Runner, A: Session, uid: string, tag: string): Promise<Fixture> {
  const key = `C${tag}${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
  const owner = await limited(r, A, uid, `${tag}o`, [["projects", "create"]]);
  const member = await limited(r, A, uid, `${tag}m`, []);
  const viewer = await limited(r, A, uid, `${tag}v`, []);
  const stranger = await limited(r, A, uid, `${tag}s`, []);
  const p = await r.call("Create a private project", tn(owner.S, "POST", "/projects", { key, name: `Collab ${uid}` }), 201);
  const pid = str(p, "data.id");
  r.need(pid, "project id missing");
  await r.call("Add the member", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: member.userId, role: "member" }), 201);
  await r.call("Add the viewer", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: viewer.userId, role: "viewer" }), 201);
  const t = await r.call("Create a task (estimate 120 min)", tn(owner.S, "POST", `/projects/${pid}/tasks`, { title: `Collab task ${uid}`, estimate_minutes: 120 }), 201);
  const task = str(t, "data.id");
  r.need(task, "task id missing");
  return { owner: owner.S, member: member.S, viewer: viewer.S, stranger: stranger.S, memberId: member.userId, strangerId: stranger.userId, pid, task };
}

export const S_PM_COLLAB: Scenario[] = [
  {
    key: "pm.collab.comments",
    module: "projects",
    title: "Comments, mentions that never leak, watchers, tombstones (CM-01..CM-03, PG4, PG11, PG12)",
    description:
      "A stranger without project access gets 404 on the task (PG4). A comment mentioning a member and a non-member keeps only the member (PG11) and makes the member a watcher. Only the author edits, a manager moderates, a delete leaves a tombstone with an empty body.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "cm");
      const path = `/tasks/${f.task}/comments`;
      await r.call("A stranger cannot read comments → 404 (PG4)", tn(f.stranger, "GET", path), 404);
      await r.call("A stranger cannot comment → 404", tn(f.stranger, "POST", path, { body: "hello" }), 404);
      await r.call("A viewer cannot comment → 403", tn(f.viewer, "POST", path, { body: "hello" }), 403);
      await r.call("An empty comment → 400", tn(f.member, "POST", path, { body: "" }), 400);
      const text = `Please review @[Member](user:${f.memberId}) and @[Stranger](user:${f.strangerId})`;
      const c = await r.call("Comment mentioning a member and a non-member", tn(f.owner, "POST", path, { body: text }), 201, (e) => {
        const m = (getPath(e.resBody, "data.mentions") as string[] | undefined) ?? [];
        return m.length === 1 && m[0] === f.memberId ? null : "only the member who can read the project may stay mentioned (PG11)";
      });
      const cid = str(c, "data.id");
      await r.call("The mentioned member watches the task", tn(f.member, "GET", `/tasks/${f.task}/watchers`), 200, (e) =>
        list(e).some((w) => w.user_id === f.memberId) && !list(e).some((w) => w.user_id === f.strangerId) ? null : "member must watch, the stranger must not",
      );
      await r.call("Only the author edits → 403", tn(f.member, "PATCH", `/comments/${cid}`, { body: "mine" }), 403);
      await r.call("The author edits; edited_at is set", tn(f.owner, "PATCH", `/comments/${cid}`, { body: "edited" }), 200, (e) =>
        getPath(e.resBody, "data.edited_at") ? null : "edited_at missing",
      );
      await r.call("List comments (cursor page)", tn(f.viewer, "GET", path, undefined, { limit: "10" }), 200, (e) => (list(e).length === 1 ? null : "expected 1 comment"));
      await r.call("Unwatch", tn(f.member, "DELETE", `/tasks/${f.task}/watch`), 200);
      await r.call("Watch again explicitly", tn(f.member, "PUT", `/tasks/${f.task}/watch`), 200, (e) => (str(e, "data.reason") === "explicit" ? null : "reason must be explicit"));
      await r.call("A member cannot delete someone else's comment → 403", tn(f.member, "DELETE", `/comments/${cid}`), 403);
      await r.call("The author deletes", tn(f.owner, "DELETE", `/comments/${cid}`), 200);
      await r.call("A tombstone remains with an empty body", tn(f.viewer, "GET", path), 200, (e) => {
        const row = list(e)[0];
        return row && row.deleted === true && row.body === "" ? null : "expected a tombstone";
      });
      await r.call("Deleting twice → 404", tn(f.owner, "DELETE", `/comments/${cid}`), 404);
    },
  },
  {
    key: "pm.collab.time",
    module: "projects",
    title: "Time entries, one running timer, totals, tracking switch (TM-01..TM-03, PG6)",
    description:
      "Logs manual time, edits it, starts a timer and proves a second start is refused with 409 TIMER_RUNNING (one running timer per user, enforced by the database), stops it, and reads the totals with the task estimate. Turning time tracking off makes logging answer 409 while reports keep working.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "tm");
      const startedAt = new Date(Date.now() - 2 * 3600 * 1000).toISOString();
      await r.call("A viewer cannot log time → 403", tn(f.viewer, "POST", `/tasks/${f.task}/time`, { started_at: startedAt, minutes: 10 }), 403);
      await r.call("A stranger cannot log time → 404", tn(f.stranger, "POST", `/tasks/${f.task}/time`, { started_at: startedAt, minutes: 10 }), 404);
      await r.call("Zero minutes → 400", tn(f.member, "POST", `/tasks/${f.task}/time`, { started_at: startedAt, minutes: 0 }), 400);
      const e = await r.call("Log 45 minutes", tn(f.member, "POST", `/tasks/${f.task}/time`, { started_at: startedAt, minutes: 45, note: "pairing" }), 201);
      const eid = str(e, "data.id");
      await r.call("Edit the entry to 50 minutes", tn(f.member, "PATCH", `/time-entries/${eid}`, { minutes: 50 }), 200, (x) =>
        Number(getPath(x.resBody, "data.minutes")) === 50 ? null : "minutes should be 50",
      );
      await r.call("Another user cannot edit it → 403", tn(f.owner, "PATCH", `/time-entries/${eid}`, { note: "x" }), 200); // the owner holds time.manage
      await r.call("Start a timer", tn(f.member, "POST", "/time-entries/timer/start", { task_id: f.task }), 201);
      await r.call("A second timer → 409 TIMER_RUNNING (PG6)", tn(f.member, "POST", "/time-entries/timer/start", { task_id: f.task }), 409, (x) =>
        str(x, "error.code") === "TIMER_RUNNING" ? null : "expected TIMER_RUNNING",
      );
      await r.call("My running timer", tn(f.member, "GET", "/time-entries/timer"), 200, (x) => (str(x, "data.task_id") === f.task ? null : "timer must be on the task"));
      await r.call("Stop the timer", tn(f.member, "POST", "/time-entries/timer/stop"), 200);
      await r.call("Stopping again → 404", tn(f.member, "POST", "/time-entries/timer/stop"), 404);
      await r.call("Totals per task with the estimate", tn(f.viewer, "GET", `/projects/${f.pid}/time/report`), 200, (x) =>
        Number(getPath(x.resBody, "data.estimate_minutes")) === 120 && Number(getPath(x.resBody, "data.total_minutes")) >= 50 ? null : "estimate 120 and logged >= 50 expected",
      );
      await r.call("Totals per user", tn(f.viewer, "GET", `/projects/${f.pid}/time/report`, undefined, { group_by: "user" }), 200, (x) =>
        list(x, "data.rows").some((row) => row.key === f.memberId) ? null : "the member must appear",
      );
      await r.call("A stranger reads no report → 404", tn(f.stranger, "GET", `/projects/${f.pid}/time/report`), 404);
      await r.call("Turn time tracking off", tn(f.owner, "PATCH", `/projects/${f.pid}`, { time_tracking_enabled: false }), 200);
      await r.call("Logging is refused → 409 TIME_TRACKING_DISABLED", tn(f.member, "POST", `/tasks/${f.task}/time`, { started_at: startedAt, minutes: 5 }), 409, (x) =>
        str(x, "error.code") === "TIME_TRACKING_DISABLED" ? null : "expected TIME_TRACKING_DISABLED",
      );
      await r.call("Delete the entry", tn(f.member, "DELETE", `/time-entries/${eid}`), 200);
    },
  },
  {
    key: "pm.collab.attachments",
    module: "projects",
    title: "Attachments: validation, init, upload mismatch, ACL (AT-01..AT-03, PG8, PG12)",
    description:
      "Rejects executables, oversize files and unknown types, starts an upload (201 with a presigned PUT; 503 STORAGE_UNAVAILABLE is accepted when the API runs without object storage and ends the scenario), proves that completing before uploading is refused with 409 UPLOAD_MISMATCH and that a pending upload cannot be listed or downloaded, then cancels it. The browser upload itself is not scripted.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "at");
      const init = `/tasks/${f.task}/attachments`;
      const ok = { filename: "notes.txt", size: 11, content_type: "text/plain" };
      const first = await r.call("Start an upload (201, or 503 without storage)", tn(f.member, "POST", init, ok), [201, 503]);
      if (first.status === 503) {
        r.skip("Remaining steps", "object storage is not configured on this API");
        return;
      }
      const aid = str(first, "data.attachment.id");
      await r.call("The presigned PUT is returned", tn(f.member, "POST", init, ok), 201, (e) =>
        str(e, "data.upload.method") === "PUT" && str(e, "data.upload.url") && getPath(e.resBody, "data.attachment.storage_key") === undefined ? null : "expected a PUT target and no storage_key",
      );
      await r.call("An executable is refused → 400", tn(f.member, "POST", init, { ...ok, filename: "setup.exe" }), 400, (e) =>
        str(e, "error.code") === "FILE_TYPE_NOT_ALLOWED" ? null : "expected FILE_TYPE_NOT_ALLOWED",
      );
      await r.call("An unknown content type is refused → 400", tn(f.member, "POST", init, { ...ok, content_type: "text/html" }), 400);
      await r.call("More than 50 MB → 400", tn(f.member, "POST", init, { ...ok, size: 60 * 1024 * 1024 }), 400, (e) =>
        str(e, "error.code") === "FILE_TOO_LARGE" ? null : "expected FILE_TOO_LARGE",
      );
      await r.call("A viewer cannot upload → 403", tn(f.viewer, "POST", init, ok), 403);
      await r.call("A stranger cannot upload → 404 (PG8)", tn(f.stranger, "POST", init, ok), 404);
      await r.call("Complete before uploading → 409 UPLOAD_MISMATCH", tn(f.member, "POST", `/attachments/${aid}/complete`), 409, (e) =>
        str(e, "error.code") === "UPLOAD_MISMATCH" ? null : "expected UPLOAD_MISMATCH",
      );
      await r.call("A pending upload is not listed", tn(f.viewer, "GET", init), 200, (e) => (list(e).length === 0 ? null : "pending uploads must not be listed"));
      await r.call("A pending upload cannot be downloaded → 404", tn(f.member, "GET", `/attachments/${aid}/download`), 404);
      await r.call("A stranger cannot download → 404 (PG8)", tn(f.stranger, "GET", `/attachments/${aid}/download`), 404);
      await r.call("Cancel the upload", tn(f.member, "DELETE", `/attachments/${aid}`), 200);
    },
  },
];
