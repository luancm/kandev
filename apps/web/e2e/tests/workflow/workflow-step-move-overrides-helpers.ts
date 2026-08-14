import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export const MOVE_INSTRUCTIONS = "Reproduce the checkout failure before editing.";

export type MoveOverrideFixture = {
  taskId: string;
  targetStepId: string;
  profileId: string;
  session: SessionPage;
};

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
  const targetStep = await apiClient.createWorkflowStep(workflow.id, "Verify", 1);
  await apiClient.updateWorkflowStep(sourceStep.id, {
    prompt: 'e2e:message("spec ready")\n{{task_prompt}}',
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(targetStep.id, {
    prompt: 'e2e:message("verification started")\n{{task_prompt}}',
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });

  const task = await apiClient.createTask(seedData.workspaceId, `${name} Task`, {
    workflow_id: workflow.id,
    workflow_step_id: sourceStep.id,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return {
    taskId: task.id,
    targetStepId: targetStep.id,
    profileId: profile.id,
    session,
  };
}

export async function fillMoveOverrides(page: Page, profileId: string): Promise<void> {
  await page.getByTestId("workflow-move-reset-context").click();
  await page.getByTestId("workflow-move-instructions").fill(MOVE_INSTRUCTIONS);
  await page.getByTestId("workflow-move-agent-profile").click();
  await page.getByTestId(`workflow-move-profile-option-${profileId}`).click();
  await page.getByTestId("workflow-move-model").click();
  await page.getByTestId("workflow-move-model-option-mock-smart").click();
}

export function waitForMoveRequest(page: Page, taskId: string) {
  return page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      new URL(request.url()).pathname === `/api/v1/tasks/${taskId}/move`,
  );
}
