import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import {
  hasCausalReplaySettlement,
  replaySettlementSessionId,
} from "./workflow-step-move-overrides-replay";

export const MOVE_INSTRUCTIONS = "Reproduce the checkout failure before editing.";
export type MoveOverrideFixture = {
  taskId: string;
  workflowId: string;
  sourceStepId: string;
  targetStepId: string;
  profileId: string;
  targetSessionId: string;
  session: SessionPage;
};

export async function seedMoveTargetSession(
  apiClient: ApiClient,
  taskId: string,
  profileId: string,
): Promise<string> {
  const targetSession = await apiClient.seedTaskSession(taskId, {
    state: "IDLE",
    agentProfileId: profileId,
    metadata: {
      context_window: {
        size: 200_000,
        used: 190_000,
        remaining: 10_000,
      },
    },
  });
  return targetSession.session_id;
}

export async function seedMoveOverrideFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
): Promise<MoveOverrideFixture> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.name === "mock-agent");
  if (!agent) throw new Error("E2E mock agent is required for workflow move override coverage");

  const profile = await apiClient.createAgentProfile(agent.id, `${name} Profile`, {
    model: "mock-fast",
  });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} Workflow`);
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Spec", 0, {
    is_start_step: true,
  });
  const targetStep = await apiClient.createWorkflowStep(workflow.id, "Verify", 1, {
    auto_advance_requires_signal: true,
  });
  await apiClient.updateWorkflowStep(sourceStep.id, {
    prompt: 'e2e:message("spec ready")\n{{task_prompt}}',
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(targetStep.id, {
    prompt: 'e2e:message("verification started")\n{{task_prompt}}',
    events: { on_enter: [{ type: "auto_start_agent" }] },
    auto_advance_requires_signal: true,
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `${name} Task`,
    seedData.agentProfileId,
    {
      workflow_id: workflow.id,
      workflow_step_id: sourceStep.id,
      repository_ids: [seedData.repositoryId],
    },
  );
  const targetSessionId = await seedMoveTargetSession(apiClient, task.id, profile.id);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return {
    taskId: task.id,
    workflowId: workflow.id,
    sourceStepId: sourceStep.id,
    targetStepId: targetStep.id,
    profileId: profile.id,
    targetSessionId,
    session,
  };
}

export async function fillMoveOverrides(
  page: Page,
  profileId: string,
  options: { instructions?: string } = {},
): Promise<void> {
  await page.getByTestId("workflow-move-reset-context").click();
  await page
    .getByTestId("workflow-move-instructions")
    .fill(options.instructions ?? MOVE_INSTRUCTIONS);
  await page.getByTestId("workflow-move-agent-profile").click();
  await page.getByTestId(`workflow-move-profile-option-${profileId}`).click();
}

export function waitForMoveRequest(page: Page, taskId: string) {
  return page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      new URL(request.url()).pathname === `/api/v1/tasks/${taskId}/move`,
  );
}

export type WorkflowMoveReplayBoundary = {
  taskId: string;
  workflowId: string;
  expectedInstruction: string;
  /** Destination session whose one-shot prompt must remain exactly once. */
  targetSessionId?: string;
  /**
   * The isolated backend restart is the causal replay boundary. Service.Start
   * drains persisted workflow lifecycle markers before /health becomes ready.
   */
  backend: { restart: () => Promise<void>; ensureReady: () => Promise<void> };
};

/**
 * Observe one transition, cross the isolated-backend restart boundary, let the
 * destination session settle, and verify the history and target instruction
 * are still exactly one entry. The restart is the causal replay boundary; the
 * browser must not manufacture a second public move to simulate delivery.
 */
export async function assertSingleTransitionAcrossRestartReplay(
  apiClient: ApiClient,
  sessionId: string,
  targetStepId: string,
  boundary: WorkflowMoveReplayBoundary,
  timeout = 30_000,
): Promise<void> {
  const transitionCount = async () => {
    const { history } = await apiClient.listWorkflowHistory(sessionId);
    return history.filter((entry) => entry.to_step_id === targetStepId).length;
  };

  await expect
    .poll(transitionCount, {
      timeout,
      message: `waiting for exactly one transition to ${targetStepId} in session ${sessionId}`,
    })
    .toBe(1);

  await boundary.backend.restart();
  await boundary.backend.ensureReady();
  const backendRestarted = true;
  const settlementSessionId = replaySettlementSessionId(boundary, sessionId);
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(boundary.taskId);
        return hasCausalReplaySettlement({
          state: sessions.find((session) => session.id === settlementSessionId)?.state ?? "",
          backendRestarted,
        });
      },
      {
        timeout,
        message: `waiting for target session ${settlementSessionId} to settle after replay boundary`,
      },
    )
    .toBe(true);

  await expect
    .poll(transitionCount, {
      timeout,
      message: `move replay duplicated transition to ${targetStepId} in session ${sessionId}`,
    })
    .toBe(1);

  const instructionCount = async () => {
    const { sessions } = await apiClient.listTaskSessions(boundary.taskId);
    const targetSession = sessions.find((session) => session.id === boundary.targetSessionId);
    if (!targetSession || !boundary.expectedInstruction) return 0;
    const { messages } = await apiClient.listSessionMessages(targetSession.id);
    return messages.filter((message) => message.content.includes(boundary.expectedInstruction))
      .length;
  };

  await expect
    .poll(instructionCount, {
      timeout,
      message: `waiting for one replay-safe instruction in target session ${boundary.targetSessionId ?? ""}`,
    })
    .toBe(1);
}
