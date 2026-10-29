import { humanize, initials } from "../../lib/format";
import type { Priority } from "../../types/common";
import type { Label } from "../../types/project";

export function PriorityChip({ priority }: { priority: Priority }) {
  if (priority === "none") return null;
  return <span className={`pm-chip pm-prio-${priority}`}>{humanize(priority)}</span>;
}

export function RoleBadge({ role }: { role: string }) {
  return <span className="pm-chip pm-role">{role ? humanize(role) : "Admin access"}</span>;
}

export function LabelChip({ label }: { label: Label }) {
  const safeColor = /^#[0-9a-fA-F]{3,8}$/.test(label.color) ? label.color : "#64748b";
  return (
    <span className="pm-chip pm-label" style={{ borderColor: safeColor }}>
      <span className="pm-dot" style={{ background: safeColor }} aria-hidden="true" />
      {label.name}
    </span>
  );
}

export function Avatar({ name }: { name: string }) {
  return (
    <span className="pm-avatar" title={name} aria-label={name}>
      {initials(name)}
    </span>
  );
}

export function StatusPill({ kind, children }: { kind: "overdue" | "blocked" | "warning" | "ok"; children: string }) {
  return <span className={`pm-chip pm-pill-${kind}`}>{children}</span>;
}
