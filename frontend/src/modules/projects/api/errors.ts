/** Typed API errors and their mapping to messages a person can act on. */

export interface ErrorDetails {
  currentVersion?: number;
  retryAfterSeconds?: number;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ErrorDetails;

  constructor(status: number, code: string, message: string, details: ErrorDetails = {}) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** Builds an ApiError from an HTTP status and the raw response text (never throws). */
export function parseErrorBody(status: number, text: string, retryAfterSeconds?: number): ApiError {
  let code = status === 429 ? "RATE_LIMITED" : "HTTP_ERROR";
  let message = `Request failed (${status})`;
  const details: ErrorDetails = {};
  if (retryAfterSeconds !== undefined) details.retryAfterSeconds = retryAfterSeconds;
  try {
    const body: unknown = JSON.parse(text);
    const err = isRecord(body) ? body.error : undefined;
    if (isRecord(err)) {
      if (typeof err.code === "string") code = err.code;
      if (typeof err.message === "string") message = err.message;
      if (typeof err.current_version === "number") details.currentVersion = err.current_version;
    } else if (typeof err === "string") {
      message = err;
    }
  } catch {
    // Non-JSON body (proxy page, empty body): keep the generic message.
  }
  return new ApiError(status, code, message, details);
}

export function networkError(): ApiError {
  return new ApiError(0, "NETWORK_ERROR", "Cannot reach the server. Check your connection and the server address.");
}

export function sessionLostError(): ApiError {
  return new ApiError(401, "SESSION_LOST", "Your session has ended. Sign in again.");
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

export function isVersionConflict(e: unknown): e is ApiError {
  return isApiError(e) && e.status === 409 && e.code === "VERSION_CONFLICT";
}

export function isForbidden(e: unknown): boolean {
  return isApiError(e) && e.status === 403;
}

export function isNotFound(e: unknown): boolean {
  return isApiError(e) && e.status === 404;
}

const FRIENDLY: Record<string, string> = {
  VERSION_CONFLICT: "Someone else changed this just now. The latest version has been loaded.",
  WIP_LIMIT: "That column is at its work-in-progress limit.",
  DEPENDENCY_CYCLE: "That relation would create a dependency cycle.",
  TASK_HAS_HIERARCHY: "A task with subtasks or a parent cannot be moved to another project.",
  PROJECT_MODE: "This action is only available in scrum projects.",
  TIMER_RUNNING: "You already have a running timer. Stop it first.",
  TIME_LOCKED: "That time entry is locked.",
  TIME_TRACKING_DISABLED: "Time tracking is turned off for this project.",
  FILE_TOO_LARGE: "That file is too large (50 MB maximum).",
  FILE_TYPE_NOT_ALLOWED: "That file type is not allowed.",
  QUOTA_EXCEEDED: "The storage quota of your organisation is used up.",
  UPLOAD_MISMATCH: "The uploaded file did not match what was announced. Try again.",
  RATE_LIMITED: "Too many requests. Please wait a moment.",
  NETWORK_ERROR: "Cannot reach the server. Check your connection and the server address.",
};

/** The message to show a person for any thrown value. */
export function describeError(e: unknown): string {
  if (isApiError(e)) {
    if (e.status === 403) return "You do not have permission to do that.";
    if (e.status === 404) return "That item was not found, or you cannot see it.";
    const friendly = FRIENDLY[e.code];
    if (friendly) return friendly;
    return e.message;
  }
  return e instanceof Error ? e.message : "Something went wrong.";
}
