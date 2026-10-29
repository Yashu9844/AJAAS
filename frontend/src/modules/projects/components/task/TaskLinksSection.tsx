"use client";

import { listTaskLinks } from "../../api/integrations-api";
import { useResource } from "../../hooks/use-resource";
import { safeHttpsUrl } from "../../lib/links";
import type { Task } from "../../types/task";
import { ErrorNotice } from "../ui/States";

/** Pull requests, branches and commits linked to the task by a git integration. Only https links are clickable. */
export function TaskLinksSection({ task }: { task: Task }) {
  const res = useResource(`links:${task.id}`, () => listTaskLinks(task.id));
  const items = res.data ?? [];
  if (res.data && items.length === 0) return null;
  return (
    <section className="pm-section" aria-labelledby={`ln-${task.id}`}>
      <h2 id={`ln-${task.id}`}>Linked code</h2>
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      <ul className="pm-plain-list">
        {items.map((l) => {
          const href = safeHttpsUrl(l.url);
          return (
            <li key={l.id}>
              <span className="pm-chip">{l.kind}</span> <span className="pm-chip">{l.state}</span>{" "}
              {href ? (
                <a href={href} target="_blank" rel="noopener noreferrer">
                  {l.provider} {l.external_id}
                </a>
              ) : (
                <span>
                  {l.provider} {l.external_id}
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
