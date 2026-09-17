import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ActionMessage } from "./action-message";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

afterEach(cleanup);

const inactiveGitPushAlert: Message = {
  id: "git-current-state-error",
  session_id: toSessionId("session-1"),
  task_id: toTaskId("task-1"),
  author_type: "agent",
  content: "Git push failed",
  type: "error",
  created_at: "2026-05-30T00:00:00Z",
  metadata: {
    git_operation_error: true,
    git_push_alert_active: false,
    actions: [{ type: "ws_request", label: "Fix", test_id: "git-fix-button" }],
  },
};

it("hides an inactive durable Git push alert card and Fix action", () => {
  render(
    <StateProvider>
      <ActionMessage comment={inactiveGitPushAlert} />
    </StateProvider>,
  );
  expect(screen.queryByTestId("git-fix-button")).toBeNull();
});
