import { getWebSocketClient } from "@/lib/ws/connection";
import {
  normalizeErrandAttention,
  normalizeErrandBrowserAttention,
  type Errand,
  type ErrandBrowserSummary,
  type ErrandHistoryPage,
  type ErrandInspection,
  type ErrandBrowserRow,
  type ErrandCancelAllResponse,
  type ErrandListResponse,
} from "@/lib/types/errands";

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
  return Array.isArray(response?.errands) ? response.errands.map(normalizeErrandAttention) : [];
}

/** Reads compact live/outstanding Errands and authoritative task totals. */
export async function getErrandBrowserSummary(
  taskId: string,
  signal?: AbortSignal,
): Promise<ErrandBrowserSummary> {
  const client = requireClient();
  const payload = { task_id: taskId };
  const response = signal
    ? await client.request<ErrandBrowserSummary>(
        "errand.browser_summary",
        payload,
        undefined,
        signal,
      )
    : await client.request<ErrandBrowserSummary>("errand.browser_summary", payload);
  return { ...response, errands: response.errands.map(normalizeErrandBrowserAttention) };
}

/** Reads one explicit, parent-scoped page of finished Errand history. */
export async function getErrandHistoryPage(
  taskId: string,
  parentSessionId: string,
  options: { limit?: number; cursor?: string } = {},
  signal?: AbortSignal,
): Promise<ErrandHistoryPage> {
  const client = requireClient();
  const payload = {
    task_id: taskId,
    parent_session_id: parentSessionId,
    ...(options.limit === undefined ? {} : { limit: options.limit }),
    ...(options.cursor === undefined ? {} : { cursor: options.cursor }),
  };
  const response = signal
    ? await client.request<ErrandHistoryPage>("errand.history_page", payload, undefined, signal)
    : await client.request<ErrandHistoryPage>("errand.history_page", payload);
  return { ...response, errands: response.errands.map(normalizeErrandBrowserAttention) };
}

/** Verifies and reads only one retained child and its minimal parent identity. */
export async function inspectErrand(
  taskId: string,
  childSessionId: string,
  signal?: AbortSignal,
): Promise<ErrandInspection> {
  const client = requireClient();
  const payload = { task_id: taskId, child_session_id: childSessionId };
  const response = signal
    ? await client.request<ErrandInspection>("errand.inspect", payload, undefined, signal)
    : await client.request<ErrandInspection>("errand.inspect", payload);
  const errand: ErrandBrowserRow = normalizeErrandBrowserAttention(response.errand);
  return { ...response, errand };
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
