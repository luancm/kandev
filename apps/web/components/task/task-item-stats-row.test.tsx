import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { TaskStatusSummaryErrands } from "@/lib/types/task-status-summary";
import { TaskItemStatsRow } from "./task-item-stats-row";

const WAITING_LABEL = "Waiting on errands";

const testState = vi.hoisted(() => ({
  useFeature: vi.fn(() => true),
  waitingLabel: "Waiting on errands",
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ sessionPollMode: { bySessionId: {} } }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: testState.useFeature }));

vi.mock("@/lib/config", () => ({ isDebugUI: () => false }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number }) => {
      if (key === "sidebar:errandsLive") return `${options?.count} errands`;
      if (key === "sidebar:errandsWaiting") return testState.waitingLabel;
      return key;
    },
  }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

afterEach(() => {
  cleanup();
  testState.useFeature.mockReturnValue(true);
  testState.waitingLabel = WAITING_LABEL;
});

const QUEUE = {
  session_id: "session-1",
  agent_profile_id: "profile-1",
  queued_at: "2026-09-17T10:00:00Z",
  reason: "session_capacity" as const,
  retrying: true,
};

describe("TaskItemStatsRow launch queue indicator", () => {
  it("does not activate its parent row on click or keyboard input", () => {
    const parentClick = vi.fn();
    const parentKeyDown = vi.fn();
    render(
      <div onClick={parentClick} onKeyDown={parentKeyDown}>
        <TaskItemStatsRow launchQueue={QUEUE} />
      </div>,
    );

    const indicator = screen.getByTestId("sidebar-task-launch-queue");
    fireEvent.click(indicator);
    fireEvent.keyDown(indicator, { key: "Enter" });
    fireEvent.keyDown(indicator, { key: " " });

    expect(parentClick).not.toHaveBeenCalled();
    expect(parentKeyDown).not.toHaveBeenCalled();
  });
});

describe("TaskItemStatsRow errands summary", () => {
  function renderErrandsRow(errandsSummary?: TaskStatusSummaryErrands | null) {
    return render(<TaskItemStatsRow errandsSummary={errandsSummary} showRelativeTime={false} />);
  }

  it("shows the backend live count and waiting state", () => {
    renderErrandsRow({ live: 2, waiting: true });

    expect(screen.getByTestId("sidebar-task-errands-count").textContent).toBe("2 errands");
    expect(screen.getByTestId("sidebar-task-errands-waiting").textContent).toBe(WAITING_LABEL);
  });

  it("shows waiting without inventing live children", () => {
    renderErrandsRow({ live: 0, waiting: true });

    expect(screen.queryByTestId("sidebar-task-errands-count")).toBeNull();
    expect(screen.getByTestId("sidebar-task-errands-waiting").textContent).toBe(WAITING_LABEL);
  });

  it("omits empty summaries and all summaries while the feature is disabled", () => {
    renderErrandsRow({ live: 0, waiting: false });
    expect(screen.queryByTestId("sidebar-task-errands")).toBeNull();

    cleanup();
    testState.useFeature.mockReturnValue(false);
    renderErrandsRow({ live: 3, waiting: true });
    expect(screen.queryByTestId("sidebar-task-errands")).toBeNull();
  });

  it("truncates a long localized waiting label and keeps its full accessible name", () => {
    testState.waitingLabel =
      "Waiting on the results of several lengthy errand descriptions across many workspaces";
    renderErrandsRow({ live: 0, waiting: true });

    const waiting = screen.getByTestId("sidebar-task-errands-waiting");
    expect(waiting.classList.contains("truncate")).toBe(true);
    expect(waiting.getAttribute("title")).toBe(testState.waitingLabel);
    expect(waiting.getAttribute("aria-label")).toBe(testState.waitingLabel);
  });
});
