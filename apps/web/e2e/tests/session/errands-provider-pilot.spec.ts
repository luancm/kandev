import { copyFile, mkdir, chmod, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  deliverySnapshots,
  listErrands,
  parentChat,
  waitDelivered,
  waitParentIdle,
} from "../../helpers/errands";

const selectedModel = process.env.KANDEV_ERRANDS_PILOT_MODEL;

test("selected real Codex profile delivers errands and preserves opt-in rollback", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.skip(!selectedModel, "Requires an explicitly selected real model and isolated auth source");
  test.setTimeout(420_000);
  const authSource = process.env.KANDEV_ERRANDS_PILOT_AUTH;
  if (!authSource)
    throw new Error("KANDEV_ERRANDS_PILOT_AUTH must name the selected Codex auth file");
  const codexBinary = process.env.KANDEV_ERRANDS_PILOT_CODEX ?? "/home/luancm/.local/bin/codex";
  const isolatedHome = join(backend.tmpDir, "errands-pilot-codex");
  await mkdir(isolatedHome, { mode: 0o700 });
  await copyFile(authSource, join(isolatedHome, "auth.json"));
  await chmod(join(isolatedHome, "auth.json"), 0o600);
  let release: (() => Promise<void>) | null = await backend.useEnv({
    KANDEV_MOCK_AGENT: "false",
    KANDEV_FEATURES_ERRANDS: "true",
  });
  const evidence: Record<string, unknown> = {
    provider: "codex-acp",
    model: selectedModel,
    mode: "agent-full-access",
    store: "SQLite",
    revision: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    dirty: execFileSync("git", ["status", "--porcelain"], { encoding: "utf8" }).length > 0,
    cliVersion: execFileSync(codexBinary, ["--version"], { encoding: "utf8" }).trim(),
    startedAt: new Date().toISOString(),
  };
  try {
    const codex = (await api.listAgents()).agents.find((agent) => agent.name === "codex-acp");
    expect(codex, "Real structured Codex family is available").toBeTruthy();
    const profile = await api.createAgentProfile(codex!.id, "Errands selected-provider pilot", {
      model: selectedModel!,
      mode: "agent-full-access",
      cli_passthrough: false,
      require_exact_model: true,
      auto_fallback: false,
      auto_approve: true,
      config_options: { reasoning_effort: "max" },
      env_vars: [
        { key: "CODEX_HOME", value: isolatedHome },
        { key: "CODEX_PATH", value: codexBinary },
        { key: "npm_config_cache", value: join(backend.tmpDir, "pilot-npm") },
      ],
    });
    evidence.profileId = profile.id;
    evidence.executorProfileId = seed.worktreeExecutorProfileId;
    const markers = [
      join(backend.tmpDir, "pilot-child-a.release"),
      join(backend.tmpDir, "pilot-child-b.release"),
    ];
    const objectives = markers.map(
      (marker, index) =>
        `Read-only pilot. Use a shell tool to run: timeout 180 sh -c 'until test -f ${marker}; do sleep 0.2; done'. After the marker exists, reply with exactly ERRAND_PILOT_RESULT_${index + 1}. Do not write files or start other agents, tasks, sessions or errands.`,
    );
    const prompt = `Use start_errand_kandev to start exactly two errands, titled Pilot A and Pilot B, using agent_profile ${profile.id}. Their prompts are ${JSON.stringify(objectives)}. Retain their actual returned IDs. Do not await, join, poll or stop them. End your own turn immediately after both starts, replying PILOT_PARENT_DISPATCHED. Do not complete a workflow step.`;
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Disposable Errands real-provider pilot",
      profile.id,
      {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
        executor_profile_id: seed.worktreeExecutorProfileId,
        start_agent: false,
      },
    );
    const launched = await api.launchSession(
      {
        task_id: task.id,
        agent_profile_id: profile.id,
        executor_profile_id: seed.worktreeExecutorProfileId,
        prompt,
      },
      120_000,
    );
    evidence.taskId = task.id;
    evidence.parentId = launched.session_id;
    await expect
      .poll(async () => (await listErrands(api, task.id)).length, { timeout: 150_000 })
      .toBe(2);
    await waitParentIdle(api, task.id, launched.session_id);
    await api.setQueueAutoRun(
      await api.getQueueSessionIdentity(task.id, launched.session_id),
      true,
    );
    const allErrands = await listErrands(api, task.id);
    const errands = ["Pilot A", "Pilot B"].map((title) => {
      const row = allErrands.find((candidate) => candidate.title === title);
      expect(row, `Started errand ${title}`).toBeTruthy();
      return row!;
    });
    expect(errands.every((row) => row.status === "running")).toBe(true);
    evidence.errandIds = errands.map((row) => row.errand_id);
    await api.addUserMessage(
      task.id,
      launched.session_id,
      "While the errands run, reply PILOT_PARENT_CONVERSATION. Do not await or stop them or complete a workflow step.",
    );
    await expect
      .poll(
        async () =>
          (await api.listSessionMessages(launched.session_id)).messages.some(
            (message) =>
              message.author_type === "agent" &&
              message.content.includes("PILOT_PARENT_CONVERSATION"),
          ),
        { timeout: 90_000 },
      )
      .toBe(true);
    await waitParentIdle(api, task.id, launched.session_id);
    await page.goto(`/t/${task.id}`);
    const chat = parentChat(page, launched.session_id);
    await chat.getByTestId("errands-inline-disclosure").click();
    await chat
      .getByTestId("errands-inline-summary")
      .getByTestId(`errand-open-${errands[0].errand_id}`)
      .click();
    await expect(parentChat(page, errands[0].errand_id).getByTestId("errand-header")).toBeVisible();
    await parentChat(page, errands[0].errand_id).getByTestId("errand-back-to-parent").click();
    await writeFile(markers[0], "release");
    await waitDelivered(api, task.id, [errands[0].errand_id]);
    await waitParentIdle(api, task.id, launched.session_id);
    await expect(chat.getByTestId("errand-finished-preview")).toContainText(
      "ERRAND_PILOT_RESULT_1",
    );
    await api.addUserMessage(
      task.id,
      errands[0].errand_id,
      "Reply PILOT_CHILD_STEERED. Do not use tools, write files, or start any work.",
    );
    await expect
      .poll(
        async () =>
          (await api.listSessionMessages(errands[0].errand_id)).messages.some(
            (message) =>
              message.author_type === "agent" && message.content.includes("PILOT_CHILD_STEERED"),
          ),
        { timeout: 90_000 },
      )
      .toBe(true);
    await waitParentIdle(api, task.id, errands[0].errand_id);
    await api.wsRequest("errand.send_result", { session_id: errands[0].errand_id });
    await expect
      .poll(async () => (await deliverySnapshots(api, launched.session_id)).length, {
        timeout: 60_000,
      })
      .toBe(2);
    await waitParentIdle(api, task.id, launched.session_id);
    const parentMarker = join(backend.tmpDir, "pilot-parent-never-release");
    await api.addUserMessage(
      task.id,
      launched.session_id,
      `Run this shell command now and wait for it: timeout 180 sh -c 'until test -f ${parentMarker}; do sleep 0.2; done'. Do not stop the errand.`,
    );
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(task.id)).sessions.find(
            (row) => row.id === launched.session_id,
          )?.state,
        { timeout: 60_000 },
      )
      .toBe("RUNNING");
    await api.wsRequest(
      "agent.cancel",
      { task_id: task.id, session_id: launched.session_id },
      40_000,
    );
    await waitParentIdle(api, task.id, launched.session_id);
    await writeFile(parentMarker, "release after cancellation");
    expect(
      (await listErrands(api, task.id)).find((row) => row.errand_id === errands[1].errand_id)
        ?.status,
    ).toBe("running");
    await api.wsRequest("errand.cancel", { session_id: errands[1].errand_id });
    await expect
      .poll(
        async () =>
          (await listErrands(api, task.id)).find((row) => row.errand_id === errands[1].errand_id)
            ?.status,
      )
      .toBe("cancelled");
    await waitDelivered(api, task.id, [errands[1].errand_id]);
    await waitParentIdle(api, task.id, launched.session_id);
    await api.addUserMessage(
      task.id,
      launched.session_id,
      "Acknowledge the errand results with PILOT_RESULTS_RECEIVED. Do not start or stop any work or complete a workflow step.",
    );
    await waitDelivered(
      api,
      task.id,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, task.id, launched.session_id);
    evidence.snapshots = await deliverySnapshots(api, launched.session_id);
    await release();
    release = null;
    await page.reload();
    await expect(page.getByTestId("errands-inline-summary")).toHaveCount(0);
    const retained = (await api.listTaskSessions(task.id)).sessions.filter((row) =>
      errands.some((errand) => errand.errand_id === row.id),
    );
    expect(retained).toHaveLength(2);
    expect(retained.every((row) => !row.is_primary)).toBe(true);
    evidence.rollback =
      "disabled flag, restarted disposable backend, retained two detached sessions";
    evidence.outcome = "passed";
  } catch (error) {
    evidence.outcome = "failed";
    evidence.failure = String(error);
    throw error;
  } finally {
    if (release) await release();
    evidence.finishedAt = new Date().toISOString();
    await testInfo.attach("errands-provider-pilot.json", {
      body: JSON.stringify(evidence, null, 2),
      contentType: "application/json",
    });
  }
});
