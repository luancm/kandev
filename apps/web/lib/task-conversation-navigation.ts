import { isTaskDetailPath } from "./links";

/** Keep an explicit conversation choice reloadable without replacing the mounted task view. */
export function replaceTaskConversationUrl(taskId: string, sessionId: string): void {
  if (typeof window === "undefined" || !isTaskDetailPath(window.location.pathname, taskId)) return;
  const url = new URL(window.location.href);
  if (url.searchParams.get("sessionId") === sessionId) return;
  url.searchParams.set("sessionId", sessionId);
  window.history.replaceState(window.history.state, "", `${url.pathname}${url.search}${url.hash}`);
}
