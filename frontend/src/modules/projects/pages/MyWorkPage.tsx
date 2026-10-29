"use client";

import Link from "next/link";
import { getMyWork } from "../api/planning-api";
import { CalendarFeedSection } from "../components/feed/CalendarFeedSection";
import { PriorityChip, StatusPill } from "../components/ui/Chips";
import { EmptyState, ErrorNotice, SkeletonRows } from "../components/ui/States";
import { useNow } from "../hooks/use-now";
import { useResource } from "../hooks/use-resource";
import { formatDay, isOverdue } from "../lib/dates";
import { taskPath } from "../lib/paths";
import type { MyWorkGroupKey } from "../types/planning";
import type { TaskCard } from "../types/task";

const GROUPS: { key: MyWorkGroupKey; title: string }[] = [
  { key: "overdue", title: "Overdue" },
  { key: "today", title: "Today" },
  { key: "this_week", title: "This week" },
  { key: "later", title: "Later" },
  { key: "no_date", title: "No date" },
];

function WorkRow({ card, nowMs }: { card: TaskCard; nowMs: number }) {
  const overdue = isOverdue(card.due_at, card.category, nowMs);
  return (
    <li className="pm-work-row">
      <Link href={taskPath(card.ticket)} className="pm-work-link">
        <span className="pm-key">{card.ticket}</span>
        <span className="pm-task-title">{card.title}</span>
      </Link>
      <PriorityChip priority={card.priority} />
      {card.due_at && <span className={overdue ? "pm-due pm-due-overdue" : "pm-due"}>{formatDay(card.due_at)}</span>}
      {overdue && <StatusPill kind="overdue">Overdue</StatusPill>}
    </li>
  );
}

/** Landing page: tasks assigned to the caller, grouped by when they are due. */
export function MyWorkPage() {
  const res = useResource("my-work", () => getMyWork());
  const nowMs = useNow();
  const work = res.data;

  return (
    <div className="pm-page">
      <div className="pm-page-head">
        <h1>My work</h1>
        {work && <span className="pm-muted">{work.counts.total} open assigned · {work.counts.watching} watching · {work.counts.mentions} mentions</span>}
      </div>
      {res.loading && <SkeletonRows rows={6} height={40} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {work && work.counts.total === 0 && (
        <EmptyState title="Nothing assigned to you" hint="Tasks assigned to you across all projects will show up here." action={<Link href="/workspace/projects" className="pm-btn">Browse projects</Link>} />
      )}
      {work &&
        GROUPS.map(({ key, title }) => {
          const cards = work.groups[key] ?? [];
          if (cards.length === 0) return null;
          return (
            <section key={key} className="pm-section" aria-labelledby={`mw-${key}`}>
              <h2 id={`mw-${key}`}>
                {title} <span className="pm-count-badge">{work.counts[key]}</span>
              </h2>
              <ul className="pm-work-list">
                {cards.map((c) => (
                  <WorkRow key={c.id} card={c} nowMs={nowMs} />
                ))}
              </ul>
            </section>
          );
        })}
      <CalendarFeedSection />
    </div>
  );
}
