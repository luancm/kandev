import { expect, type Page, type Locator } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { Errand } from "../../lib/types/errands";
import { mockTurnGate } from "./mock-turn-gate";

export async function listErrands(api: ApiClient, taskId: string): Promise<Errand[]> {
  return (await api.wsRequest<{ errands: Errand[] }>("errand.list", { task_id: taskId })).errands;
}

export function parentChat(page: Page, sessionId: string) {
  return page
    .locator(`[data-testid="session-chat"][data-session-id="${sessionId}"]:visible`)
    .first();
}

export async function waitParentIdle(api: ApiClient, taskId: string, sessionId: string) {
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(taskId)).sessions.find((session) => session.id === sessionId)
          ?.state,
      { message: "Parent turn settles while errands remain independent", timeout: 60_000 },
    )
    .toBe("WAITING_FOR_INPUT");
}

export async function startErrands(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  options: { count?: number; await?: boolean } = {},
) {
  const count = options.count ?? 2;
  const gates = Array.from({ length: count }, (_, index) =>
    mockTurnGate(backend.tmpDir, `ERRAND_RESULT_${index + 1}`),
  );
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Errands daily-use proof",
    seed.agentProfileId,
    {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
      start_agent: false,
    },
  );
  const request = JSON.stringify({
    profile: seed.agentProfileId,
    prompts: gates.map((gate) => gate.prompt),
  });
  const launched = await api.launchSession({
    task_id: task.id,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    prompt: `/e2e:errands${options.await ? "-await" : ""} ${request}`,
  });
  await expect
    .poll(async () => (await listErrands(api, task.id)).length, {
      timeout: 60_000,
      message: "Errands admitted through real MCP start responses",
    })
    .toBe(Math.min(count, 5));
  if (!options.await) await waitParentIdle(api, task.id, launched.session_id);
  await api.setQueueAutoRun(await api.getQueueSessionIdentity(task.id, launched.session_id), true);
  const errands = await listErrands(api, task.id);
  return { taskId: task.id, parentId: launched.session_id, errands, gates };
}

export async function waitDelivered(api: ApiClient, taskId: string, ids: string[]) {
  await expect
    .poll(
      async () =>
        (await listErrands(api, taskId))
          .filter((row) => ids.includes(row.errand_id))
          .map((row) => row.result_state),
      { timeout: 60_000, message: "Settled errand results reach their parent" },
    )
    .toEqual(ids.map(() => "delivered"));
}

export async function deliverySnapshots(api: ApiClient, parentId: string) {
  return (await api.listSessionMessages(parentId)).messages.flatMap((message) => {
    const envelope = message.metadata?.errand_delivery as { items?: unknown[] } | undefined;
    return envelope?.items ?? [];
  });
}

export async function expectTouchTarget(control: Locator) {
  await expect
    .poll(async () => {
      const box = await control.boundingBox();
      return box !== null && box.height >= 44 && box.width >= 44;
    })
    .toBe(true);
}

export async function expectNoHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
