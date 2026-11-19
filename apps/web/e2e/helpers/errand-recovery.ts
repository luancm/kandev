import { execFile } from "node:child_process";
import { basename, isAbsolute, resolve } from "node:path";
import { promisify } from "node:util";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { watchWs } from "./causal-waits";
import {
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  ownedErrandChat,
  parentChat,
  readOwnedMessages,
  startErrands,
} from "./errands";

type RecoveryBrowserContext = {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
};

export async function exerciseErrandRecoveryRefusal({
  page,
  api,
  seed,
  backend,
  testInfo,
  mobile,
}: RecoveryBrowserContext) {
  const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, { count: 1 });
  const errandId = errands[0].errand_id;
  try {
    await expect
      .poll(async () =>
        (await readOwnedMessages(api, taskId, errandId)).map((item) => item.content).join("\n"),
      )
      .toContain("Queue blocking turn started");
    await markOwnedRecovery(backend, errandId);
    const before = await readOwnedMessages(api, taskId, errandId);
    const task = await api.getTask(taskId);
    const watch = watchWs(page);
    if (mobile) await page.setViewportSize({ width: 393, height: 650 });
    await page.goto(`/t/${taskId}`);
    const chip = parentChat(page, parentId).getByTestId("errands-chip");
    await (mobile ? chip.tap() : chip.click());
    const panel = page.getByTestId(mobile ? "errands-drawer-content" : "errands-popover");
    const row = panel.getByTestId(`errands-panel-row-${errandId}`);
    await expect(row).toContainText("Recovery needs verification");
    const open = mobile ? row : panel.getByTestId(`errand-open-${errandId}`);
    await (mobile ? open.tap() : open.click());

    const conversation = ownedErrandChat(page, errandId);
    await expect(conversation.getByTestId("errand-header")).toContainText(
      "Earlier work and possible external effects are unknown",
    );
    await expect(conversation).toContainText("Queue blocking turn started");
    const composer = conversation.getByTestId("errand-continuation");
    const editor = composer.locator('.tiptap.ProseMirror[contenteditable="true"]');
    const send = composer.getByRole("button", { name: "Send" });
    if (mobile) await expectTouchTarget(send);
    const draft = "Check the unresolved Errand recovery.";
    await editor.fill(draft);
    const reply = watch.waitForResponse("errand.continue", {
      requestWhere: (payload) => payload.task_id === taskId && payload.errand_id === errandId,
    });
    const refused = expect(reply).rejects.toThrow(/errand_recovery_unresolved/);
    await (mobile ? send.tap() : send.click());
    await refused;
    await expect(composer.getByRole("alert")).toBeVisible();
    await expect(editor).toHaveText(draft);
    expect(await readOwnedMessages(api, taskId, errandId)).toEqual(before);
    expect((await api.getTask(taskId)).workflow_step_id).toBe(task.workflow_step_id);
    expect(
      (await listErrands(api, taskId)).find((errand) => errand.errand_id === errandId),
    ).toMatchObject({
      status: "needs_attention",
      attention_reason: "recovery_unresolved",
    });
    expect((await api.listTaskSessions(taskId)).sessions.map((item) => item.id)).toEqual([
      parentId,
    ]);
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("owned-errand-recovery-refusal.png") });
  } finally {
    await gates[0].release();
  }
}

async function markOwnedRecovery(backend: BackendContext, errandId: string) {
  if (!isAbsolute(backend.tmpDir) || !basename(backend.tmpDir).startsWith("kandev-e2e-")) {
    throw new Error("Recovery fixture requires an isolated browser worker directory");
  }
  const { stdout } = await promisify(execFile)(
    resolve("../backend/bin/e2e-send-now-fixture"),
    ["-test.run=^TestErrandRecoveryBrowserFixture$", "-test.timeout=30s", "-test.v"],
    {
      env: {
        ...process.env,
        KANDEV_E2E_ERRAND_RECOVERY_ROOT: backend.tmpDir,
        KANDEV_E2E_ERRAND_RECOVERY_ID: errandId,
      },
      timeout: 40_000,
    },
  );
  expect(stdout).toContain("KANDEV_E2E_ERRAND_RECOVERY_FIXTURE: attention");
}
