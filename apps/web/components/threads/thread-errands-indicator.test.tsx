import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ThreadErrandsIndicator } from "./thread-errands-indicator";
const SUMMARY = "thread-errands-summary";
const state = vi.hoisted(() => ({ enabled: true }));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => state.enabled }));
afterEach(() => {
  cleanup();
  state.enabled = true;
});
// @covers AC-TASKS-ERRANDS-010.5 AC-TASKS-ERRANDS-010.6
describe("Threads errands summary", () => {
  it("shows live and waiting state, and updates to waiting only", () => {
    const view = render(<ThreadErrandsIndicator summary={{ live: 2, waiting: true }} />);
    expect(screen.getByTestId(SUMMARY).textContent).toContain("2 errands");
    view.rerender(<ThreadErrandsIndicator summary={{ live: 0, waiting: true }} />);
    expect(screen.getByTestId(SUMMARY).textContent).toBe("Waiting on errands");
  });
  it("omits absent, zero and disabled summaries", () => {
    const view = render(<ThreadErrandsIndicator />);
    expect(screen.queryByTestId(SUMMARY)).toBeNull();
    view.rerender(<ThreadErrandsIndicator summary={{ live: 0, waiting: false }} />);
    expect(screen.queryByTestId(SUMMARY)).toBeNull();
    state.enabled = false;
    view.rerender(<ThreadErrandsIndicator summary={{ live: 2, waiting: true }} />);
    expect(screen.queryByTestId(SUMMARY)).toBeNull();
  });
});
