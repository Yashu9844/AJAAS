"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { describeError } from "../../api/errors";
import { createProject } from "../../api/projects-api";
import { projectPath } from "../../lib/paths";
import type { ProjectMode, ProjectVisibility } from "../../types/common";
import { Dialog } from "../ui/Dialog";

const KEY_RE = /^[A-Z0-9]{2,10}$/;

export function CreateProjectDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const key = String(data.get("key") ?? "").trim().toUpperCase();
    const name = String(data.get("name") ?? "").trim();
    if (!KEY_RE.test(key)) return setError("The key needs 2 to 10 letters or digits.");
    setBusy(true);
    setError("");
    try {
      const created = await createProject({
        key,
        name,
        visibility: String(data.get("visibility")) as ProjectVisibility,
        mode: String(data.get("mode")) as ProjectMode,
      });
      onClose();
      router.push(projectPath(created.id));
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog title="New project" open={open} onClose={onClose}>
      <form onSubmit={submit} className="pm-form">
        <label className="pm-field">
          <span>Key (2-10 letters or digits, used in ticket ids such as KEY-12)</span>
          <input name="key" required maxLength={10} autoCapitalize="characters" spellCheck={false} />
        </label>
        <label className="pm-field">
          <span>Name</span>
          <input name="name" required maxLength={120} />
        </label>
        <label className="pm-field">
          <span>Visibility</span>
          <select name="visibility" defaultValue="private">
            <option value="private">Private (members only)</option>
            <option value="tenant">Organisation (everyone can read)</option>
          </select>
        </label>
        <label className="pm-field">
          <span>Mode</span>
          <select name="mode" defaultValue="kanban">
            <option value="kanban">Kanban</option>
            <option value="scrum">Scrum (sprints and backlog)</option>
          </select>
        </label>
        {error && (
          <p className="pm-error" role="alert">
            {error}
          </p>
        )}
        <div className="pm-actions">
          <button type="submit" className="pm-btn pm-btn-primary" disabled={busy}>
            {busy ? "Creating…" : "Create project"}
          </button>
          <button type="button" className="pm-btn" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Dialog>
  );
}
