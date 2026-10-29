"use client";

import { useRef, useState } from "react";
import { deleteAttachment, getDownloadUrl, listAttachments, uploadAttachment } from "../../api/collab-api";
import { describeError } from "../../api/errors";
import { useResource } from "../../hooks/use-resource";
import { useSession } from "../../hooks/use-session";
import { formatDateTime } from "../../lib/dates";
import { formatBytes } from "../../lib/format";
import type { Task } from "../../types/task";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";

const MAX_BYTES = 50 * 1024 * 1024;

/** Upload (init, PUT to the presigned URL, complete), list, download and delete. Errors are shown inline. */
export function AttachmentsSection({ task, canUpload }: { task: Task; canUpload: boolean }) {
  const session = useSession();
  const inputRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const res = useResource(`attachments:${task.id}`, () => listAttachments(task.id));

  async function attempt(action: () => Promise<unknown>) {
    setMessage("");
    try {
      await action();
    } catch (e) {
      setMessage(describeError(e));
    }
  }

  async function onPick(file: File | undefined) {
    if (!file) return;
    if (file.size > MAX_BYTES) return setMessage("That file is too large (50 MB maximum).");
    setBusy(true);
    await attempt(async () => {
      await uploadAttachment(task.id, file);
      res.reload();
    });
    setBusy(false);
    if (inputRef.current) inputRef.current.value = "";
  }

  const download = (id: string) =>
    attempt(async () => {
      const url = await getDownloadUrl(id);
      window.open(url, "_blank", "noopener,noreferrer");
    });
  const remove = (id: string) =>
    attempt(async () => {
      await deleteAttachment(id);
      res.reload();
    });

  const items = res.data?.items ?? [];
  return (
    <section className="pm-section" aria-labelledby={`at-${task.id}`}>
      <h3 id={`at-${task.id}`}>Attachments</h3>
      {res.loading && <SkeletonRows rows={1} height={28} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && items.length === 0 && <EmptyState title="No attachments" />}
      <ul className="pm-plain-list">
        {items.map((a) => (
          <li key={a.id} className="pm-row-between">
            <span>
              {a.filename} <span className="pm-muted">{formatBytes(a.size)} · {formatDateTime(a.created_at)}</span>
            </span>
            <span className="pm-actions">
              <button type="button" className="pm-btn pm-btn-small" onClick={() => void download(a.id)}>
                Download
              </button>
              {(a.uploaded_by === session.userId || canUpload) && (
                <button type="button" className="pm-btn pm-btn-small" onClick={() => void remove(a.id)} aria-label={`Delete ${a.filename}`}>
                  Delete
                </button>
              )}
            </span>
          </li>
        ))}
      </ul>
      {canUpload && (
        <div className="pm-actions">
          <label className="pm-btn">
            {busy ? "Uploading…" : "Attach a file"}
            <input ref={inputRef} type="file" className="pm-sr-only" disabled={busy} onChange={(e) => void onPick(e.target.files?.[0])} />
          </label>
        </div>
      )}
      {message && (
        <p className="pm-error" role="alert">
          {message}
        </p>
      )}
    </section>
  );
}
