"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { describeError } from "../../api/errors";
import { addRelation, getTask, isTicketKey, listRelations, removeRelation } from "../../api/tasks-api";
import { useResource } from "../../hooks/use-resource";
import { useToast } from "../../hooks/use-toast";
import { taskPath } from "../../lib/paths";
import type { RelationType, Task } from "../../types/task";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

const TYPES: { value: RelationType; label: string }[] = [
  { value: "blocks", label: "blocks" },
  { value: "related", label: "is related to" },
  { value: "duplicates", label: "duplicates" },
];

function describeRelation(type: RelationType, direction: "outgoing" | "incoming"): string {
  if (type === "blocks") return direction === "outgoing" ? "Blocks" : "Blocked by";
  if (type === "duplicates") return direction === "outgoing" ? "Duplicates" : "Duplicated by";
  return "Related to";
}

export function RelationsSection({ task, canWrite }: { task: Task; canWrite: boolean }) {
  const toast = useToast();
  const [target, setTarget] = useState("");
  const [type, setType] = useState<RelationType>("blocks");
  const res = useResource(`relations:${task.id}`, () => listRelations(task.id));

  async function add(e: FormEvent) {
    e.preventDefault();
    const key = target.trim().toUpperCase();
    if (!isTicketKey(key)) return toast.show("Enter a ticket key such as ABC-12.", "error");
    try {
      const other = await getTask(key);
      await addRelation(task.id, other.id, type);
      setTarget("");
      res.reload();
    } catch (err) {
      toast.show(describeError(err), "error");
    }
  }

  async function remove(relationId: string) {
    try {
      await removeRelation(task.id, relationId);
      res.reload();
    } catch (err) {
      toast.show(describeError(err), "error");
    }
  }

  return (
    <section className="pm-section" aria-labelledby={`rel-${task.id}`}>
      <h3 id={`rel-${task.id}`}>Relations</h3>
      {res.loading && <SkeletonRows rows={2} height={28} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && res.data.length === 0 && <EmptyState title="No relations" />}
      <ul className="pm-plain-list">
        {(res.data ?? []).map((r) => (
          <li key={r.id} className="pm-row-between">
            <span>
              {describeRelation(r.type, r.direction)} <Link href={taskPath(r.other_key)}>{r.other_key}</Link> {r.other_title}
              {r.other_done && <span className="pm-muted"> (done)</span>}
            </span>
            {canWrite && (
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void remove(r.id)} aria-label={`Remove relation to ${r.other_key}`}>
                Remove
              </button>
            )}
          </li>
        ))}
      </ul>
      {canWrite && (
        <form className="pm-inline-form" onSubmit={add}>
          <label className="pm-sr-only" htmlFor={`rel-type-${task.id}`}>
            Relation type
          </label>
          <select id={`rel-type-${task.id}`} value={type} onChange={(e) => setType(e.target.value as RelationType)}>
            {TYPES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
          <label className="pm-sr-only" htmlFor={`rel-target-${task.id}`}>
            Ticket key
          </label>
          <input id={`rel-target-${task.id}`} value={target} onChange={(e) => setTarget(e.target.value)} placeholder="ABC-12" maxLength={20} />
          <button type="submit" className="pm-btn" disabled={!target.trim()}>
            Add
          </button>
        </form>
      )}
    </section>
  );
}
