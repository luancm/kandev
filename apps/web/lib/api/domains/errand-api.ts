import { getWebSocketClient } from "@/lib/ws/connection";
import type { Errand, ErrandCancelAllResponse, ErrandListResponse } from "@/lib/types/errands";

// i18n-exempt: programmer precondition; UI callers render localized errors.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/** Cancelling stops errand sessions, which can outlast the default request timeout. */
const ERRAND_MUTATION_TIMEOUT_MS = 15_000;

function requireClient() {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  return client;
}

/** Every errand of a task, as the backend projects them. */
export async function listErrands(taskId: string): Promise<Errand[]> {
  const response = await requireClient().request<ErrandListResponse>("errand.list", {
    task_id: taskId,
  });
  return Array.isArray(response?.errands) ? response.errands : [];
}

/** Cancels one errand; the errand ID is its session ID. */
export async function cancelErrand(sessionId: string): Promise<void> {
  await requireClient().request(
    "errand.cancel",
    { session_id: sessionId },
    ERRAND_MUTATION_TIMEOUT_MS,
  );
}

/** Cancels every live errand of a task; resolves with how many were cancelled. */
export async function cancelAllErrands(taskId: string): Promise<number> {
  const response = await requireClient().request<ErrandCancelAllResponse>(
    "errand.cancel_all",
    { task_id: taskId },
    ERRAND_MUTATION_TIMEOUT_MS,
  );
  return response?.cancelled ?? 0;
}

/** Captures the errand's latest settled turn as a result for its parent. */
export async function sendErrandResult(sessionId: string): Promise<void> {
  await requireClient().request(
    "errand.send_result",
    { session_id: sessionId },
    ERRAND_MUTATION_TIMEOUT_MS,
  );
}
