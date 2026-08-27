import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkflowStepper, type WorkflowStepperStep } from "./workflow-stepper";

const mocks = vi.hoisted(() => {
  const plan = {
    setPlanMode: vi.fn(),
    setActiveDocument: vi.fn(),
    closeDocument: vi.fn(),
    removeContextFile: vi.fn(),
    applyBuiltInPreset: vi.fn(),
  };
  return {
    workflowMove: { move: vi.fn(), isMoving: false },
    toast: vi.fn(),
    touchDrawer: { enabled: false },
    plan,
    appState: {
      tasks: { activeSessionId: "session-1" },
      chatInput: { planModeBySessionId: { "session-1": true } },
      setPlanMode: plan.setPlanMode,
      setActiveDocument: plan.setActiveDocument,
      agentProfiles: { items: [] },
      availableAgents: { items: [], loading: false, loaded: true, tools: [] },
    },
  };
});

vi.mock("@/hooks/domains/kanban/use-workflow-move", () => ({
  useWorkflowMove: () => mocks.workflowMove,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => mocks.touchDrawer.enabled,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

// useToolbarCollapsed is mocked because the test DOM can't measure offsetWidth.
const collapsedMock = vi.fn(() => false);
vi.mock("@/hooks/use-toolbar-collapsed", () => ({
  useToolbarCollapsed: () => collapsedMock(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.appState) => unknown) => selector(mocks.appState),
}));
vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: (selector: (state: { removeFile: () => void }) => unknown) =>
    selector({ removeFile: mocks.plan.removeContextFile }),
}));
vi.mock("@/lib/state/layout-store", () => ({
  useLayoutStore: (selector: (state: { closeDocument: () => void }) => unknown) =>
    selector({ closeDocument: mocks.plan.closeDocument }),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { applyBuiltInPreset: () => void }) => unknown) =>
    selector({ applyBuiltInPreset: mocks.plan.applyBuiltInPreset }),
}));

const STEP_SPEC_ID = "workflow-step-Spec";
const STEP_WORK_ID = "workflow-step-Work";
const STEP_REVIEW_ID = "workflow-step-Review";

const MINIMAL_ID = "workflow-stepper-minimal";
const MOVE_HERE_ID = "workflow-step-move-here";
const OPTIONS_ID = "workflow-step-move-options";
const OPTIONS_TRIGGER_ID = "workflow-step-move-options-trigger";
const OPTIONS_CONTENT_ID = "workflow-step-move-options-content";
const INSTRUCTIONS_ID = "workflow-move-instructions";
const AGENT_PROFILE_ID = "workflow-move-agent-profile";
const POPOVER_CONTENT_SELECTOR = '[data-slot="popover-content"]';
const TASK_ID = "task-1";
const WORKFLOW_ID = "wf-1";
const CURRENT_STEP_ID = "b";

const STEPS: WorkflowStepperStep[] = [
  { id: "a", name: "Spec", color: "#111", position: 0 },
  { id: "b", name: "Work", color: "#222", position: 1 },
  { id: "c", name: "Review", color: "#333", position: 2 },
];

beforeEach(() => {
  mocks.workflowMove.move.mockReset();
  mocks.workflowMove.move.mockResolvedValue({ disposition: "committed", response: {} });
  mocks.toast.mockReset();
  mocks.touchDrawer.enabled = false;
  Object.values(mocks.plan).forEach((mock) => mock.mockReset());
  mocks.appState.chatInput.planModeBySessionId["session-1"] = true;
});

function renderStepper() {
  render(
    <WorkflowStepper
      steps={STEPS}
      currentStepId={CURRENT_STEP_ID}
      taskId={TASK_ID}
      workflowId={WORKFLOW_ID}
    />,
  );
}

async function openStepActions(stepName = "Spec") {
  fireEvent.pointerEnter(screen.getByTestId(`workflow-step-${stepName}`));
  await screen.findByRole("button", { name: /move here/i });
}

describe("WorkflowStepper", () => {
  it("renders every step when there is room (not collapsed)", () => {
    collapsedMock.mockReturnValue(false);
    render(<WorkflowStepper steps={STEPS} currentStepId={CURRENT_STEP_ID} />);

    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.queryByTestId(MINIMAL_ID)).toBeNull();
    // All steps render under the persistent outer container.
    expect(screen.getByTestId(STEP_SPEC_ID)).toBeTruthy();
    expect(screen.getByTestId(STEP_WORK_ID)).toBeTruthy();
    expect(screen.getByTestId(STEP_REVIEW_ID)).toBeTruthy();
  });

  it("collapses to only the current step when space runs out", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId={CURRENT_STEP_ID} />);

    // Outer container persists across variants (stable e2e locator); minimal child marks collapsed state.
    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.getByTestId(MINIMAL_ID)).toBeTruthy();

    // Current step keeps its test id + aria-current in either variant.
    const current = screen.getByTestId(STEP_WORK_ID);
    expect(current.getAttribute("aria-current")).toBe("step");
    expect(screen.queryByTestId(STEP_SPEC_ID)).toBeNull();
    expect(screen.queryByTestId(STEP_REVIEW_ID)).toBeNull();

    // Position indicator reflects the current step out of the total.
    expect(screen.getByText("2/3")).toBeTruthy();
  });

  it("falls back to the first step when collapsed with no current step", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId={null} />);

    // Fallback step isn't the real current step, so it must not claim aria-current.
    expect(screen.getByTestId(STEP_SPEC_ID).getAttribute("aria-current")).toBeNull();
    expect(screen.getByText("1/3")).toBeTruthy();
  });

  it("renders the collapsed stepper as a passive indicator without a navigator trigger", () => {
    collapsedMock.mockReturnValue(true);
    renderStepper();

    // The minimal stepper is a passive div: no button, no target navigator, no options action.
    const minimal = screen.getByTestId(MINIMAL_ID);
    expect(minimal.tagName).toBe("DIV");
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByTestId("workflow-step-c-move-options")).toBeNull();
  });

  it("uses bounded sizing for the anchored current-step surface", async () => {
    collapsedMock.mockReturnValue(false);
    render(<WorkflowStepper steps={STEPS} currentStepId={CURRENT_STEP_ID} />);

    fireEvent.pointerEnter(screen.getByTestId(STEP_WORK_ID));
    await screen.findByText("Current step");

    // The current step cannot be moved to, so the surface stays content-sized.
    const content = document.querySelector(POPOVER_CONTENT_SELECTOR);
    expect(content?.className).toContain("max-w-[min(24rem,calc(100vw-1rem))]");
    expect(content?.className).toContain("overflow-hidden");
  });

  it("shows the one-shot options disclosure on a movable step surface", async () => {
    collapsedMock.mockReturnValue(false);
    renderStepper();

    await openStepActions();
    expect(screen.getByTestId(OPTIONS_ID)).toBeTruthy();
    expect(screen.getByTestId(OPTIONS_TRIGGER_ID)).toBeTruthy();
    expect(screen.getByTestId(OPTIONS_CONTENT_ID).hasAttribute("hidden")).toBe(true);
    expect(screen.queryByTestId(INSTRUCTIONS_ID)).toBeNull();
    expect(document.querySelector(POPOVER_CONTENT_SELECTOR)?.className).toContain("w-auto");
  });

  it("keeps the move options collapsed until the disclosure is opened", async () => {
    renderStepper();

    await openStepActions();
    const trigger = screen.getByTestId(OPTIONS_TRIGGER_ID);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId(OPTIONS_CONTENT_ID)).toBeTruthy();
    expect(screen.getByTestId(INSTRUCTIONS_ID)).toBeTruthy();
    expect(screen.getByTestId(AGENT_PROFILE_ID)).toBeTruthy();
  });

  it("bounds the touch step drawer to one safe-area-aware scroll body", async () => {
    mocks.touchDrawer.enabled = true;
    renderStepper();

    fireEvent.click(screen.getByTestId(STEP_SPEC_ID));
    const drawer = await screen.findByRole("dialog");
    expect(drawer.className).toContain("h-dvh");
    expect(drawer.className).toContain("max-h-dvh");
    expect(drawer.className).toContain("pb-[env(safe-area-inset-bottom,0px)]");

    const trigger = screen.getByTestId(STEP_SPEC_ID);
    expect(trigger.className).toContain("min-h-11");
    expect(trigger.className).toContain("min-w-11");

    const body = screen.getByTestId("workflow-step-drawer-body");
    expect(body.className).toContain("min-h-0");
    expect(body.className).toContain("overflow-y-auto");
    expect(drawer.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
  });

  it("opens the anchored surface from the step trigger", async () => {
    collapsedMock.mockReturnValue(false);
    renderStepper();

    // Pointer-enter must open the shared anchored surface (openDelay=200ms).
    const trigger = screen.getByTestId(STEP_SPEC_ID);
    expect(trigger.tagName).toBe("BUTTON");
    fireEvent.pointerEnter(trigger);
    await waitFor(() => expect(screen.getByRole("button", { name: /move here/i })).toBeTruthy(), {
      timeout: 2_000,
    });
  });
});

describe("WorkflowStepper — move behaviour", () => {
  it("cleans up plan mode only after a move succeeds", async () => {
    let resolveMove!: (result: unknown) => void;
    mocks.workflowMove.move.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveMove = resolve;
      }),
    );
    renderStepper();

    await openStepActions();
    fireEvent.click(screen.getByRole("button", { name: /move here/i }));
    await waitFor(() => expect(mocks.workflowMove.move).toHaveBeenCalledOnce());
    expect(mocks.plan.setPlanMode).not.toHaveBeenCalled();

    resolveMove({ disposition: "committed", response: {} });
    await waitFor(() => expect(mocks.plan.setPlanMode).toHaveBeenCalledWith("session-1", false));
  });

  it("shows a localized error toast and keeps plan mode on a failed move", async () => {
    mocks.workflowMove.move.mockResolvedValueOnce({
      disposition: "failed",
      error: new Error("step is at its WIP limit"),
    });
    renderStepper();

    await openStepActions();
    fireEvent.click(screen.getByRole("button", { name: /move here/i }));

    await waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith({
        title: "Failed to move task",
        description: "step is at its WIP limit",
        variant: "error",
      }),
    );
    expect(mocks.plan.setPlanMode).not.toHaveBeenCalled();
  });

  it("keeps Move here a direct move while the options disclosure stays untouched", async () => {
    renderStepper();

    await openStepActions();
    fireEvent.click(screen.getByTestId(MOVE_HERE_ID));

    await waitFor(() => expect(mocks.workflowMove.move).toHaveBeenCalledOnce());
    const payload = mocks.workflowMove.move.mock.calls[0][1];
    expect(payload.entry_options).toBeUndefined();
  });

  it("submits the visible anchored-form draft as one-shot entry options", async () => {
    renderStepper();

    await openStepActions();
    fireEvent.click(screen.getByTestId(OPTIONS_TRIGGER_ID));
    const instructions = await screen.findByTestId(INSTRUCTIONS_ID);
    fireEvent.change(instructions, { target: { value: "start the review with tests" } });
    fireEvent.click(screen.getByTestId(MOVE_HERE_ID));

    await waitFor(() => expect(mocks.workflowMove.move).toHaveBeenCalledOnce());
    const payload = mocks.workflowMove.move.mock.calls[0][1];
    expect(payload.entry_options).toEqual({ instructions: "start the review with tests" });
  });

  it("keeps option values and the popover open when an optioned move fails", async () => {
    mocks.workflowMove.move.mockResolvedValueOnce({
      disposition: "failed",
      error: new Error("step is at its WIP limit"),
    });
    renderStepper();

    await openStepActions();
    fireEvent.click(screen.getByTestId(OPTIONS_TRIGGER_ID));
    const instructions = await screen.findByTestId(INSTRUCTIONS_ID);
    fireEvent.change(instructions, { target: { value: "retry after capacity opens" } });
    fireEvent.click(screen.getByTestId(MOVE_HERE_ID));

    await waitFor(() => expect(mocks.toast).toHaveBeenCalled());
    expect(screen.getByTestId(MOVE_HERE_ID)).toBeTruthy();
    expect((screen.getByTestId(INSTRUCTIONS_ID) as HTMLTextAreaElement).value).toBe(
      "retry after capacity opens",
    );
  });
});

describe("WorkflowStepper — anchored options", () => {
  it("pins the surface after interacting with the form", async () => {
    vi.useFakeTimers();
    try {
      render(
        <WorkflowStepper
          steps={STEPS}
          currentStepId={CURRENT_STEP_ID}
          taskId={TASK_ID}
          workflowId={WORKFLOW_ID}
        />,
      );
      const trigger = screen.getByTestId(STEP_SPEC_ID);
      fireEvent.pointerEnter(trigger);
      act(() => vi.advanceTimersByTime(250));
      fireEvent.click(screen.getByTestId(OPTIONS_TRIGGER_ID));
      fireEvent.pointerDown(screen.getByTestId(AGENT_PROFILE_ID));
      act(() => vi.advanceTimersByTime(250));

      // Pointer travel away from the surface must not dismiss it mid-entry.
      fireEvent.pointerLeave(screen.getByTestId("workflow-step-popover"), {
        relatedTarget: document.body,
      });
      act(() => vi.advanceTimersByTime(1_000));
      expect(screen.getByTestId(AGENT_PROFILE_ID)).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows the archived badge instead of a step when collapsed and archived", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId={CURRENT_STEP_ID} isArchived />);

    expect(screen.getByText("Archived")).toBeTruthy();
    // Archived badge carries the minimal test id for collapsed-mode detection.
    expect(screen.getByTestId(MINIMAL_ID)).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-Work")).toBeNull();
  });

  it("renders nothing when there are no steps", () => {
    collapsedMock.mockReturnValue(false);
    const { container } = render(<WorkflowStepper steps={[]} currentStepId={null} />);
    expect(container.innerHTML).toBe("");
  });
});
