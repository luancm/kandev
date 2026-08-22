import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  useWorkflowMoveLongPress,
  WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS,
  WORKFLOW_MOVE_LONG_PRESS_MS,
} from "./workflow-move-proceed-button";
import { WorkflowMoveProceedButton } from "./workflow-move-proceed-button";

const touchMocks = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchMocks.enabled,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: [] },
      availableAgents: { items: [], loading: false, loaded: true, tools: [] },
    }),
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  touchMocks.enabled = false;
});

beforeEach(() => {
  vi.useFakeTimers();
});

const INSTRUCTIONS_TEST_ID = "workflow-move-instructions";
const PROCEED_TEST_ID = "proceed-next-step";
const OPTIONS_TEST_ID = "proceed-next-step-options";

function renderProceed(onProceed = vi.fn()) {
  render(
    <WorkflowMoveProceedButton
      nextStepName="Review"
      onProceed={onProceed}
      isMoving={false}
      testId={PROCEED_TEST_ID}
    />,
  );
  return onProceed;
}

function touchPointer(overrides: Record<string, unknown> = {}) {
  return {
    pointerType: "touch",
    button: 0,
    clientX: 20,
    clientY: 20,
    ...overrides,
  } as never;
}

function openHoveredForm() {
  const proceed = screen.getByTestId(PROCEED_TEST_ID);
  fireEvent.pointerEnter(proceed, { pointerType: "mouse" });
  // The fine-pointer affordance opens synchronously (no open delay).
  return screen.getByTestId(OPTIONS_TEST_ID);
}

describe("WorkflowMoveProceedButton", () => {
  it("moves directly on a short desktop click", () => {
    const onProceed = renderProceed();

    fireEvent.click(screen.getByTestId(PROCEED_TEST_ID));

    expect(onProceed).toHaveBeenCalledWith(undefined);
    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
  });

  it("reveals the options form itself from fine-pointer hover without an intermediate button", async () => {
    const onProceed = renderProceed();

    const form = openHoveredForm();

    // The real fields render directly inside the hover popover.
    expect(screen.getByTestId("workflow-move-agent-profile")).toBeTruthy();
    expect(screen.getByTestId(INSTRUCTIONS_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("workflow-move-model")).toBeNull();
    expect(form).toBeTruthy();
    expect(onProceed).not.toHaveBeenCalled();
  });

  it("submits the popover draft as one-shot entry options", async () => {
    const onProceed = vi.fn().mockResolvedValue(true);
    renderProceed(onProceed);
    openHoveredForm();

    fireEvent.change(screen.getByTestId(INSTRUCTIONS_TEST_ID), {
      target: { value: "create the PR ready for review" },
    });
    fireEvent.click(screen.getByTestId("workflow-move-submit"));
    await act(async () => {});

    expect(onProceed).toHaveBeenCalledWith({
      instructions: "create the PR ready for review",
    });
    // A successful optioned move closes the popover form.
    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
  });

  it("keeps the popover form and its draft open when the move fails", async () => {
    const onProceed = vi.fn().mockResolvedValue(false);
    renderProceed(onProceed);
    openHoveredForm();

    fireEvent.change(screen.getByTestId(INSTRUCTIONS_TEST_ID), {
      target: { value: "retry after capacity opens" },
    });
    fireEvent.click(screen.getByTestId("workflow-move-submit"));
    await act(async () => {});

    expect(onProceed).toHaveBeenCalledOnce();
    expect(screen.getByTestId(OPTIONS_TEST_ID)).toBeTruthy();
    expect((screen.getByTestId(INSTRUCTIONS_TEST_ID) as HTMLTextAreaElement).value).toBe(
      "retry after capacity opens",
    );
  });

  it("reveals the options form when the fine-pointer button receives keyboard focus", async () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.keyDown(proceed, { key: "Tab" });
    fireEvent.focus(proceed);

    expect(screen.getByTestId("workflow-move-agent-profile")).toBeTruthy();
  });

  it("does not reveal the options form when the button receives pointer focus", () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, { pointerType: "mouse", button: 0 });
    fireEvent.focus(proceed);

    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
  });

});

describe("WorkflowMoveProceedButton — hover close timing", () => {
  it("closes a hover affordance when the pointer leaves the whole surface", async () => {
    renderProceed();
    const form = openHoveredForm();

    fireEvent.pointerLeave(form, { relatedTarget: document.body });
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS));

    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
  });

  it("delays closing while the pointer crosses from the trigger toward the content", async () => {
    renderProceed();
    const form = openHoveredForm();

    fireEvent.pointerLeave(form, { relatedTarget: document.body });

    act(() => vi.advanceTimersByTime(50));
    expect(screen.getByTestId(OPTIONS_TEST_ID)).toBe(form);

    fireEvent.pointerEnter(form, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(200));

    expect(screen.getByTestId(OPTIONS_TEST_ID)).toBe(form);
  });

  it("cancels a delayed close when the pointer re-enters the trigger", async () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    openHoveredForm();

    fireEvent.pointerLeave(proceed, { relatedTarget: document.body });
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS / 2));

    fireEvent.pointerEnter(proceed, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS));

    expect(screen.getByTestId(OPTIONS_TEST_ID)).toBeTruthy();
  });

  it("keeps the popover open while the pointer interacts with the form fields", async () => {
    renderProceed();
    const form = openHoveredForm();

    // Interacting with a field pins the surface so pointer travel towards a
    // portaled dropdown cannot dismiss it.
    fireEvent.pointerDown(screen.getByTestId("workflow-move-agent-profile"));
    fireEvent.pointerLeave(form, { relatedTarget: document.body });
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS * 5));

    expect(screen.getByTestId(OPTIONS_TEST_ID)).toBeTruthy();
  });

  it("closes after the delayed pointer leave when the content is not entered", async () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    openHoveredForm();

    fireEvent.pointerLeave(proceed, { relatedTarget: document.body });

    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS));

    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
  });

});

describe("WorkflowMoveProceedButton — coarse pointer", () => {
  it("opens the Drawer after a long press and suppresses its duplicate click", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));

    expect(screen.getByTestId("workflow-move-options")).toBeTruthy();
    fireEvent.pointerUp(proceed, touchPointer());
    fireEvent.click(proceed);

    expect(onProceed).not.toHaveBeenCalled();
  });

  it("suppresses a long-press release retargeted to the Drawer submit action", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));

    fireEvent.click(screen.getByTestId("workflow-move-submit"));

    expect(onProceed).not.toHaveBeenCalled();
  });

  it("moves directly on a coarse-pointer short tap", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    fireEvent.pointerUp(proceed, touchPointer());
    fireEvent.click(proceed);

    expect(onProceed).toHaveBeenCalledWith(undefined);
    expect(screen.queryByTestId("workflow-move-options")).toBeNull();
  });

  it("keeps the desktop control compact and gives touch input a larger hitbox", () => {
    renderProceed();
    expect(screen.getByTestId(PROCEED_TEST_ID).className).toContain("h-6");

    cleanup();
    touchMocks.enabled = true;
    renderProceed();

    expect(screen.getByTestId(PROCEED_TEST_ID).className).toContain("min-h-11");
  });

});

describe("WorkflowMoveProceedButton — in-flight guarding", () => {
  it("suppresses rapid duplicate clicks before the move state re-renders", () => {
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.click(proceed);
    fireEvent.click(proceed);

    expect(onProceed).toHaveBeenCalledTimes(1);
  });

  it("keeps direct and options actions disabled while a move is in flight", () => {
    const onProceed = vi.fn();
    render(
      <WorkflowMoveProceedButton
        nextStepName="Review"
        onProceed={onProceed}
        isMoving
        testId={PROCEED_TEST_ID}
      />,
    );

    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    expect((proceed as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(proceed);
    expect(onProceed).not.toHaveBeenCalled();
  });

  it("returns focus to the direct button after closing the options popover with Escape", async () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.keyDown(proceed, { key: "Tab" });
    fireEvent.focus(proceed);
    const form = screen.getByTestId(OPTIONS_TEST_ID);

    fireEvent.keyDown(form, { key: "Escape" });
    // FocusScope restores focus inside a setTimeout(0) after unmount.
    act(() => vi.advanceTimersByTime(10));
    await act(async () => {});

    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
    expect(document.activeElement).toBe(proceed);
  });

  it("returns focus to the direct button after closing the options Drawer", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));
    fireEvent.pointerUp(proceed, touchPointer());
    fireEvent.click(proceed);
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    act(() => vi.runOnlyPendingTimers());

    expect(onProceed).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(proceed);
  });
});

describe("useWorkflowMoveLongPress", () => {
  type PointerHandlers = ReturnType<typeof useWorkflowMoveLongPress>["pointerHandlers"];

  it.each([
    ["pointerup", (handlers: PointerHandlers) => handlers.onPointerUp(touchPointer())],
    ["pointercancel", (handlers: PointerHandlers) => handlers.onPointerCancel(touchPointer())],
  ])("cancels before the threshold on %s", (_name, cancel) => {
    const onLongPress = vi.fn();
    const { result } = renderHook(() => useWorkflowMoveLongPress(onLongPress));

    act(() => {
      result.current.pointerHandlers.onPointerDown(touchPointer());
      cancel(result.current.pointerHandlers);
      vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS);
    });

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("cancels when movement passes the slop without preventing scrolling", () => {
    const onLongPress = vi.fn();
    const { result } = renderHook(() => useWorkflowMoveLongPress(onLongPress));
    const moveEvent = touchPointer({ clientX: 31, clientY: 20 });

    act(() => {
      result.current.pointerHandlers.onPointerDown(touchPointer());
      result.current.pointerHandlers.onPointerMove(moveEvent);
      vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS);
    });

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("cancels an in-flight timer when unmounted", () => {
    const onLongPress = vi.fn();
    const { result, unmount } = renderHook(() => useWorkflowMoveLongPress(onLongPress));

    act(() => result.current.pointerHandlers.onPointerDown(touchPointer()));
    unmount();
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("marks one synthetic click as handled after a completed long press", () => {
    const onLongPress = vi.fn();
    const { result } = renderHook(() => useWorkflowMoveLongPress(onLongPress));

    act(() => {
      result.current.pointerHandlers.onPointerDown(touchPointer());
      vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS);
    });

    expect(onLongPress).toHaveBeenCalledOnce();
    expect(result.current.consumePendingClick()).toBe(true);
    expect(result.current.consumePendingClick()).toBe(false);
  });
});
