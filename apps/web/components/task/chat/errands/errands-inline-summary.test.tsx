import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { makeErrand } from "@/lib/state/slices/errands/errands.test-helpers";
import { ErrandsInlineSummary } from "./errands-inline-summary";
const DISCLOSURE = "errands-inline-disclosure";
const EXPANDED = "aria-expanded";
const mocks = vi.hoisted(() => ({
  enabled: true,
  errands: [] as import("@/lib/types/errands").Errand[],
  cancelAll: vi.fn(),
  open: vi.fn(),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => mocks.enabled }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      taskSessions: { items: {} },
      errands: { byTaskId: {} },
      agentProfiles: { items: [] },
      sessionModels: { bySessionId: {} },
      activeModel: { bySessionId: {} },
    }),
}));
vi.mock("@/hooks/domains/session/use-task-errands", () => ({
  useTaskErrands: () => ({
    errands: mocks.errands,
    errandsByParent: Object.groupBy(mocks.errands, (errand) => errand.parent_session_id),
    liveCount: mocks.errands.filter((errand) => errand.status === "running").length,
    isLoaded: true,
  }),
}));
vi.mock("@/hooks/domains/session/use-errand-actions", () => ({
  useErrandActions: () => ({ cancelAllErrands: mocks.cancelAll, isPending: () => false }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("./use-open-task-session", () => ({ useOpenTaskSession: () => mocks.open }));
afterEach(cleanup);
beforeEach(() => {
  mocks.enabled = true;
  mocks.errands = [
    makeErrand({
      errand_id: "a",
      task_id: "task",
      title: "Audit",
      parent_session_id: "parent",
      status: "running",
    }),
    makeErrand({
      errand_id: "b",
      task_id: "task",
      title: "Review",
      parent_session_id: "other",
      status: "done",
    }),
  ];
  mocks.cancelAll.mockReset().mockResolvedValue(1);
  mocks.open.mockReset();
});
// @covers AC-TASKS-ERRANDS-010.10 AC-TASKS-ERRANDS-011.6
it("starts collapsed and keeps task-wide Stop and Open independent of disclosure", () => {
  render(<ErrandsInlineSummary taskId="task" sessionId="parent" />);
  const disclosure = screen.getByTestId(DISCLOSURE);
  expect(disclosure.getAttribute(EXPANDED)).toBe("false");
  fireEvent.click(screen.getByTestId("errands-stop-all"));
  expect(mocks.cancelAll).toHaveBeenCalledWith("task");
  expect(disclosure.getAttribute(EXPANDED)).toBe("false");
  fireEvent.click(disclosure);
  expect(screen.getAllByTestId("errands-panel-group")).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "Open errand Audit" }));
  expect(mocks.open).toHaveBeenCalledWith({ taskId: "task", sessionId: "a", title: "Audit" });
  expect(disclosure.getAttribute(EXPANDED)).toBe("true");
});
it("hides empty, flag-off and child views and resets expansion on session changes", () => {
  const view = render(<ErrandsInlineSummary taskId="task" sessionId="parent" />);
  fireEvent.click(screen.getByTestId(DISCLOSURE));
  view.rerender(<ErrandsInlineSummary taskId="task" sessionId="other" />);
  expect(screen.getByTestId(DISCLOSURE).getAttribute(EXPANDED)).toBe("false");
  view.rerender(<ErrandsInlineSummary taskId="task" sessionId="a" sessionRole="errand" />);
  expect(screen.queryByTestId("errands-inline-summary")).toBeNull();
  mocks.enabled = false;
  view.rerender(<ErrandsInlineSummary taskId="task" sessionId="parent" />);
  expect(screen.queryByTestId("errands-inline-summary")).toBeNull();
  mocks.enabled = true;
  mocks.errands = [];
  view.rerender(<ErrandsInlineSummary taskId="task" sessionId="parent" />);
  expect(screen.queryByTestId("errands-inline-summary")).toBeNull();
});
