import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkflowMoveOptions } from "./workflow-move-options";

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
  touchMocks.enabled = false;
});

function renderOptions(onSubmit = vi.fn(), onOpenChange = vi.fn()) {
  render(
    <WorkflowMoveOptions
      open
      onOpenChange={onOpenChange}
      targetStepName="Review"
      onSubmit={onSubmit}
    />,
  );
  return { onSubmit, onOpenChange };
}

describe("WorkflowMoveOptions", () => {
  it("exposes the profile override without a model control", () => {
    renderOptions();

    expect(screen.getByTestId("workflow-move-agent-profile")).toBeTruthy();
    expect(screen.getByTestId("workflow-move-reset-context")).toBeTruthy();
    expect(screen.getByTestId("workflow-move-instructions")).toBeTruthy();
    expect(screen.queryByTestId("workflow-move-model")).toBeNull();
  });

  it("submits the draft payload and keeps values when the move fails", async () => {
    const onSubmit = vi.fn().mockResolvedValue(false);
    renderOptions(onSubmit);

    fireEvent.change(screen.getByTestId("workflow-move-instructions"), {
      target: { value: "  create the PR ready for review  " },
    });
    fireEvent.click(screen.getByTestId("workflow-move-submit"));
    await act(async () => {});

    expect(onSubmit).toHaveBeenCalledWith({
      instructions: "create the PR ready for review",
    });
    expect((screen.getByTestId("workflow-move-instructions") as HTMLTextAreaElement).value).toBe(
      "  create the PR ready for review  ",
    );
  });

  it("closes through the cancel action", () => {
    const { onOpenChange } = renderOptions();

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("uses the Drawer presentation on touch surfaces", () => {
    touchMocks.enabled = true;
    renderOptions();

    expect(screen.getByTestId("workflow-move-options")).toBeTruthy();
    expect(document.querySelector('[data-slot="drawer-content"]')).toBeTruthy();
  });

  it("uses the Dialog presentation on fine-pointer surfaces", () => {
    renderOptions();

    expect(screen.getByTestId("workflow-move-options")).toBeTruthy();
    expect(document.querySelector('[data-slot="dialog-content"]')).toBeTruthy();
    expect(document.querySelector('[data-slot="drawer-content"]')).toBeNull();
  });
});
