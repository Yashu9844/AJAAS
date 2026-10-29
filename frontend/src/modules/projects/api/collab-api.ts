import type { Page } from "../types/common";
import type { ActivityEntry, Attachment, AttachmentInit, Comment, TimeEntry } from "../types/collab";
import { ApiError } from "./errors";
import { del, getData, getPage, patch, post } from "./client";
import { seg } from "./query";

export function listComments(taskId: string): Promise<Page<Comment>> {
  return getPage<Comment>(`/tasks/${seg(taskId)}/comments`, { limit: 100 });
}

export function addComment(taskId: string, body: string): Promise<Comment> {
  return post<Comment>(`/tasks/${seg(taskId)}/comments`, { body });
}

export function editComment(commentId: string, body: string): Promise<Comment> {
  return patch<Comment>(`/comments/${seg(commentId)}`, { body });
}

export function deleteComment(commentId: string): Promise<unknown> {
  return del(`/comments/${seg(commentId)}`);
}

export function listActivity(taskId: string, cursor?: string): Promise<Page<ActivityEntry>> {
  return getPage<ActivityEntry>(`/tasks/${seg(taskId)}/activity`, { cursor, limit: 20 });
}

export function listTimeEntries(taskId: string): Promise<Page<TimeEntry>> {
  return getPage<TimeEntry>(`/tasks/${seg(taskId)}/time`, { limit: 50 });
}

/** Logs finished work that ended now. */
export function logTime(taskId: string, minutes: number, note: string): Promise<TimeEntry> {
  const startedAt = new Date(Date.now() - minutes * 60_000).toISOString();
  return post<TimeEntry>(`/tasks/${seg(taskId)}/time`, { started_at: startedAt, minutes, note });
}

export function deleteTimeEntry(entryId: string): Promise<unknown> {
  return del(`/time-entries/${seg(entryId)}`);
}

export function getRunningTimer(): Promise<TimeEntry | null> {
  return getData<TimeEntry | null>("/time-entries/timer");
}

export function startTimer(taskId: string): Promise<TimeEntry> {
  return post<TimeEntry>("/time-entries/timer/start", { task_id: taskId });
}

export function stopTimer(): Promise<TimeEntry> {
  return post<TimeEntry>("/time-entries/timer/stop");
}

export function listAttachments(taskId: string): Promise<Page<Attachment>> {
  return getPage<Attachment>(`/tasks/${seg(taskId)}/attachments`, { limit: 50 });
}

export function deleteAttachment(attachmentId: string): Promise<unknown> {
  return del(`/attachments/${seg(attachmentId)}`);
}

/** A download link is opened only when it is a plain http(s) URL. */
export async function getDownloadUrl(attachmentId: string): Promise<string> {
  const data = await getData<{ url?: unknown }>(`/attachments/${seg(attachmentId)}/download`);
  const url = typeof data.url === "string" ? data.url : "";
  if (!isHttpUrl(url)) throw new ApiError(502, "BAD_RESPONSE", "The server returned an unusable download link.");
  return url;
}

export function isHttpUrl(value: string): boolean {
  try {
    const u = new URL(value);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

/** The three-step upload: announce, PUT to the presigned URL, complete. Throws ApiError with a readable message. */
export async function uploadAttachment(taskId: string, file: File): Promise<Attachment> {
  const init = await post<AttachmentInit>(`/tasks/${seg(taskId)}/attachments`, {
    filename: file.name,
    size: file.size,
    content_type: file.type || "application/octet-stream",
  });
  if (!isHttpUrl(init.upload.url)) throw new ApiError(502, "BAD_RESPONSE", "The server returned an unusable upload address.");
  let res: Response;
  try {
    res = await fetch(init.upload.url, { method: init.upload.method || "PUT", headers: init.upload.headers, body: file });
  } catch {
    throw new ApiError(0, "UPLOAD_FAILED", "The file could not be sent to storage. The storage server may be unreachable or may not allow this site.");
  }
  if (!res.ok) throw new ApiError(res.status, "UPLOAD_FAILED", `Storage refused the file (${res.status}).`);
  return post<Attachment>(`/attachments/${seg(init.attachment.id)}/complete`);
}
