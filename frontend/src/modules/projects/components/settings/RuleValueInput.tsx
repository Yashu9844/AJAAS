"use client";

import { usePeople } from "../../hooks/use-people";
import type { ValueKind } from "../../lib/automation-form";
import { humanize } from "../../lib/format";
import { PRIORITIES } from "../../types/common";
import type { Column, Label, Member } from "../../types/project";

const CATEGORIES = ["backlog", "todo", "in_progress", "done", "canceled"];

interface Props {
  id: string;
  label: string;
  kind: ValueKind;
  value: string;
  columns: Column[];
  labels: Label[];
  members: Member[];
  onChange: (value: string) => void;
}

/** The value control of a rule condition or action: a select for ids and enums, a field for text and days. */
export function RuleValueInput({ id, label, kind, value, columns, labels, members, onChange }: Props) {
  const { nameOf } = usePeople();
  const options: { value: string; label: string }[] =
    kind === "column"
      ? columns.map((c) => ({ value: c.id, label: c.name }))
      : kind === "category"
        ? CATEGORIES.map((c) => ({ value: c, label: humanize(c) }))
        : kind === "priority"
          ? PRIORITIES.map((p) => ({ value: p, label: humanize(p) }))
          : kind === "label"
            ? labels.map((l) => ({ value: l.id, label: l.name }))
            : kind === "member"
              ? members.map((m) => ({ value: m.user_id, label: nameOf(m.user_id) }))
              : [];
  return (
    <>
      <label className="pm-sr-only" htmlFor={id}>
        {label}
      </label>
      {kind === "text" || kind === "days" ? (
        <input id={id} value={value} onChange={(e) => onChange(e.target.value)} placeholder={kind === "days" ? "Days from now" : "Comment text"} inputMode={kind === "days" ? "numeric" : undefined} maxLength={1000} />
      ) : (
        <select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">Choose…</option>
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )}
    </>
  );
}
