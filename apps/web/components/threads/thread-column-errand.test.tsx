import { fireEvent, screen } from "@testing-library/react";
import { it, expect } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/ids";
import { render, sessionMocks } from "./threads-board.test-helpers";
import { ThreadColumn } from "./thread-column";

const TIMESTAMP = "2026-01-01T00:00:00Z";

it("keeps an explicitly inspected child selected outside ordinary membership", async () => {
  sessionMocks.inspectedChildId = "child";
  sessionMocks.sessionLists.set(
    "a",
    ["session-a", "session-b"].map((id) => ({
      id,
      task_id: "a",
      state: "IDLE",
      is_primary: id === "session-a",
      started_at: TIMESTAMP,
      updated_at: TIMESTAMP,
    })),
  );
  render(
    <StateProvider
      initialState={{
        taskSessions: {
          items: {
            child: {
              id: sessionId("child"),
              task_id: taskId("a"),
              role: "errand",
              parent_session_id: sessionId("session-a"),
              state: "WAITING_FOR_INPUT",
              started_at: TIMESTAMP,
              updated_at: TIMESTAMP,
            },
          },
        },
      }}
    >
      <ThreadColumn
        thread={{
          taskId: "a",
          title: "Task A",
          workflowId: "wf",
          workflowName: "Delivery",
          stepTitle: "Build",
          sessionId: "session-a",
          sessionState: "RUNNING",
          pendingAction: null,
          activeSubagentCount: 0,
          queuedPromptCount: 0,
          lastActivityAt: TIMESTAMP,
        }}
        isPreloaded
        isDetailActive
        onOpenTask={() => {}}
      />
    </StateProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "child" }));
  expect(await screen.findByTestId("thread-conversation-child")).toBeTruthy();
  expect(screen.queryByTestId("thread-conversation-session-a")).toBeNull();
  expect(sessionMocks.sessionLists.get("a")?.map((row) => row.id)).toEqual([
    "session-a",
    "session-b",
  ]);
  expect(screen.queryByTestId("thread-session-tab-child")).toBeNull();
  const parentTab = screen.getByTestId("thread-session-tab-session-a");
  expect(parentTab.getAttribute("data-state")).toBe("active");
  fireEvent.mouseDown(screen.getByTestId("thread-session-tab-session-b"), { button: 0 });
  expect(await screen.findByTestId("thread-conversation-session-b")).toBeTruthy();
  fireEvent.mouseDown(screen.getByTestId("thread-session-tab-session-a"), { button: 0 });
  expect(await screen.findByTestId("thread-conversation-session-a")).toBeTruthy();
  expect(screen.queryByTestId("thread-conversation-child")).toBeNull();
});
