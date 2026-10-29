import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KanbanCardErrands } from "./kanban-card-errands";

const WAITING_LABEL = "Waiting on errands";

const testState = vi.hoisted(() => ({
  enabled: true,
  waitingLabel: "Waiting on errands",
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => testState.enabled,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number }) => {
      if (key === "kanban:errandsLive") return `${options?.count} errands`;
      if (key === "kanban:errandsWaiting") return testState.waitingLabel;
      return key;
    },
  }),
}));

afterEach(() => {
  cleanup();
  testState.enabled = true;
  testState.waitingLabel = WAITING_LABEL;
});

describe("KanbanCardErrands", () => {
  it("shows the live count and waiting state from the bounded summary", () => {
    render(<KanbanCardErrands summary={{ live: 2, waiting: true }} />);

    expect(screen.getByTestId("task-errands-count").textContent).toBe("2 errands");
    expect(screen.getByTestId("task-errands-waiting").textContent).toBe(WAITING_LABEL);
  });

  it("shows waiting without claiming that any errands are live", () => {
    render(<KanbanCardErrands summary={{ live: 0, waiting: true }} />);

    expect(screen.queryByTestId("task-errands-count")).toBeNull();
    expect(screen.getByTestId("task-errands-waiting").textContent).toBe(WAITING_LABEL);
  });

  it("omits absent and zero summaries", () => {
    const { rerender } = render(<KanbanCardErrands />);
    expect(screen.queryByTestId("task-errands-summary")).toBeNull();

    rerender(<KanbanCardErrands summary={{ live: 0, waiting: false }} />);
    expect(screen.queryByTestId("task-errands-summary")).toBeNull();
  });

  it("omits the decoration while errands are disabled", () => {
    testState.enabled = false;
    render(<KanbanCardErrands summary={{ live: 2, waiting: true }} />);

    expect(screen.queryByTestId("task-errands-summary")).toBeNull();
  });

  it("truncates long localized waiting text while retaining the full title", () => {
    testState.waitingLabel =
      "Waiting on the results of several lengthy errand descriptions across many workspaces";
    render(<KanbanCardErrands summary={{ live: 0, waiting: true }} />);

    const waiting = screen.getByTestId("task-errands-waiting");
    expect(waiting.classList.contains("truncate")).toBe(true);
    expect(waiting.getAttribute("title")).toBe(testState.waitingLabel);
  });
});
