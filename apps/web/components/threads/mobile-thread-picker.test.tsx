import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { MobileThreadPicker } from "./mobile-thread-picker";
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
afterEach(cleanup);
// @covers AC-TASKS-ERRANDS-010.5 AC-TASKS-ERRANDS-011.1
it("shows separate errand and native-subagent counts without changing task selection", () => {
  const thread = {
    taskId: "task",
    title: "Review API",
    sessionState: "WAITING_FOR_INPUT",
    pendingAction: null,
    workflowName: "Delivery",
    stepTitle: "Work",
    activeSubagentCount: 3,
    queuedPromptCount: 0,
    errandsSummary: { live: 2, waiting: true },
  } as ActiveThread;
  const select = vi.fn();
  render(
    <MobileThreadPicker
      threads={[thread]}
      selectedTaskId="task"
      open
      onOpenChange={vi.fn()}
      onSelect={select}
      onCloseAutoFocus={vi.fn()}
    />,
  );
  const row = screen.getByTestId("thread-picker-row-task");
  expect(row.textContent).toContain("2 errands");
  expect(row.textContent).toContain("3 subagents");
  fireEvent.click(row);
  expect(select).toHaveBeenCalledWith("task");
});
