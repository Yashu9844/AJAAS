export interface Comment {
  id: string;
  task_id: string;
  author_user_id: string;
  body: string;
  edited_at: string | null;
  deleted_at: string | null;
  deleted: boolean;
  created_at: string;
}

export interface TimeEntry {
  id: string;
  task_id: string;
  user_id: string;
  started_at: string;
  ended_at: string | null;
  minutes: number;
  note: string;
  source: string;
  auto_stopped: boolean;
}

export interface Attachment {
  id: string;
  task_id: string;
  filename: string;
  content_type: string;
  size: number;
  status: string;
  uploaded_by: string;
  created_at: string;
}

export interface UploadTarget {
  url: string;
  method: string;
  headers: Record<string, string>;
  expires_at: string;
}

export interface AttachmentInit {
  attachment: Attachment;
  upload: UploadTarget;
}

export interface ActivityEntry {
  seq: number;
  id: string;
  task_id: string | null;
  actor_user_id: string;
  type: string;
  data: Record<string, unknown>;
  occurred_at: string;
}
