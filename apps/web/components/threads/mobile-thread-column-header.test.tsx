import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { MobileThreadColumnHeader } from "./mobile-thread-column-header";
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ features: { errands: true }, agentProfiles: { items: [] } }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true }),
}));
vi.mock("./thread-task-actions", () => ({ ThreadTaskMenuButton: () => null }));
afterEach(cleanup);
// @covers AC-TASKS-ERRANDS-010.5 AC-TASKS-ERRANDS-011.1
it("keeps the errands summary visible alongside phone thread navigation", () => {
  const thread = {
    taskId: "task",
    title: "Long thread title",
    errandsSummary: { live: 2, waiting: true },
  } as ActiveThread;
  const choose = vi.fn();
  const open = vi.fn();
  const view = render(
    <MobileThreadColumnHeader
      thread={thread}
      status={{ kind: "waiting", hasAttention: false, labelKey: "threads:statusWaiting" }}
      sessions={[]}
      selectedSessionId={null}
      onSelectSession={vi.fn()}
      onOpenTask={open}
      navigation={{ onChoose: choose }}
    />,
  );
  expect(screen.getByTestId("thread-errands-summary").textContent).toContain("2 errands");
  fireEvent.click(screen.getByTestId("thread-picker-trigger"));
  expect(choose).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "Open task" }));
  expect(open).toHaveBeenCalledWith("task");
  view.rerender(
    <MobileThreadColumnHeader
      thread={{ ...thread, errandsSummary: { live: 0, waiting: true } }}
      status={{ kind: "waiting", hasAttention: false, labelKey: "threads:statusWaiting" }}
      sessions={[]}
      selectedSessionId={null}
      onSelectSession={vi.fn()}
      onOpenTask={open}
      navigation={{ onChoose: choose }}
    />,
  );
  expect(screen.getByTestId("thread-errands-summary").textContent).toBe("Waiting on errands");
});
