import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { gl, login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 webhook and git-integration scenarios (docs/TEST_CONSOLE.md), scripted against the live backend. No real receiver is
// reachable from the test environment, so deliveries are proven by their recorded outcome: the ping to an unresolvable
// host is stored as failed with a short fixed error, and every private or unsafe URL is refused before anything is sent.
// Each run creates its own users and project in session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];

async function projectOwner(r: Runner, A: Session, uid: string): Promise<Session> {
  const create = await permId(r, A, "projects", "create");
  const role = await r.call("Role {projects:create}", tn(A, "POST", "/roles", { name: `dt-pmw-${uid}`, permission_ids: [create] }), 201);
  const u = await mkUser(r, A, uid, "pmw", [str(role, "data.id")]);
  const L = await login(r, "Login owner", A.slug, u.email, PASSWORD);
  return { ...A, accessToken: L.at };
}

const RECEIVER = "https://receiver.invalid/hook"; // .invalid never resolves: the delivery attempt must fail safely

export const S_PM_WEBHOOKS: Scenario[] = [
  {
    key: "pm.webhooks.flow",
    module: "projects",
    title: "Webhooks: validation, SSRF refusal, secret shown once, rotate, test ping, deliveries (phase 4)",
    description:
      "Creates a webhook and shows the signing secret exactly once, refuses plain http, credentials, odd ports and every private, loopback, link-local (cloud metadata) and numeric-looking host, rejects unknown formats and event names, rotates the secret, sends a test ping to an unresolvable host and shows it recorded as failed (and never retried), lists and redelivers deliveries, and checks outsiders see nothing.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const O = await projectOwner(r, A, uid);
      const outsider = await mkUser(r, A, uid, "pmwx");
      const OL = await login(r, "Login outsider", A.slug, outsider.email, PASSWORD);
      const key = `W${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const p = await r.call("Create a project", tn(O, "POST", "/projects", { key, name: `Hooks ${uid}` }), 201);
      const pid = str(p, "data.id");
      const base = `/projects/${pid}/webhooks`;
      for (const [label, url] of [
        ["plain http", "http://receiver.example.com/hook"],
        ["credentials in the URL", "https://user:pw@receiver.example.com/hook"],
        ["loopback", "https://127.0.0.1/hook"],
        ["IPv4-mapped loopback", "https://[::ffff:127.0.0.1]/hook"],
        ["hex loopback", "https://0x7f.1/hook"],
        ["decimal loopback", "https://2130706433/hook"],
        ["cloud metadata address", "https://169.254.169.254/latest/meta-data/"],
        ["private network", "https://10.1.2.3/hook"],
        ["localhost", "https://localhost/hook"],
        ["port 8080", "https://receiver.example.com:8080/hook"],
      ] as const) {
        await r.call(`Refused: ${label} → 400`, tn(O, "POST", base, { name: "bad", url }), 400);
      }
      await r.call("Unknown format → 400", tn(O, "POST", base, { name: "bad", url: RECEIVER, format: "teams" }), 400);
      await r.call("Unknown event name → 400", tn(O, "POST", base, { name: "bad", url: RECEIVER, event_types: ["Task.Created"] }), 400);
      const c = await r.call("Create a webhook (secret shown once)", tn(O, "POST", base, { name: "CI", url: RECEIVER, format: "slack", event_types: ["task.created", "task.moved"] }), 201, (e) =>
        String(getPath(e.resBody, "data.secret")).startsWith("whsec_") ? null : "expected a whsec_ secret",
      );
      const wid = str(c, "data.id");
      await r.call("The same name twice → 409", tn(O, "POST", base, { name: "ci", url: RECEIVER }), 409);
      await r.call("The list never shows the secret", tn(O, "GET", base), 200, (e) =>
        list(e).length === 1 && !JSON.stringify(e.resBody).includes("whsec_") ? null : "secret leaked or wrong count",
      );
      await r.call("An outsider sees nothing → 404", tn({ ...A, accessToken: OL.at }, "GET", base), 404);
      await r.call("Edit the webhook", tn(O, "PATCH", `${base}/${wid}`, { name: "CI bot", format: "discord" }), 200);
      await r.call("Editing to a private URL → 400", tn(O, "PATCH", `${base}/${wid}`, { url: "https://192.168.1.10/hook" }), 400);
      const rot = await r.call("Rotate the secret (new one shown once)", tn(O, "POST", `${base}/${wid}/rotate-secret`), 200);
      if (str(rot, "data.secret") === str(c, "data.secret")) throw new Error("rotation must change the secret");
      const ping = await r.call("Test ping: recorded as failed (no receiver is reachable)", tn(O, "POST", `${base}/${wid}/test`), 200, (e) =>
        getPath(e.resBody, "data.status") === "failed" && getPath(e.resBody, "data.next_attempt_at") == null ? null : `expected a failed, unretried ping, got ${JSON.stringify(getPath(e.resBody, "data"))}`,
      );
      await r.call("Deliveries list the ping with a short error and no URL", tn(O, "GET", `${base}/${wid}/deliveries`), 200, (e) =>
        list(e).length === 1 && !JSON.stringify(e.resBody).includes("receiver.invalid") ? null : "expected one delivery without the URL",
      );
      await r.call("Filter deliveries by status", tn(O, "GET", `${base}/${wid}/deliveries`, undefined, { status: "delivered" }), 200, (e) => (list(e).length === 0 ? null : "no delivered ones"));
      await r.call("Unknown status filter → 400", tn(O, "GET", `${base}/${wid}/deliveries`, undefined, { status: "bogus" }), 400);
      await r.call("Redeliver the ping", tn(O, "POST", `${base}/${wid}/deliveries/${str(ping, "data.id")}/redeliver`), 200);
      await r.call("Redelivering twice while queued → 409", tn(O, "POST", `${base}/${wid}/deliveries/${str(ping, "data.id")}/redeliver`), 409);
      await r.call("Disable the webhook", tn(O, "PATCH", `${base}/${wid}`, { active: false }), 200);
      await r.call("Delete the webhook", tn(O, "DELETE", `${base}/${wid}`), 200);
      await r.call("Deleted: 404", tn(O, "DELETE", `${base}/${wid}`), 404);
    },
  },
  {
    key: "pm.webhooks.integrations",
    module: "projects",
    title: "Git integrations: secret shown once, signed provider calls only, task links (phase 4)",
    description:
      "Adds a GitHub and a Gitea integration (the secret and the incoming path are shown once), proves an unsigned or wrongly signed provider call answers the same 404 as an unknown integration id (no oracle), rotates and deletes integrations, and lists a task's links (empty until a signed provider call links a pull request).",
    needs: "A",
    run: async (r, { A, uid }) => {
      const O = await projectOwner(r, A, uid);
      const key = `G${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const p = await r.call("Create a project", tn(O, "POST", "/projects", { key, name: `Git ${uid}` }), 201);
      const pid = str(p, "data.id");
      const t = await r.call("Create a task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "Linked work" }), 201);
      const tid = str(t, "data.id");
      const base = `/projects/${pid}/integrations`;
      await r.call("Unknown provider → 400", tn(O, "POST", base, { provider: "svn", name: "repo" }), 400);
      const gh = await r.call("Add a GitHub integration (secret + incoming path shown once)", tn(O, "POST", base, { provider: "github", name: "repo" }), 201, (e) =>
        String(getPath(e.resBody, "data.path")).startsWith("/integrations/github/") && String(getPath(e.resBody, "data.secret")).startsWith("whsec_") ? null : "expected path and secret",
      );
      await r.call("Add a Gitea integration", tn(O, "POST", base, { provider: "gitea", name: "mirror" }), 201);
      await r.call("The same name twice → 409", tn(O, "POST", base, { provider: "gitea", name: "REPO" }), 409);
      await r.call("The list never shows secrets", tn(O, "GET", base), 200, (e) => (list(e).length === 2 && !JSON.stringify(e.resBody).includes("whsec_") ? null : "secret leaked or wrong count"));
      const id = str(gh, "data.id");
      const unsigned = { action: "opened", pull_request: { number: 7, html_url: "https://github.com/o/r/pull/7", title: `${key}-1 add`, head: { ref: "x" } } };
      const bad = await r.call("An unsigned provider call → 404", gl("POST", `/integrations/github/${id}`, unsigned, null), 404);
      const unknown = await r.call("An unknown integration id → 404, same answer", gl("POST", `/integrations/github/00000000-0000-4000-8000-000000000001`, unsigned, null), 404);
      if (JSON.stringify(bad.resBody) !== JSON.stringify(unknown.resBody)) throw new Error("a bad signature and an unknown id must be indistinguishable");
      await r.call("The task has no links yet", tn(O, "GET", `/tasks/${tid}/links`), 200, (e) => (list(e).length === 0 ? null : "expected no links"));
      await r.call("Rotate the secret", tn(O, "POST", `${base}/${id}/rotate-secret`), 200);
      await r.call("Delete the integration", tn(O, "DELETE", `${base}/${id}`), 200);
      await r.call("Deleted: 404", tn(O, "DELETE", `${base}/${id}`), 404);
    },
  },
];
