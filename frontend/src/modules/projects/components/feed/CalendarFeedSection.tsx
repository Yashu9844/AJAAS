"use client";

import { useState, type FormEvent } from "react";
import { createCalendarToken, listCalendarTokens, revokeCalendarToken } from "../../api/feed-api";
import { currentBaseUrl } from "../../api/session-store";
import { useGuardedAction } from "../../hooks/use-guarded-action";
import { useResource } from "../../hooks/use-resource";
import { formatDateTime } from "../../lib/dates";
import { feedUrl } from "../../lib/reports";
import { SecretBox } from "../ui/SecretBox";
import { EmptyState, ErrorNotice } from "../ui/States";

/** iCalendar feed of the tasks assigned to me. The feed URL holds a secret token: shown once, revocable. */
export function CalendarFeedSection() {
  const res = useResource("calendar-tokens", listCalendarTokens);
  const { run } = useGuardedAction(res.reload);
  const [shown, setShown] = useState<string | null>(null);
  const items = (res.data ?? []).filter((t) => !t.revoked_at);

  function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const label = String(new FormData(form).get("label") ?? "").trim();
    void run(async () => {
      const created = await createCalendarToken(label);
      setShown(feedUrl(currentBaseUrl(), created.feed_path) ?? "");
      form.reset();
    });
  }

  return (
    <section className="pm-section" aria-labelledby="pm-feed-h">
      <h2 id="pm-feed-h">Calendar feed</h2>
      <p className="pm-muted">Subscribe to your assigned tasks in a calendar app. Anyone with the feed address can read your tasks, so treat it like a password and revoke it if it leaks.</p>
      {shown && <SecretBox title="Calendar feed address" secret={shown} note="Copy the address now. It contains the secret token, which is shown only once." onDismiss={() => setShown(null)} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && items.length === 0 && <EmptyState title="No active calendar feeds" />}
      <ul className="pm-plain-list">
        {items.map((t) => (
          <li key={t.id} className="pm-row-between pm-card">
            <span>
              {t.label || "Calendar feed"} <span className="pm-muted">created {formatDateTime(t.created_at)}{t.last_used_at ? `, last used ${formatDateTime(t.last_used_at)}` : ", never used"}</span>
            </span>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => revokeCalendarToken(t.id))} aria-label={`Revoke feed ${t.label || t.id.slice(0, 8)}`}>
              Revoke
            </button>
          </li>
        ))}
      </ul>
      <form className="pm-inline-form" onSubmit={create} aria-label="New calendar feed">
        <label className="pm-sr-only" htmlFor="feed-label">
          Name of the feed
        </label>
        <input id="feed-label" name="label" placeholder="Name, e.g. phone" maxLength={60} />
        <button type="submit" className="pm-btn">
          Create feed
        </button>
      </form>
    </section>
  );
}
