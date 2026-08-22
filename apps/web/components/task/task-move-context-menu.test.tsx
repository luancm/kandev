import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { TaskMoveContextMenuItems } from "./task-move-context-menu";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const steps = [
  { id: "step-1", title: "Todo", color: "bg-slate-500" },
  {
    id: "step-2",
    title: "Review",
    color: "bg-blue-500",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  },
];

function renderMoveMenu({
  onMoveToStep = vi.fn(),
  onMoveToStepWithOptions,
}: {
  onMoveToStep?: (stepId: string) => void;
  onMoveToStepWithOptions?: (stepId: string) => void;
} = {}) {
  const menuProps: Parameters<typeof TaskMoveContextMenuItems>[0] = {
    currentWorkflowId: "workflow-1",
    currentStepId: "step-1",
    workflows: [{ id: "workflow-1", name: "Workflow 1" }],
    stepsByWorkflowId: { "workflow-1": steps },
    showSeparator: false,
    onMoveToStep,
    onMoveToStepWithOptions,
  };
  render(
    <ContextMenu>
      <ContextMenuTrigger data-testid="context-trigger">Task</ContextMenuTrigger>
      <ContextMenuContent>
        <TaskMoveContextMenuItems {...menuProps} />
      </ContextMenuContent>
    </ContextMenu>,
  );

  fireEvent.contextMenu(screen.getByTestId("context-trigger"));
  return { onMoveToStep, onMoveToStepWithOptions };
}

async function openSubmenu(testId: string, childTestId: string) {
  fireEvent.pointerMove(screen.getByTestId(testId), { pointerType: "mouse" });
  return screen.findByTestId(childTestId);
}

describe("TaskMoveContextMenuItems", () => {
  it("keeps direct moves on the step row while nesting options under that row", async () => {
    const direct = vi.fn();
    const options = vi.fn();
    renderMoveMenu({ onMoveToStep: direct, onMoveToStepWithOptions: options });

    expect(screen.getByTestId("task-context-move-to")).toBeTruthy();
    expect(screen.queryByTestId("task-context-move-with-options")).toBeNull();

    const directStep = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    fireEvent.pointerDown(directStep, { pointerType: "mouse" });
    fireEvent.click(directStep);

    expect(direct).toHaveBeenCalledOnce();
    expect(direct).toHaveBeenCalledWith("step-2");
    expect(options).not.toHaveBeenCalled();
  });

  it("exposes one localized options action under the hovered step", async () => {
    const direct = vi.fn();
    const options = vi.fn();
    renderMoveMenu({ onMoveToStep: direct, onMoveToStepWithOptions: options });

    const step = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    fireEvent.pointerMove(step, { pointerType: "mouse" });

    const optionsStep = await screen.findByTestId("task-context-step-options-step-2");
    expect(optionsStep.textContent).toMatch(/Move with options/i);
    expect(screen.queryByTestId("task-context-move-with-options")).toBeNull();
    expect(direct).not.toHaveBeenCalled();
    expect(options).not.toHaveBeenCalled();
  });

  it("selects the nested options action without invoking the direct move", async () => {
    const direct = vi.fn();
    const options = vi.fn();
    renderMoveMenu({ onMoveToStep: direct, onMoveToStepWithOptions: options });

    const step = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    fireEvent.pointerMove(step, { pointerType: "mouse" });
    const optionsStep = await screen.findByTestId("task-context-step-options-step-2");
    fireEvent.click(optionsStep);

    expect(options).toHaveBeenCalledOnce();
    expect(options).toHaveBeenCalledWith("step-2");
    expect(direct).not.toHaveBeenCalled();
  });

  it("keeps a touch tap on the step label as a direct move", async () => {
    const direct = vi.fn();
    renderMoveMenu({ onMoveToStep: direct, onMoveToStepWithOptions: vi.fn() });

    const step = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    fireEvent.pointerDown(step, { pointerType: "touch", clientX: 0 });
    fireEvent.click(step, { clientX: 0 });

    expect(direct).toHaveBeenCalledOnce();
    expect(direct).toHaveBeenCalledWith("step-2");
    expect(screen.queryByTestId("task-context-step-options-step-2")).toBeNull();
  });

  it("opens the nested options submenu from the step chevron on touch", async () => {
    renderMoveMenu({ onMoveToStepWithOptions: vi.fn() });

    const step = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    vi.spyOn(step, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 100,
      height: 44,
      top: 0,
      right: 100,
      bottom: 44,
      left: 0,
      toJSON: () => ({}),
    });
    fireEvent.pointerDown(step, { pointerType: "touch", clientX: 96 });
    fireEvent.click(step, { clientX: 96 });

    expect(await screen.findByTestId("task-context-step-options-step-2")).toBeTruthy();
  });

  it("keeps Radix keyboard navigation for the nested options submenu", async () => {
    renderMoveMenu({ onMoveToStepWithOptions: vi.fn() });

    const trigger = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "ArrowRight" });

    expect(await screen.findByTestId("task-context-step-options-step-2")).toBeTruthy();
  });

  it("keeps the current step disabled when options are available", async () => {
    renderMoveMenu({ onMoveToStepWithOptions: vi.fn() });

    const currentStep = await openSubmenu("task-context-move-to", "task-context-step-step-1");

    expect(currentStep.getAttribute("aria-disabled")).toBe("true");
  });

  it("keeps plain direct step items when no options callback is supplied", async () => {
    const direct = vi.fn();
    renderMoveMenu({ onMoveToStep: direct });

    const directStep = await openSubmenu("task-context-move-to", "task-context-step-step-2");
    fireEvent.pointerMove(directStep, { pointerType: "mouse" });
    expect(
      directStep.querySelector('[data-testid="task-context-step-autostart-step-2"]'),
    ).toBeTruthy();
    expect(screen.queryByTestId("task-context-step-options-step-2")).toBeNull();
    fireEvent.click(directStep);
    expect(direct).toHaveBeenCalledWith("step-2");
  });
});
