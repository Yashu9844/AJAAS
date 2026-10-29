"use client";

import { describeError } from "../../api/errors";
import { addTaskLabel, assignUsers, moveTask, removeTaskLabel, unassignUser } from "../../api/tasks-api";
import { usePeople } from "../../hooks/use-people";
import { useToast } from "../../hooks/use-toast";
import { dateKeyToDueAt, isoToDateKey } from "../../lib/dates";
import { formatMinutes, humanize, parseMinutes } from "../../lib/format";
import { PRIORITIES, type Priority } from "../../types/common";
import type { Column, Label, Member } from "../../types/project";
import type { Task, TaskUpdate } from "../../types/task";

interface Props {
  task: Task;
  columns: Column[];
  labels: Label[];
  members: Member[];
  canWrite: boolean;
  onSave: (changes: TaskUpdate) => Promise<boolean>;
  onReload: () => void;
}

/** Column, priority, due date, estimate, assignees (project members) and labels. */
export function TaskProperties({ task, columns, labels, members, canWrite, onSave, onReload }: Props) {
  const toast = useToast();
  const { nameOf } = usePeople();

  async function run(action: () => Promise<unknown>) {
    try {
      await action();
      onReload();
    } catch (e) {
      toast.show(describeError(e), "error");
    }
  }

  const toggleAssignee = (userId: string, on: boolean) => run(() => (on ? assignUsers(task.id, [userId]) : unassignUser(task.id, userId)));
  const toggleLabel = (labelId: string, on: boolean) => run(() => (on ? addTaskLabel(task.id, labelId) : removeTaskLabel(task.id, labelId)));
  const changeColumn = (columnId: string) => run(() => moveTask(task.id, { column_id: columnId, version: task.version }));

  return (
    <div className="pm-props">
      <label className="pm-field">
        <span>Status</span>
        <select value={task.column_id} disabled={!canWrite} onChange={(e) => void changeColumn(e.target.value)}>
          {columns.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </label>
      <label className="pm-field">
        <span>Priority</span>
        <select value={task.priority} disabled={!canWrite} onChange={(e) => void onSave({ priority: e.target.value as Priority })}>
          {PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {humanize(p)}
            </option>
          ))}
        </select>
      </label>
      <label className="pm-field">
        <span>Due date</span>
        <input
          type="date"
          value={isoToDateKey(task.due_at)}
          disabled={!canWrite}
          onChange={(e) => void onSave({ due_at: e.target.value ? dateKeyToDueAt(e.target.value) : null })}
        />
      </label>
      <label className="pm-field">
        <span>Estimate (e.g. 1h 30m)</span>
        <input
          key={`${task.id}-${task.version}`}
          defaultValue={task.estimate_minutes ? formatMinutes(task.estimate_minutes) : ""}
          disabled={!canWrite}
          onBlur={(e) => {
            const v = e.target.value.trim();
            const minutes = v === "" ? null : parseMinutes(v);
            if (v !== "" && minutes === null) return toast.show("Use a form like 90, 1h 30m or 45m.", "error");
            if (minutes !== task.estimate_minutes) void onSave({ estimate_minutes: minutes });
          }}
        />
      </label>
      <fieldset className="pm-field pm-checks" disabled={!canWrite}>
        <legend>Assignees</legend>
        {members.length === 0 && <span className="pm-muted">No members.</span>}
        {members.map((m) => (
          <label key={m.user_id} className="pm-check">
            <input type="checkbox" checked={task.assignee_ids.includes(m.user_id)} onChange={(e) => void toggleAssignee(m.user_id, e.target.checked)} />
            <span>{nameOf(m.user_id)}</span>
          </label>
        ))}
      </fieldset>
      <fieldset className="pm-field pm-checks" disabled={!canWrite}>
        <legend>Labels</legend>
        {labels.length === 0 && <span className="pm-muted">No labels in this project.</span>}
        {labels.map((l) => (
          <label key={l.id} className="pm-check">
            <input type="checkbox" checked={task.label_ids.includes(l.id)} onChange={(e) => void toggleLabel(l.id, e.target.checked)} />
            <span>{l.name}</span>
          </label>
        ))}
      </fieldset>
    </div>
  );
}
