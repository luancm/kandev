import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption, AvailableAgentsState } from "@/lib/state/slices/settings/types";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowMoveOptions } from "./workflow-move-options";

const touchMocks = vi.hoisted(() => ({ enabled: false }));

const { QA_PROFILE_ID, MODEL_ID, MODEL_NAME } = vi.hoisted(() => ({
  QA_PROFILE_ID: "qa-profile",
  MODEL_ID: "gpt-5.6-sol",
  MODEL_NAME: "GPT-5.6 Sol",
}));
const PROFILE_TEST_ID = "workflow-move-agent-profile";
const INSTRUCTIONS_TEST_ID = "workflow-move-instructions";
const SUBMIT_TEST_ID = "workflow-move-submit";

type WorkflowMoveTestState = {
  agentProfiles: { items: AgentProfileOption[] };
  availableAgents: AvailableAgentsState;
};

const storeMocks = vi.hoisted(() => ({
  state: {
    agentProfiles: {
      items: [
        {
          id: QA_PROFILE_ID,
          label: "QA",
          agent_id: "agent-1",
          agent_name: "codex",
          cli_passthrough: false,
          enabled: true,
          capability_status: "ok",
        },
      ],
    },
    availableAgents: {
      items: [
        {
          name: "codex",
          display_name: "Codex",
          available: true,
          supports_mcp: true,
          installation_paths: [],
          capabilities: {
            supports_session_resume: true,
            supports_shell: true,
            supports_workspace_only: false,
          },
          model_config: {
            default_model: MODEL_ID,
            available_models: [{ id: MODEL_ID, name: MODEL_NAME }],
            supports_dynamic_models: false,
            status: "ok",
          },
          updated_at: "2026-08-24T00:00:00Z",
        },
      ],
      loading: false,
      loaded: true,
      tools: [],
    },
  } as WorkflowMoveTestState,
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchMocks.enabled,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(storeMocks.state),
}));

afterEach(() => {
  cleanup();
  touchMocks.enabled = false;
});

function renderOptions(onSubmit = vi.fn(), onOpenChange = vi.fn(), touch = true) {
  touchMocks.enabled = touch;
  render(
    <TooltipProvider>
      <WorkflowMoveOptions
        open
        onOpenChange={onOpenChange}
        targetStepName="Review"
        onSubmit={onSubmit}
      />
    </TooltipProvider>,
  );
  return { onSubmit, onOpenChange };
}

describe("WorkflowMoveOptions", () => {
  it("exposes the profile, reset-context, and instruction overrides", () => {
    renderOptions();

    expect(screen.getByTestId(PROFILE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("workflow-move-reset-context")).toBeTruthy();
    expect(screen.getByTestId(INSTRUCTIONS_TEST_ID)).toBeTruthy();
  });

  it("submits the draft payload and keeps values when the move fails", async () => {
    const onSubmit = vi.fn().mockResolvedValue(false);
    renderOptions(onSubmit);

    fireEvent.change(screen.getByTestId(INSTRUCTIONS_TEST_ID), {
      target: { value: "  create the PR ready for review  " },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    expect(onSubmit).toHaveBeenCalledWith({
      instructions: "create the PR ready for review",
    });
    expect((screen.getByTestId(INSTRUCTIONS_TEST_ID) as HTMLTextAreaElement).value).toBe(
      "  create the PR ready for review  ",
    );
  });

  it("includes the selected profile in the one-time payload", async () => {
    const onSubmit = vi.fn().mockResolvedValue(true);
    renderOptions(onSubmit);

    fireEvent.click(screen.getByTestId(PROFILE_TEST_ID));
    fireEvent.click(screen.getByTestId(`workflow-move-profile-option-${QA_PROFILE_ID}`));
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    expect(onSubmit).toHaveBeenCalledWith({ agent_profile_id: QA_PROFILE_ID });
  });
});

describe("WorkflowMoveOptions capability status", () => {
  it("does not expose raw capability errors in the profile status", () => {
    const previousState = storeMocks.state;
    const rawCapabilityError = "provider secret xyz failed at https://provider.invalid/debug";
    storeMocks.state = {
      ...previousState,
      agentProfiles: {
        items: [
          {
            ...previousState.agentProfiles.items[0],
            capability_status: "failed",
            capability_error: rawCapabilityError,
          },
        ],
      },
      availableAgents: {
        ...previousState.availableAgents,
        items: [
          {
            ...previousState.availableAgents.items[0],
            model_config: {
              ...previousState.availableAgents.items[0].model_config,
              error: rawCapabilityError,
            },
          },
        ],
      },
    };

    try {
      renderOptions();
      fireEvent.click(screen.getByTestId(PROFILE_TEST_ID));

      const option = screen
        .getByTestId(`workflow-move-profile-option-${QA_PROFILE_ID}`)
        .closest('[role="option"]');
      expect(option?.getAttribute("aria-disabled")).toBe("true");
      expect(screen.queryByText(rawCapabilityError)).toBeNull();
    } finally {
      storeMocks.state = previousState;
    }
  });
});

describe("WorkflowMoveOptions profile health", () => {
  it("keeps a normalized passthrough profile selectable when its raw model status is not configured", () => {
    const previousState = storeMocks.state;
    storeMocks.state = {
      ...previousState,
      agentProfiles: {
        items: [
          {
            id: "passthrough-profile",
            label: "Passthrough",
            agent_id: "agent-passthrough",
            agent_name: "passthrough-agent",
            cli_passthrough: true,
            enabled: true,
          },
        ],
      },
      availableAgents: {
        ...previousState.availableAgents,
        items: [
          {
            ...previousState.availableAgents.items[0],
            name: "passthrough-agent",
            model_config: {
              ...previousState.availableAgents.items[0].model_config,
              available_models: [],
              status: "not_configured",
            },
          },
        ],
      },
    };
    try {
      renderOptions();
      fireEvent.click(screen.getByTestId(PROFILE_TEST_ID));
      const option = screen.getByTestId("workflow-move-profile-option-passthrough-profile");
      expect(option.closest('[role="option"]')?.getAttribute("aria-disabled")).not.toBe("true");
    } finally {
      storeMocks.state = previousState;
    }
  });
});

describe("WorkflowMoveOptions presentation", () => {
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
    renderOptions(vi.fn(), vi.fn(), false);

    expect(screen.queryByTestId("workflow-move-options")).toBeNull();
    expect(document.querySelector('[data-slot="dialog-content"]')).toBeNull();
    expect(document.querySelector('[data-slot="drawer-content"]')).toBeNull();
  });
});
