"use client";

import { useState, type FormEvent } from "react";
import { addComment, deleteComment, editComment, listComments } from "../../api/collab-api";
import { describeError } from "../../api/errors";
import { useEnsureNames, usePeople } from "../../hooks/use-people";
import { useResource } from "../../hooks/use-resource";
import { useSession } from "../../hooks/use-session";
import { useToast } from "../../hooks/use-toast";
import { formatDateTime } from "../../lib/dates";
import { mentionToken } from "../../lib/mentions";
import type { Comment } from "../../types/collab";
import type { Member } from "../../types/project";
import type { Task } from "../../types/task";
import { Avatar } from "../ui/Chips";
import { EmptyState, ErrorNotice, SkeletonRows } from "../ui/States";
import { MarkdownText } from "./MarkdownText";

interface Props {
  task: Task;
  members: Member[];
  canComment: boolean;
}

function MentionPicker({ members, onPick }: { members: Member[]; onPick: (userId: string) => void }) {
  const { nameOf } = usePeople();
  return (
    <select aria-label="Mention a person" value="" onChange={(e) => e.target.value && onPick(e.target.value)}>
      <option value="">@ Mention…</option>
      {members.map((m) => (
        <option key={m.user_id} value={m.user_id}>
          {nameOf(m.user_id)}
        </option>
      ))}
    </select>
  );
}

function CommentItem({ comment, mine, onChanged }: { comment: Comment; mine: boolean; onChanged: () => void }) {
  const { nameOf } = usePeople();
  const toast = useToast();
  const [editing, setEditing] = useState(false);
  const [body, setBody] = useState(comment.body);

  async function run(action: () => Promise<unknown>) {
    try {
      await action();
      setEditing(false);
      onChanged();
    } catch (e) {
      toast.show(describeError(e), "error");
    }
  }

  const author = nameOf(comment.author_user_id);
  if (comment.deleted) return <li className="pm-comment pm-muted">Comment deleted</li>;
  return (
    <li className="pm-comment">
      <Avatar name={author} />
      <div className="pm-comment-body">
        <div className="pm-comment-meta">
          <strong>{author}</strong> <span className="pm-muted">{formatDateTime(comment.created_at)}</span>
          {comment.edited_at && <span className="pm-muted"> (edited)</span>}
        </div>
        {editing ? (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void run(() => editComment(comment.id, body));
            }}
          >
            <label className="pm-sr-only" htmlFor={`ce-${comment.id}`}>
              Edit comment
            </label>
            <textarea id={`ce-${comment.id}`} value={body} onChange={(e) => setBody(e.target.value)} rows={3} maxLength={10000} />
            <div className="pm-actions">
              <button type="submit" className="pm-btn pm-btn-primary pm-btn-small" disabled={!body.trim()}>
                Save
              </button>
              <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(false)}>
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <MarkdownText text={comment.body} />
        )}
        {mine && !editing && (
          <div className="pm-actions">
            <button type="button" className="pm-btn pm-btn-small" onClick={() => setEditing(true)}>
              Edit
            </button>
            <button type="button" className="pm-btn pm-btn-small" onClick={() => void run(() => deleteComment(comment.id))}>
              Delete
            </button>
          </div>
        )}
      </div>
    </li>
  );
}

export function CommentsSection({ task, members, canComment }: Props) {
  const session = useSession();
  const toast = useToast();
  const { nameOf } = usePeople();
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const res = useResource(`comments:${task.id}`, () => listComments(task.id));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!draft.trim()) return;
    setBusy(true);
    try {
      await addComment(task.id, draft.trim());
      setDraft("");
      res.reload();
    } catch (err) {
      toast.show(describeError(err), "error");
    } finally {
      setBusy(false);
    }
  }

  const comments = res.data?.items ?? [];
  useEnsureNames(comments.map((c) => c.author_user_id));
  return (
    <section className="pm-section" aria-labelledby={`cm-${task.id}`}>
      <h3 id={`cm-${task.id}`}>Comments</h3>
      {res.loading && <SkeletonRows rows={2} height={48} />}
      {res.error !== undefined && <ErrorNotice error={res.error} onRetry={res.reload} />}
      {res.data && comments.length === 0 && <EmptyState title="No comments yet" />}
      <ul className="pm-comments">
        {comments.map((c) => (
          <CommentItem key={c.id} comment={c} mine={c.author_user_id === session.userId} onChanged={res.reload} />
        ))}
      </ul>
      {canComment && (
        <form onSubmit={submit} className="pm-comment-form">
          <label className="pm-sr-only" htmlFor={`cn-${task.id}`}>
            New comment
          </label>
          <textarea id={`cn-${task.id}`} value={draft} onChange={(e) => setDraft(e.target.value)} rows={3} maxLength={10000} placeholder="Write a comment (markdown)" />
          <div className="pm-actions">
            <button type="submit" className="pm-btn pm-btn-primary" disabled={busy || !draft.trim()}>
              Comment
            </button>
            <MentionPicker members={members} onPick={(id) => setDraft((d) => `${d}${d && !d.endsWith(" ") ? " " : ""}${mentionToken(nameOf(id), id)} `)} />
          </div>
        </form>
      )}
    </section>
  );
}
