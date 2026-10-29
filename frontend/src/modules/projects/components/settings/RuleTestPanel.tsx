"use client";

import { useState, type FormEvent } from "react";
import { testRule } from "../../api/automation-api";
import { describeError } from "../../api/errors";
import { getTask, isTicketKey } from "../../api/tasks-api";
import type { Rule, RuleTestResult } from "../../types/settings";

/** Dry run of a rule against one task (by ticket key): nothing changes; shows which actions would run. */
export function RuleTestPanel({ projectId, rule }: { projectId: string; rule: Rule }) {
  const [ticket, setTicket] = useState("");
  const [result, setResult] = useState<RuleTestResult | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const key = ticket.trim().toUpperCase();
    if (!isTicketKey(key)) return setError("Enter a ticket key such as ABC-12.");
    setBusy(true);
    setError("");
    setResult(null);
    try {
      const task = await getTask(key);
      setResult(await testRule(projectId, rule.id, task.id));
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="pm-deliveries" role="region" aria-label={`Test rule ${rule.name}`}>
      <form className="pm-inline-form" onSubmit={(e) => void submit(e)}>
        <label className="pm-sr-only" htmlFor={`rt-${rule.id}`}>
          Task to test against
        </label>
        <input id={`rt-${rule.id}`} value={ticket} onChange={(e) => setTicket(e.target.value)} placeholder="Task key, e.g. ABC-12" maxLength={20} />
        <button type="submit" className="pm-btn pm-btn-small" disabled={busy || !ticket.trim()}>
          {busy ? "Testing…" : "Run test"}
        </button>
      </form>
      {error && (
        <p className="pm-error" role="alert">
          {error}
        </p>
      )}
      {result && (
        <div role="status">
          <p>
            <strong>{result.matches ? "The conditions match this task." : "The conditions do not match this task."}</strong>
            {result.failed_conditions.length > 0 && <span className="pm-muted"> Failed: {result.failed_conditions.join(", ")}.</span>}
            {result.assumed_conditions.length > 0 && <span className="pm-muted"> Assumed for a dry run: {result.assumed_conditions.join(", ")}.</span>}
            {!result.creator_allowed && <span className="pm-muted"> The rule creator may no longer run these actions.</span>}
          </p>
          <ul className="pm-plain-list">
            {result.actions.map((a, i) => (
              <li key={`${a.type}-${i}`}>
                <span className={a.would_run ? "pm-chip pm-pill-ok" : "pm-chip"}>{a.would_run ? "would run" : "would not run"}</span> {a.type}
                {a.reason && <span className="pm-muted"> ({a.reason})</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
