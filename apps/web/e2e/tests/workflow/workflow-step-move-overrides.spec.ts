import { expect, test } from "../../fixtures/test-base";
import {
  fillMoveOverrides,
  MOVE_INSTRUCTIONS,
  seedMoveTargetSession,
  seedMoveOverrideFixture,
  assertSingleTransitionAcrossRestartReplay,
  waitForMoveRequest,
} from "./workflow-step-move-overrides-helpers";

test("moves immediately with one-shot options from the desktop stepper", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Move Override",
  );
  const defaultsBefore = (await apiClient.listWorkflowSteps(fixture.workflowId)).steps.find(
    (step) => step.id === fixture.targetStepId,
  );
  expect(defaultsBefore).toBeDefined();
  const sourceSessionId = (await apiClient.getTask(fixture.taskId)).primary_session_id;
  expect(sourceSessionId).toBeTruthy();
  const target = testPage.getByTestId("workflow-step-Verify");
  await target.hover();

  // Fine-pointer hover opens the compact anchored surface; the one-shot fields are opt-in.
  await expect(testPage.getByTestId("workflow-step-popover")).toBeVisible();
  await testPage.getByTestId("workflow-step-move-options-trigger").click();
  await expect(testPage.getByTestId("workflow-move-agent-profile")).toBeVisible();
  await fillMoveOverrides(testPage, fixture.profileId);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-step-move-here").click();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
      agent_profile_id: fixture.profileId,
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  await expect(
    fixture.session.chat.getByText(MOVE_INSTRUCTIONS, { exact: false }).first(),
  ).toBeVisible({ timeout: 30_000 });
  const targetSessionId = await waitForMoveEffects({
    apiClient,
    taskId: fixture.taskId,
    profileId: fixture.profileId,
    instructions: MOVE_INSTRUCTIONS,
    expectedTargetSessionId: fixture.targetSessionId,
  });
  const defaultsAfter = (await apiClient.listWorkflowSteps(fixture.workflowId)).steps.find(
    (step) => step.id === fixture.targetStepId,
  );
  expect(defaultsAfter).toEqual(defaultsBefore);
  const targetMessages = await apiClient.listSessionMessages(targetSessionId);
  expect(
    targetMessages.messages.filter((message) => message.content.includes(MOVE_INSTRUCTIONS)),
  ).toHaveLength(1);
  await assertSingleTransitionAcrossRestartReplay(
    apiClient,
    sourceSessionId!,
    fixture.targetStepId,
    {
      taskId: fixture.taskId,
      workflowId: fixture.workflowId,
      targetSessionId,
      expectedInstruction: MOVE_INSTRUCTIONS,
      backend,
    },
  );
});

test("uses the desktop next-step anchored form for the same one-shot move contract", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Sidecar Move Override",
  );
  const sourceSessionId = (await apiClient.getTask(fixture.taskId)).primary_session_id;
  expect(sourceSessionId).toBeTruthy();
  const nextStepButton = testPage.getByTestId("proceed-next-step");
  await expect(nextStepButton).toBeVisible();
  await nextStepButton.hover();
  await expect(testPage.getByTestId("proceed-next-step-options")).toBeVisible();
  await fillMoveOverrides(testPage, fixture.profileId);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-move-submit").click();

  expect((await moveRequest).postDataJSON()).toMatchObject({
    workflow_step_id: fixture.targetStepId,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
      agent_profile_id: fixture.profileId,
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  const targetSessionId = await waitForMoveEffects({
    apiClient,
    taskId: fixture.taskId,
    profileId: fixture.profileId,
    instructions: MOVE_INSTRUCTIONS,
    expectedTargetSessionId: fixture.targetSessionId,
  });
  await assertSingleTransitionAcrossRestartReplay(
    apiClient,
    sourceSessionId!,
    fixture.targetStepId,
    {
      taskId: fixture.taskId,
      workflowId: fixture.workflowId,
      targetSessionId,
      expectedInstruction: MOVE_INSTRUCTIONS,
      backend,
    },
  );
});

type JsonRecord = Record<string, unknown>;

type DeferredMoveExpectation = {
  taskId: string;
  workflowId: string;
  targetStepId: string;
  profileId: string;
  instructions: string;
};

function asRecord(value: unknown): JsonRecord | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as JsonRecord)
    : undefined;
}

function parseRecord(value: unknown): JsonRecord | undefined {
  if (typeof value === "string") {
    try {
      return asRecord(JSON.parse(value) as unknown);
    } catch {
      return undefined;
    }
  }
  return asRecord(value);
}

function deferredOutput(value: unknown): JsonRecord | undefined {
  const direct = parseRecord(value);
  if (!direct) return undefined;
  if (direct.disposition === "deferred") return direct;
  return (
    parseRecord(direct.result) ??
    parseRecord(direct.output) ??
    parseRecord(asRecord(direct.rawOutput)?.output)
  );
}

function completedMoveToolCall(message: {
  type?: string;
  content?: string;
  metadata?: JsonRecord;
}) {
  if (message.type !== "tool_call") return undefined;
  const metadata = message.metadata;
  if (!metadata?.tool_call_id || !["complete", "completed"].includes(String(metadata.status))) {
    return undefined;
  }
  const generic = parseRecord(parseRecord(metadata.normalized)?.generic);
  const toolName = generic?.name ?? message.content ?? metadata.title;
  return toolName === "move_task_kandev" || message.content === "move_task_kandev"
    ? generic
    : undefined;
}

function matchesDeferredMove(generic: JsonRecord, expected: DeferredMoveExpectation): boolean {
  const input = parseRecord(generic.input);
  const normalizedInput = parseRecord(input?.raw_input) ?? input;
  const entryOptions = parseRecord(normalizedInput?.entry_options);
  return (
    normalizedInput?.task_id === expected.taskId &&
    normalizedInput?.workflow_id === expected.workflowId &&
    normalizedInput?.workflow_step_id === expected.targetStepId &&
    entryOptions?.reset_context === true &&
    entryOptions.instructions === expected.instructions &&
    entryOptions.agent_profile_id === expected.profileId
  );
}

function deferredMoveResult(
  message: { type?: string; content?: string; metadata?: JsonRecord },
  expected?: DeferredMoveExpectation,
): boolean {
  const generic = completedMoveToolCall(message);
  if (!generic || deferredOutput(generic.output)?.disposition !== "deferred") return false;
  return expected ? matchesDeferredMove(generic, expected) : true;
}

type MoveEffectsExpectation = {
  apiClient: Parameters<typeof seedMoveOverrideFixture>[1];
  taskId: string;
  profileId: string;
  instructions: string;
  expectedTargetSessionId?: string;
  timeout?: number;
};

const SETTLED_SESSION_STATES = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

// eslint-disable-next-line complexity -- this poll asserts the complete durable hand-off contract in one causal observation.
async function waitForMoveEffects({
  apiClient,
  taskId,
  profileId,
  instructions,
  expectedTargetSessionId,
  timeout = 45_000,
}: MoveEffectsExpectation): Promise<string> {
  let sessionId = "";
  let settledObservationKey = "";
  let settledObservationCount = 0;
  await expect
    .poll(
      // eslint-disable-next-line complexity -- one poll observes the durable session, prompt, and reset invariants together.
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        sessionId = sessions.find((session) => session.agent_profile_id === profileId)?.id ?? "";
        if (!sessionId) return false;
        const { messages } = await apiClient.listSessionMessages(sessionId);
        const session = sessions.find((candidate) => candidate.id === sessionId);
        const { turns } = await apiClient.listSessionTurns(sessionId);
        const metadata = session?.metadata ?? {};
        const settled =
          SETTLED_SESSION_STATES.has(session?.state ?? "") &&
          turns.every((turn) => Boolean(turn.completed_at));
        const nextSettledObservationKey = `${sessionId}:${session?.state ?? ""}`;
        if (settled) {
          settledObservationCount =
            settledObservationKey === nextSettledObservationKey ? settledObservationCount + 1 : 1;
          settledObservationKey = nextSettledObservationKey;
        } else {
          settledObservationCount = 0;
          settledObservationKey = "";
        }
        return {
          profileSessionCount: sessions.filter(
            (candidate) => candidate.agent_profile_id === profileId,
          ).length,
          instructions: messages.filter((message) => message.content.includes(instructions)).length,
          agentResponse: messages.some(
            (message) => message.author_type === "agent" && message.type !== "tool_call",
          ),
          freshRuntimeContext: messages.filter((message) =>
            (message.raw_content ?? message.content).includes("step_complete_kandev"),
          ).length,
          resetContext:
            (!expectedTargetSessionId || sessionId === expectedTargetSessionId) &&
            Object.prototype.hasOwnProperty.call(metadata, "context_window") &&
            metadata.context_window === null,
          settled: settled && settledObservationCount >= 2,
        };
      },
      {
        timeout,
        message: `waiting for one move hand-off on profile ${profileId}`,
      },
    )
    .toEqual({
      profileSessionCount: 1,
      instructions: 1,
      agentResponse: true,
      freshRuntimeContext: 1,
      resetContext: true,
      settled: true,
    });
  return sessionId;
}

async function waitForAgentMoveCall(
  apiClient: Parameters<typeof seedMoveOverrideFixture>[1],
  sessionId: string,
  expected: DeferredMoveExpectation,
  timeout = 30_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((message) => deferredMoveResult(message, expected));
      },
      {
        timeout,
        message:
          "agent did not complete a structured deferred move_task_kandev result before restart",
      },
    )
    .toBe(true);
}

test("keeps all one-shot fields through an active source turn and delivers the hand-off once", async ({
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(90_000);
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent is required for workflow move override coverage");
  const profile = await apiClient.createAgentProfile(mockAgent.id, "Deferred Move Profile", {
    model: "mock-fast",
  });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Deferred Move Workflow");
  const source = await apiClient.createWorkflowStep(workflow.id, "Working", 0, {
    is_start_step: true,
  });
  const target = await apiClient.createWorkflowStep(workflow.id, "Review", 1, {
    auto_advance_requires_signal: true,
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(target.id, {
    prompt: 'e2e:message("target workflow prompt")',
    agent_profile_id: seedData.agentProfileId,
    auto_advance_requires_signal: true,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Deferred Move Task",
    seedData.agentProfileId,
    {
      description: [
        "e2e:delay(1000)",
        `e2e:mcp:kandev:move_task_kandev({"task_id":"{task_id}","workflow_id":"${workflow.id}","workflow_step_id":"${target.id}","entry_options":{"reset_context":true,"instructions":"${MOVE_INSTRUCTIONS}","agent_profile_id":"${profile.id}"}})`,
        'e2e:message("source turn complete")',
      ].join("\n"),
      workflow_id: workflow.id,
      workflow_step_id: source.id,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("active-turn fixture did not create a session");
  const targetSessionId = await seedMoveTargetSession(apiClient, task.id, profile.id);

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === task.session_id)?.state ?? "";
      },
      { timeout: 20_000, message: "source agent did not enter an active turn" },
    )
    .toMatch(/^(STARTING|RUNNING)$/);

  await waitForAgentMoveCall(apiClient, task.session_id, {
    taskId: task.id,
    workflowId: workflow.id,
    targetStepId: target.id,
    profileId: profile.id,
    instructions: MOVE_INSTRUCTIONS,
  });
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
      timeout: 20_000,
      message: "active-turn move did not commit its target step",
    })
    .toBe(target.id);
  const sessionId = await waitForMoveEffects({
    apiClient,
    taskId: task.id,
    profileId: profile.id,
    instructions: MOVE_INSTRUCTIONS,
    expectedTargetSessionId: targetSessionId,
  });
  const { messages } = await apiClient.listSessionMessages(sessionId);
  expect(messages.filter((message) => message.content.includes(MOVE_INSTRUCTIONS))).toHaveLength(1);
  await assertSingleTransitionAcrossRestartReplay(apiClient, task.session_id, target.id, {
    taskId: task.id,
    workflowId: workflow.id,
    targetSessionId: sessionId,
    expectedInstruction: MOVE_INSTRUCTIONS,
    backend,
  });
});

test("recovers a deferred agent move after backend restart without duplicating its prompt", async ({
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent is required for workflow move override coverage");
  const profile = await apiClient.createAgentProfile(mockAgent.id, "Restart Move Profile", {
    model: "mock-fast",
  });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Restart Move Workflow");
  const source = await apiClient.createWorkflowStep(workflow.id, "Working", 0, {
    is_start_step: true,
  });
  const target = await apiClient.createWorkflowStep(workflow.id, "Review", 1, {
    auto_advance_requires_signal: true,
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(target.id, {
    prompt: 'e2e:message("restart target prompt")',
    agent_profile_id: seedData.agentProfileId,
    auto_advance_requires_signal: true,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Restart Move Task",
    seedData.agentProfileId,
    {
      description: [
        "e2e:delay(1000)",
        `e2e:mcp:kandev:move_task_kandev({"task_id":"{task_id}","workflow_id":"${workflow.id}","workflow_step_id":"${target.id}","entry_options":{"reset_context":true,"instructions":"${MOVE_INSTRUCTIONS}","agent_profile_id":"${profile.id}"}})`,
        "e2e:delay(20000)",
      ].join("\n"),
      workflow_id: workflow.id,
      workflow_step_id: source.id,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("restart fixture did not create a session");
  const targetSessionId = await seedMoveTargetSession(apiClient, task.id, profile.id);
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === task.session_id)?.state ?? "";
      },
      { timeout: 20_000, message: "restart fixture source did not enter an active turn" },
    )
    .toMatch(/^(STARTING|RUNNING)$/);

  await waitForAgentMoveCall(apiClient, task.session_id, {
    taskId: task.id,
    workflowId: workflow.id,
    targetStepId: target.id,
    profileId: profile.id,
    instructions: MOVE_INSTRUCTIONS,
  });
  await backend.restart();
  await backend.ensureReady();

  await expect
    .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
      timeout: 45_000,
      message: "pending move did not recover to its target step after restart",
    })
    .toBe(target.id);
  const sessionId = await waitForMoveEffects({
    apiClient,
    taskId: task.id,
    profileId: profile.id,
    instructions: MOVE_INSTRUCTIONS,
    expectedTargetSessionId: targetSessionId,
  });
  const { messages } = await apiClient.listSessionMessages(sessionId);
  expect(messages.filter((message) => message.content.includes(MOVE_INSTRUCTIONS))).toHaveLength(1);
  const { history } = await apiClient.listWorkflowHistory(task.session_id);
  expect(history.filter((entry) => entry.to_step_id === target.id)).toHaveLength(1);
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const replayTarget = sessions.find((candidate) => candidate.id === sessionId);
  expect(replayTarget).toBeDefined();
  const replayMessages = await apiClient.listSessionMessages(sessionId);
  expect(
    replayMessages.messages.filter((message) => message.content.includes(MOVE_INSTRUCTIONS)),
  ).toHaveLength(1);
});

test("preserves move options while WIP-queued and applies them exactly once on promotion", async ({
  apiClient,
  seedData,
}) => {
  test.setTimeout(90_000);
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent is required for workflow move override coverage");
  const profile = await apiClient.createAgentProfile(mockAgent.id, "WIP Move Profile", {
    model: "mock-fast",
  });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "WIP Move Workflow");
  const source = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0, {
    is_start_step: true,
  });
  const target = await apiClient.createWorkflowStep(workflow.id, "Work", 1, {
    auto_advance_requires_signal: true,
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const release = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
  await apiClient.updateWorkflowStep(target.id, {
    wip_limit: 1,
    prompt: 'e2e:message("wip target prompt")',
    agent_profile_id: seedData.agentProfileId,
  });
  const occupant = await apiClient.createTask(seedData.workspaceId, "WIP Occupant", {
    workflow_id: workflow.id,
    workflow_step_id: target.id,
    repository_ids: [seedData.repositoryId],
  });
  const queued = await apiClient.createTask(seedData.workspaceId, "WIP Queued Move", {
    workflow_id: workflow.id,
    workflow_step_id: source.id,
    repository_ids: [seedData.repositoryId],
  });
  await apiClient.seedTaskSession(queued.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
  });
  await apiClient.moveTask(queued.id, workflow.id, target.id, {
    reset_context: true,
    instructions: MOVE_INSTRUCTIONS,
    agent_profile_id: profile.id,
  });
  await expect
    .poll(async () => {
      const task = await apiClient.getTask(queued.id);
      return {
        step: task.workflow_step_id,
        admitted: task.wip_admitted,
        queued: task.queued_for_step_id ?? "",
      };
    })
    .toEqual({ step: target.id, admitted: false, queued: target.id });

  await apiClient.moveTask(occupant.id, workflow.id, release.id);
  await expect
    .poll(
      async () => {
        const task = await apiClient.getTask(queued.id);
        return {
          step: task.workflow_step_id,
          admitted: task.wip_admitted,
          queued: task.queued_for_step_id ?? "",
        };
      },
      { timeout: 30_000, message: "queued task was not promoted after WIP capacity opened" },
    )
    .toEqual({ step: target.id, admitted: true, queued: "" });

  const sessionId = await waitForMoveEffects({
    apiClient,
    taskId: queued.id,
    profileId: profile.id,
    instructions: MOVE_INSTRUCTIONS,
  });
  const { messages } = await apiClient.listSessionMessages(sessionId);
  expect(messages.filter((message) => message.content.includes(MOVE_INSTRUCTIONS))).toHaveLength(1);
});
