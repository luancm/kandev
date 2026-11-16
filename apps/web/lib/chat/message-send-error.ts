import { WebSocketRequestError } from "@/lib/ws/request-error";

export type MessageSendErrorCode =
  | "connection-unavailable"
  | "no-active-session"
  | "session-unavailable"
  | "plan-comment-migration-pending"
  | "plan-comments-changed"
  | "preview-feedback-changed"
  | "primary-session-changed"
  | "late-answer-admission-failed";

export class MessageSendError extends Error {
  readonly code: MessageSendErrorCode;

  constructor(code: MessageSendErrorCode, message: string) {
    super(message);
    this.name = "MessageSendError";
    this.code = code;
  }
}

export function isMessageSendError(error: unknown): error is MessageSendError {
  if (!(error instanceof Error) || error.name !== "MessageSendError") return false;
  const code = (error as Error & { code?: unknown }).code;
  return (
    code === "connection-unavailable" ||
    code === "no-active-session" ||
    code === "session-unavailable" ||
    code === "plan-comment-migration-pending" ||
    code === "plan-comments-changed" ||
    code === "preview-feedback-changed" ||
    code === "primary-session-changed" ||
    code === "late-answer-admission-failed"
  );
}

/** The backend's stable reason is the only signal for this localized refusal. */
export function isErrandRecoveryRefusal(error: unknown): error is WebSocketRequestError {
  return (
    error instanceof WebSocketRequestError &&
    error.code === "CONFLICT" &&
    error.details?.reason === "errand_recovery_unresolved"
  );
}
