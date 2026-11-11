import { randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { promisify } from "node:util";
import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import { dwell, watchWs, type WsWatcher } from "./causal-waits";
import { listErrands, parentChat, startErrands } from "./errands";
import { mockTurnGate } from "./mock-turn-gate";
import type { Errand } from "../../lib/types/errands";

const execute = promisify(execFile);
const FIXTURE_MARKER = "KANDEV_E2E_ERRAND_RECOVERY_FIXTURE:";
const ORIGINAL_PROMPT_TOKEN = "Queue blocking turn started";
const ERRAND_PANEL = "errands-popover";
const ERRAND_DRAWER = "errands-drawer-content";
const ERRAND_CHIP = "errands-chip";

type FixtureProof = {
  action: string;
  task_id: string;
  session_id: string;
  parent_session_id: string;
  incarnation_id: string;
  harness_generation: number;
  native_session_id: string;
  submission_id: string;
  turn_id: string;
  payload_hash: string;
  workflow_step_id: string;
  attention: boolean;
  message_count: number;
  turn_count: number;
  revision: number;
  result_kind?: string;
  result_turn_id?: string;
  result_text?: string;
  settlement_receipts?: number;
  settlement_outcome?: string;
};

type ACPTraceRecord = {
  event: string;
  session_id: string;
  prompt?: string;
  process_id?: string;
  connection_id?: string;
};

type RecoveryBrowserContext = {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
};

type TracedErrand = {
  taskId: string;
  parentId: string;
  errand: Errand;
  tracePath: string;
  gates: Array<ReturnType<typeof mockTurnGate>>;
  proof: FixtureProof;
};

export async function exerciseErrandRecoveryRefusal({
  page,
  api,
  seed,
  backend,
  testInfo,
  mobile,
}: RecoveryBrowserContext): Promise<void> {
  const fixture = await createTracedErrand(api, seed, backend, "unresolved-refusal");
  try {
    const proof = await runErrandRecoveryFixture(
      backend.tmpDir,
      fixture.errand.errand_id,
      fixture.tracePath,
      {
        action: "prepare_attention",
        promptToken: ORIGINAL_PROMPT_TOKEN,
      },
    );
    expect(proof.attention).toBe(true);
    const baseline = await runErrandRecoveryFixture(
      backend.tmpDir,
      fixture.errand.errand_id,
      fixture.tracePath,
      {
        action: "snapshot",
      },
    );
    expect(baseline.attention).toBe(true);
    expect(baseline.message_count).toBe(fixture.proof.message_count);
    expect(baseline.turn_count).toBe(fixture.proof.turn_count);

    const watcher = watchWs(page);
    await openErrandParent(page, fixture, seed.workspaceId, mobile);
    const reloadList = watcher.waitForResponse("errand.list", {
      requestWhere: (payload) => payload.task_id === fixture.taskId,
    });
    await page.reload();
    const listResponse = await reloadList;
    assertRecoveryErrandList(listResponse.rawPayload, fixture.errand.errand_id);
    expect(
      (await listErrands(api, fixture.taskId)).find(
        (row) => row.errand_id === fixture.errand.errand_id,
      ),
    ).toEqual(
      expect.objectContaining({
        status: "needs_attention",
        attention_reason: "recovery_unresolved",
      }),
    );

    const parent = parentChat(page, fixture.parentId);
    const panel = await openErrandList(parent, page, fixture.errand.errand_id, mobile);
    const row = panel.getByTestId(`errands-panel-row-${fixture.errand.errand_id}`);
    await expect(row).toContainText("Recovery needs verification");
    await expect(row).toContainText("Earlier work and possible external effects are unknown");
    if (mobile) await expectNoHorizontalOverflow(page);
    await page.screenshot({
      path: testInfo.outputPath(
        mobile ? "phone-errand-recovery-row.png" : "desktop-errand-recovery-row.png",
      ),
    });
    await openErrandTranscript(panel, page, fixture.errand.errand_id, mobile);

    const child = parentChat(page, fixture.errand.errand_id);
    const header = child.getByTestId("errand-header");
    await expect(header).toBeVisible();
    await expect(header).toContainText("Earlier work and possible external effects are unknown");
    await expect(child.locator(".chat-message-list")).toContainText(ORIGINAL_PROMPT_TOKEN);
    if (mobile) await expectNoHorizontalOverflow(page);
    await page.screenshot({
      path: testInfo.outputPath(
        mobile ? "phone-errand-recovery-transcript.png" : "desktop-errand-recovery-transcript.png",
      ),
    });

    const traceBefore = readACPTrace(fixture.tracePath);
    expect(traceBefore.filter((record) => record.event === "prompt")).toHaveLength(1);
    const refusalBaseline = await runErrandRecoveryFixture(
      backend.tmpDir,
      fixture.errand.errand_id,
      fixture.tracePath,
      { action: "snapshot" },
    );
    const editor = errandComposer(child);
    const submit = child.getByTestId("submit-message-button");
    for (let attempt = 0; attempt < 2; attempt += 1) {
      const draft = `Check the unresolved errand recovery ${attempt + 1}.`;
      await expect(editor).toBeEditable();
      await editor.fill(draft);
      const refused = expectRecoveryRefusal(watcher, fixture.errand.errand_id, draft);
      await expect(submit).toBeEnabled();
      if (mobile) await submit.tap();
      else await submit.click();
      await refused;

      const toast = page
        .getByTestId("toast-message")
        .filter({ hasText: "Message not sent" })
        .last();
      await expect(toast).toContainText(
        "Earlier work and possible external effects remain unknown.",
      );
      await expect(toast).toContainText(
        "Recovery must verify what happened before this errand can continue.",
      );
      await expect(editor).toHaveText(draft);
      await expect(submit).toBeEnabled();

      await dwell(
        page,
        300,
        "negative-assertion",
        "a refused prompt must not reach the mock ACP provider",
      );
      const traceAfter = readACPTrace(fixture.tracePath);
      expect(traceAfter.filter((record) => record.event === "prompt")).toEqual(
        traceBefore.filter((record) => record.event === "prompt"),
      );
      const after = await runErrandRecoveryFixture(
        backend.tmpDir,
        fixture.errand.errand_id,
        fixture.tracePath,
        {
          action: "snapshot",
        },
      );
      expect(after.attention).toBe(true);
      expect(after.message_count).toBe(refusalBaseline.message_count);
      expect(after.turn_count).toBe(refusalBaseline.turn_count);
      expect(after.revision).toBe(refusalBaseline.revision);
    }
    if (mobile) {
      await expectTouchTarget(submit);
      await expectNoHorizontalOverflow(page);
    }
  } finally {
    await stopTracedErrand(api, fixture);
  }
}

export async function exerciseErrandInterruptedContinuation({
  page,
  api,
  seed,
  backend,
  testInfo,
  mobile,
}: RecoveryBrowserContext): Promise<void> {
  const fixture = await createTracedErrand(api, seed, backend, "interrupted-continuation");
  try {
    const watcher = watchWs(page);
    await openErrandParent(page, fixture, seed.workspaceId, mobile);
    const parent = parentChat(page, fixture.parentId);
    const panel = await openErrandList(parent, page, fixture.errand.errand_id, mobile);
    await openErrandTranscript(panel, page, fixture.errand.errand_id, mobile);
    const child = parentChat(page, fixture.errand.errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    const cancel = child.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();
    if (mobile) await cancel.tap();
    else await cancel.click();
    await expect
      .poll(async () => (await api.getTaskSession(fixture.errand.errand_id)).session.state, {
        message: "cancelled Errand turn settles into an idle retained session",
        timeout: 60_000,
      })
      .toBe("WAITING_FOR_INPUT");

    const interrupted = await runErrandRecoveryFixture(
      backend.tmpDir,
      fixture.errand.errand_id,
      fixture.tracePath,
      {
        action: "settle_interrupted",
        expected: fixture.proof,
      },
    );
    expect(interrupted.result_kind).toBe("interrupted");
    expect(interrupted.settlement_receipts).toBe(1);
    expect(interrupted.settlement_outcome).toBe("parent:interrupted");
    expect(interrupted.native_session_id).toBe(fixture.proof.native_session_id);
    expect(interrupted.harness_generation).toBe(fixture.proof.harness_generation);

    const before = await runErrandRecoveryFixture(
      backend.tmpDir,
      fixture.errand.errand_id,
      fixture.tracePath,
      {
        action: "snapshot",
      },
    );
    const taskBefore = await api.getTask(fixture.taskId);
    const turnsBefore = (await api.listSessionTurns(fixture.errand.errand_id)).turns;
    const messagesBefore = (await api.listSessionMessages(fixture.errand.errand_id)).messages;
    const explicitInstruction = `Continue with a new instruction ${randomUUID()}.`;
    const editor = errandComposer(child);
    const submit = child.getByTestId("submit-message-button");
    await expect(editor).toBeEditable();
    await editor.fill(explicitInstruction);
    const accepted = watcher.waitForResponse("message.add", {
      requestWhere: (payload) =>
        payload.session_id === fixture.errand.errand_id &&
        typeof payload.content === "string" &&
        payload.content.includes(explicitInstruction),
      timeout: 30_000,
    });
    await expect(submit).toBeEnabled();
    if (mobile) await submit.tap();
    else await submit.click();
    await accepted;

    await expect
      .poll(() =>
        matchingPromptCount(fixture.tracePath, interrupted.native_session_id, explicitInstruction),
      )
      .toBe(1);
    const afterProof = await runErrandRecoveryFixture(
      backend.tmpDir,
      fixture.errand.errand_id,
      fixture.tracePath,
      {
        action: "assert_continuation",
        expected: fixture.proof,
        continuationToken: explicitInstruction,
      },
    );
    expect(afterProof.native_session_id).toBe(interrupted.native_session_id);
    expect(afterProof.harness_generation).toBe(interrupted.harness_generation);
    expect(afterProof.turn_id).not.toBe(interrupted.turn_id);
    expect(afterProof.submission_id).not.toBe(interrupted.submission_id);
    expect(afterProof.workflow_step_id).toBe(taskBefore.workflow_step_id);
    expect(afterProof.revision).toBeGreaterThan(before.revision);
    expect(afterProof.turn_count).toBeGreaterThan(turnsBefore.length);
    expect(afterProof.message_count).toBeGreaterThan(messagesBefore.length);
    expect((await api.getTask(fixture.taskId)).workflow_step_id).toBe(taskBefore.workflow_step_id);

    const prompts = readACPTrace(fixture.tracePath).filter(
      (record) => record.event === "prompt" && record.session_id === interrupted.native_session_id,
    );
    expect(prompts).toHaveLength(2);
    expect(prompts.filter((record) => record.prompt?.includes(ORIGINAL_PROMPT_TOKEN))).toHaveLength(
      1,
    );
    expect(prompts.filter((record) => record.prompt?.includes(explicitInstruction))).toHaveLength(
      1,
    );
    await expect(editor).toHaveText("");
    if (mobile) {
      await expectTouchTarget(submit);
      await expectNoHorizontalOverflow(page);
      await page.screenshot({
        path: testInfo.outputPath("phone-errand-interrupted-continuation.png"),
      });
    } else {
      await page.screenshot({
        path: testInfo.outputPath("desktop-errand-interrupted-continuation.png"),
      });
    }
  } finally {
    await stopTracedErrand(api, fixture);
  }
}

async function createTracedErrand(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  purpose: string,
): Promise<TracedErrand> {
  const suffix = randomUUID();
  const tracePath = path.join(
    backend.tmpDir,
    `errand-recovery-${purpose}-${suffix}-acp-trace.jsonl`,
  );
  fs.writeFileSync(tracePath, "");
  const { agents } = await api.listAgents();
  const owner = agents.find((agent) =>
    agent.profiles?.some((profile) => profile.id === seed.agentProfileId),
  );
  if (!owner) throw new Error("seed mock-agent profile owner is missing");
  const tracedProfile = await api.createAgentProfile(
    owner.id,
    `errand-recovery-${purpose}-${suffix}`,
    {
      model: "mock-fast",
      env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
    },
  );
  const started = await startErrands(api, seed, backend, {
    count: 1,
    errandAgentProfileId: tracedProfile.id,
  });
  const errand = started.errands[0];
  if (!errand) throw new Error("Errand admission returned no child session");
  await expect
    .poll(() => matchingPromptCount(tracePath, undefined, ORIGINAL_PROMPT_TOKEN), {
      message: "the admitted child reaches the traced mock ACP provider",
      timeout: 60_000,
    })
    .toBe(1);
  const proof = await runErrandRecoveryFixture(backend.tmpDir, errand.errand_id, tracePath, {
    action: "inspect_admission",
    promptToken: ORIGINAL_PROMPT_TOKEN,
  });
  expect(proof.native_session_id).toBeTruthy();
  expect(proof.submission_id).toBeTruthy();
  return {
    taskId: started.taskId,
    parentId: started.parentId,
    errand,
    tracePath,
    gates: started.gates,
    proof,
  };
}

async function runErrandRecoveryFixture(
  fixtureRoot: string,
  sessionId: string,
  tracePath: string,
  options: {
    action: string;
    promptToken?: string;
    expected?: FixtureProof;
    continuationToken?: string;
  },
): Promise<FixtureProof> {
  if (!path.isAbsolute(fixtureRoot) || !path.basename(fixtureRoot).startsWith("kandev-e2e-")) {
    throw new Error("Errand recovery fixture requires the managed kandev-e2e-* root");
  }
  const fixture = await execute(
    path.resolve(
      process.cwd(),
      `../backend/bin/e2e-send-now-fixture${process.platform === "win32" ? ".exe" : ""}`,
    ),
    ["-test.run=^TestErrandRecoveryBrowserFixture$", "-test.timeout=2m", "-test.v"],
    {
      cwd: path.resolve(process.cwd(), "../backend"),
      env: errandRecoveryFixtureEnv(fixtureRoot, sessionId, tracePath, options),
      timeout: 150_000,
      maxBuffer: 1 << 20,
    },
  ).catch((failure: unknown) => {
    const output = failure as { stdout?: string; stderr?: string };
    throw new Error(
      `Errand recovery fixture failed (${options.action}):\n${output.stdout ?? ""}\n${output.stderr ?? ""}`,
    );
  });
  return parseErrandRecoveryFixtureProof(fixture.stdout, fixture.stderr);
}

function errandRecoveryFixtureEnv(
  fixtureRoot: string,
  sessionId: string,
  tracePath: string,
  options: {
    action: string;
    promptToken?: string;
    expected?: FixtureProof;
    continuationToken?: string;
  },
): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    KANDEV_E2E_ERRAND_RECOVERY_ROOT: fixtureRoot,
    KANDEV_E2E_ERRAND_RECOVERY_SESSION: sessionId,
    KANDEV_E2E_ERRAND_RECOVERY_ACTION: options.action,
    KANDEV_E2E_ERRAND_RECOVERY_TRACE: tracePath,
    KANDEV_E2E_ERRAND_RECOVERY_PROMPT_TOKEN: options.promptToken ?? ORIGINAL_PROMPT_TOKEN,
    KANDEV_E2E_ERRAND_RECOVERY_CONTINUATION_TOKEN: options.continuationToken ?? "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_SUBMISSION: "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_TURN: "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_NATIVE: "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_INCARNATION: "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_PAYLOAD_HASH: "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_GENERATION: "",
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_WORKFLOW_STEP: "",
  };
  if (options.expected) Object.assign(env, errandRecoveryExpectedEnv(options.expected));
  return env;
}

function errandRecoveryExpectedEnv(expected: FixtureProof): NodeJS.ProcessEnv {
  return {
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_SUBMISSION: expected.submission_id,
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_TURN: expected.turn_id,
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_NATIVE: expected.native_session_id,
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_INCARNATION: expected.incarnation_id,
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_PAYLOAD_HASH: expected.payload_hash,
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_GENERATION: String(expected.harness_generation),
    KANDEV_E2E_ERRAND_RECOVERY_EXPECT_WORKFLOW_STEP: expected.workflow_step_id,
  };
}

function parseErrandRecoveryFixtureProof(stdout: string, stderr: string): FixtureProof {
  const line = stdout.split(/\r?\n/).find((entry) => entry.startsWith(FIXTURE_MARKER));
  if (!line)
    throw new Error(`Errand recovery fixture omitted its proof marker:\n${stdout}\n${stderr}`);
  return JSON.parse(line.slice(FIXTURE_MARKER.length)) as FixtureProof;
}

async function openErrandParent(
  page: Page,
  fixture: TracedErrand,
  workspaceId: string,
  mobile: boolean,
): Promise<void> {
  if (mobile) {
    await page.setViewportSize({ width: 393, height: 650 });
    await page.goto(`/t/${fixture.taskId}`);
  } else {
    await page.goto(
      `/threads?workspace=${workspaceId}&taskId=${fixture.taskId}&sessionId=${fixture.parentId}`,
    );
  }
  await expect(parentChat(page, fixture.parentId)).toBeVisible();
}

async function openErrandList(
  parent: ReturnType<typeof parentChat>,
  page: Page,
  errandId: string,
  mobile: boolean,
) {
  const chip = parent.getByTestId(ERRAND_CHIP);
  if (mobile) await chip.tap();
  else await chip.click();
  const panel = page.getByTestId(mobile ? ERRAND_DRAWER : ERRAND_PANEL);
  await expect(panel).toBeVisible();
  await expect(panel.getByTestId(`errands-panel-row-${errandId}`)).toBeVisible();
  return panel;
}

async function openErrandTranscript(
  panel: ReturnType<typeof parentChat>,
  page: Page,
  errandId: string,
  mobile: boolean,
) {
  const open = mobile
    ? panel.getByTestId(`errands-panel-row-${errandId}`)
    : panel.getByTestId(`errand-open-${errandId}`);
  if (mobile) await open.tap();
  else await open.click();
  await expect(parentChat(page, errandId)).toBeVisible();
}

function assertRecoveryErrandList(payload: unknown, errandId: string): void {
  expect(payload).toEqual(
    expect.objectContaining({
      errands: expect.arrayContaining([
        expect.objectContaining({
          errand_id: errandId,
          status: "needs_attention",
          attention_reason: "recovery_unresolved",
        }),
      ]),
    }),
  );
}

function errandComposer(child: Locator): Locator {
  return child.locator('.tiptap.ProseMirror[contenteditable="true"]').first();
}

function expectRecoveryRefusal(
  watcher: WsWatcher,
  sessionId: string,
  draft: string,
): Promise<void> {
  const options = {
    requestWhere: (payload: Record<string, unknown>) =>
      payload.session_id === sessionId && payload.content === draft,
    timeout: 30_000,
  };
  return watcher.waitForResponse("message.add", options).then(
    () => {
      throw new Error("The unresolved Errand prompt unexpectedly reached provider admission");
    },
    (error: unknown) => {
      const message = error instanceof Error ? error.message : String(error);
      expect(message).toContain("CONFLICT");
      expect(message).toContain("errand_recovery_unresolved");
    },
  );
}

function readACPTrace(tracePath: string): ACPTraceRecord[] {
  return fs
    .readFileSync(tracePath, "utf8")
    .split("\n")
    .filter((line) => line.trim())
    .map((line) => JSON.parse(line) as ACPTraceRecord);
}

function matchingPromptCount(
  tracePath: string,
  nativeSessionId: string | undefined,
  promptToken: string,
): number {
  return readACPTrace(tracePath).filter(
    (record) =>
      record.event === "prompt" &&
      (nativeSessionId === undefined || record.session_id === nativeSessionId) &&
      record.prompt?.includes(promptToken),
  ).length;
}

async function stopTracedErrand(api: ApiClient, fixture: TracedErrand): Promise<void> {
  try {
    await api.stopSession({ session_id: fixture.errand.errand_id, force: true });
  } catch {
    // The test may already have ended the isolated child through cancellation.
  } finally {
    for (const gate of fixture.gates) await gate.release();
  }
}

async function expectTouchTarget(control: Locator): Promise<void> {
  await expect
    .poll(async () => {
      const box = await control.boundingBox();
      return box !== null && box.height >= 44 && box.width >= 44;
    })
    .toBe(true);
}

async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
