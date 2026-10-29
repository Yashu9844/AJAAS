"use client";

import { useState } from "react";
import type { Task, TaskUpdate } from "../../types/task";
import { MarkdownText } from "./MarkdownText";

interface Props {
  task: Task;
  canEdit: boolean;
  onSave: (changes: TaskUpdate) => Promise<boolean>;
}

/** Title and description editing. Saving sends the task version, so a stale edit is refused by the server (409). */
export function TaskHeaderEditor({ task, canEdit, onSave }: Props) {
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description);
  const [busy, setBusy] = useState(false);

  function begin() {
    setTitle(task.title);
    setDescription(task.description);
    setEditing(true);
  }

  async function commit() {
    const clean = title.trim();
    if (!clean) return;
    setBusy(true);
    const ok = await onSave({ title: clean, description });
    setBusy(false);
    if (ok) setEditing(false);
  }

  if (!editing) {
    return (
      <div className="pm-task-head">
        <div className="pm-task-head-top">
          <span className="pm-key">{task.key}</span>
          {canEdit && (
            <button type="button" className="pm-btn pm-btn-small" onClick={begin}>
              Edit
            </button>
          )}
        </div>
        <h2 className="pm-task-heading">{task.title}</h2>
        {task.description ? <MarkdownText text={task.description} /> : <p className="pm-muted">No description.</p>}
      </div>
    );
  }
  return (
    <form
      className="pm-task-head"
      onSubmit={(e) => {
        e.preventDefault();
        void commit();
      }}
    >
      <span className="pm-key">{task.key}</span>
      <label className="pm-field">
        <span>Title</span>
        <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={300} required autoFocus />
      </label>
      <label className="pm-field">
        <span>Description (markdown; mention people as @[Name](user:id))</span>
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={6} maxLength={50000} />
      </label>
      <div className="pm-actions">
        <button type="submit" className="pm-btn pm-btn-primary" disabled={busy}>
          {busy ? "Saving…" : "Save"}
        </button>
        <button type="button" className="pm-btn" onClick={() => setEditing(false)} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  );
}
