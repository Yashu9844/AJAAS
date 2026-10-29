"use client";

import { useState } from "react";
import { bulkUpdate } from "../../api/views-api";
import { describeError } from "../../api/errors";
import { summarizeBulk, type BulkSummary } from "../../lib/bulk";
import { humanize } from "../../lib/format";
import { PRIORITIES } from "../../types/common";
import type { Column } from "../../types/project";
import type { Task } from "../../types/task";
import type { BulkChange } from "../../types/views";
import { Dialog } from "../ui/Dialog";

interface Props {
  projectId: string;
  selectedIds: string[];
  tasks: Task[];
  columns: Column[];
  onClear: () => void;
  onDone: () => void;
}

/** Bar for the selected tasks: move to a column, set priority, archive (after a confirmation). Shows each task outcome. */
export function BulkBar({ projectId, selectedIds, tasks, columns, onClear, onDone }: Props) {
  const [busy, setBusy] = useState(false);
  const [confirmArchive, setConfirmArchive] = useState(false);
  const [summary, setSummary] = useState<BulkSummary | null>(null);
  const [error, setError] = useState("");

  async function apply(change: BulkChange) {
    setBusy(true);
    setError("");
    try {
      const out = await bulkUpdate(projectId, selectedIds, change);
      const keyOf = (id: string) => tasks.find((t) => t.id === id)?.key ?? id.slice(0, 8);
      setSummary(summarizeBulk(out, keyOf));
      onDone();
    } catch (e) {
      setError(describeError(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="pm-bulk" role="region" aria-label="Bulk actions">
      {selectedIds.length > 0 && (
        <div className="pm-bulk-bar">
          <strong>{selectedIds.length} selected</strong>
          <label>
            <span className="pm-sr-only">Move selected tasks to column</span>
            <select value="" disabled={busy} onChange={(e) => e.target.value && void apply({ column_id: e.target.value })}>
              <option value="">Move to…</option>
              {columns.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="pm-sr-only">Set priority of selected tasks</span>
            <select value="" disabled={busy} onChange={(e) => e.target.value && void apply({ priority: e.target.value })}>
              <option value="">Priority…</option>
              {PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {humanize(p)}
                </option>
              ))}
            </select>
          </label>
          <button type="button" className="pm-btn pm-btn-small" disabled={busy} onClick={() => setConfirmArchive(true)}>
            Archive
          </button>
          <button type="button" className="pm-btn pm-btn-small" onClick={onClear}>
            Clear selection
          </button>
        </div>
      )}
      {error && (
        <p className="pm-error" role="alert">
          {error}
        </p>
      )}
      {summary && (
        <div className="pm-card" role="status">
          <div className="pm-row-between">
            <strong>
              {summary.succeeded} updated, {summary.failed} failed
            </strong>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => setSummary(null)}>
              Dismiss
            </button>
          </div>
          <ul className="pm-plain-list">
            {summary.lines.map((l) => (
              <li key={l.taskId} className={l.ok ? undefined : "pm-bulk-fail"}>
                <span className="pm-key">{l.label}</span> {l.ok ? "OK" : "Failed"}: {l.message}
              </li>
            ))}
          </ul>
        </div>
      )}
      <Dialog title="Archive tasks" open={confirmArchive} onClose={() => setConfirmArchive(false)}>
        <p>Archive {selectedIds.length} selected task(s)? Archived tasks leave the board and list but can be restored.</p>
        <div className="pm-actions">
          <button
            type="button"
            className="pm-btn pm-btn-primary"
            onClick={() => {
              setConfirmArchive(false);
              void apply({ archive: true });
            }}
          >
            Archive
          </button>
          <button type="button" className="pm-btn" onClick={() => setConfirmArchive(false)}>
            Cancel
          </button>
        </div>
      </Dialog>
    </div>
  );
}
