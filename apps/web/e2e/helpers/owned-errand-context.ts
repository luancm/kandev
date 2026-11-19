import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { ErrandInspection } from "../../lib/types/errands";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import {
  contextWindowPrompt,
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  ownedErrandChat,
  parentChat,
  routeErrandBrowserSocket,
  startErrands,
  waitDelivered,
} from "./errands";
import { mockTurnGate } from "./mock-turn-gate";

type ContextFixture = {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  phone: boolean;
};

export async function exerciseOwnedErrandContext({
  page,
  api,
  seed,
  backend,
  testInfo,
  phone,
}: ContextFixture) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  const update = mockTurnGate(backend.tmpDir, "Read a higher usage sample");
  const clear = mockTurnGate(backend.tmpDir, "Read an out-of-range usage sample");
  const finish = mockTurnGate(backend.tmpDir, "Context fixture completed");
  const spare = mockTurnGate(backend.tmpDir, "Keep one phone Errand active");
  try {
    if (phone) await page.setViewportSize({ width: 393, height: 600 });
    const prompts = [
      [
        contextWindowPrompt(68_000),
        update.prompt,
        contextWindowPrompt(82_000),
        clear.prompt,
        contextWindowPrompt(100_001),
        finish.prompt,
      ].join("\n"),
      `${contextWindowPrompt(0)}\ne2e:message("Unmeasured context")`,
      'e2e:message("No context sample")',
      ...(phone ? ['e2e:message("One more finished Errand")', spare.prompt] : []),
    ];
    const { taskId, parentId, errands } = await startErrands(api, seed, backend, { prompts });
    const [measured, unmeasured, unknown, extraFinished, spareErrand] = errands;
    const history = [unmeasured, unknown, ...(extraFinished ? [extraFinished] : [])];
    await waitDelivered(
      api,
      taskId,
      history.map((row) => row.errand_id),
    );
    await expect
      .poll(() => persistedContext(api, taskId, measured.errand_id))
      .toMatchObject({ size: 100_000, used: 68_000 });

    const detailReads: string[] = [];
    page.on("request", (request) => {
      const path = new URL(request.url()).pathname;
      if (errands.some((row) => path === `/api/v1/tasks/${taskId}/sessions/${row.errand_id}`)) {
        detailReads.push(path);
      }
    });
    const socket = await routeErrandBrowserSocket(page);
    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    await socket.waitForInitialConnection();
    const open = async () => {
      const control = parentChat(page, parentId).getByTestId("errands-chip");
      await activateContextControl(control, phone);
      const panel = page.getByTestId(phone ? "errands-drawer-content" : "errands-popover");
      await activateContextControl(panel.getByTestId(`errands-history-toggle-${parentId}`), phone);
      return panel;
    };
    const panel = await open();
    const measuredRow = contextRow(panel, measured.errand_id);
    const unmeasuredRow = contextRow(panel, unmeasured.errand_id);
    const unknownRow = contextRow(panel, unknown.errand_id);
    await expect(measuredRow).toContainText("Context window: 68.0K of 100.0K tokens");
    await expect(unmeasuredRow).toContainText("usage not measured (N/A of 100.0K tokens)");
    await expect(unknownRow).toBeVisible();
    await expect(unknownRow).not.toContainText("Context window");
    if (phone) {
      await assertPhoneContextControls(panel, [measuredRow, unmeasuredRow, unknownRow]);
    } else {
      await expect(panel.getByTestId(`errand-open-${measured.errand_id}`)).toBeVisible();
    }
    await expectNoHorizontalOverflow(page);
    await testInfo.attach("owned-context-samples", {
      body: await panel.screenshot(),
      contentType: "image/png",
    });

    const before = await listErrands(api, taskId);
    await update.release();
    await expect(measuredRow).toContainText("Context window: 82.0K of 100.0K tokens");
    const after = await listErrands(api, taskId);
    expect(after.map((row) => [row.errand_id, row.status])).toEqual(
      before.map((row) => [row.errand_id, row.status]),
    );
    await page.reload();
    const reloaded = await open();
    await expect(contextRow(reloaded, measured.errand_id)).toContainText(
      "Context window: 82.0K of 100.0K tokens",
    );

    await socket.setOfflineAndClose();
    await clear.release();
    await expect.poll(() => persistedContext(api, taskId, measured.errand_id)).toBeNull();
    const reconnected = socket.waitForReconnect();
    await page.context().setOffline(false);
    await reconnected;
    const cleared = contextRow(reloaded, measured.errand_id);
    await expect(cleared).toBeVisible();
    await expect(cleared).not.toContainText("Context window");
    await expect(contextRow(reloaded, unmeasured.errand_id)).toContainText("usage not measured");
    await expectNoHorizontalOverflow(page);
    await cleared.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath(`errands-${phone ? "phone" : "desktop"}-missed-context-clear.png`),
    });

    await finish.release();
    await waitDelivered(api, taskId, [measured.errand_id]);
    await page.reload();
    const final = await open();
    await expect(contextRow(final, measured.errand_id)).toBeVisible();
    await expect(contextRow(final, measured.errand_id)).not.toContainText("Context window");
    await expect(contextRow(final, unmeasured.errand_id)).toContainText("usage not measured");
    await expect(ownedErrandChat(page, measured.errand_id)).toHaveCount(0);
    await expect(ownedErrandChat(page, unmeasured.errand_id)).toHaveCount(0);
    expect(detailReads).toEqual([]);
    expect((await api.listTaskSessions(taskId)).sessions.map((row) => row.id)).toEqual([parentId]);
    if (spareErrand) {
      await final.getByTestId("errands-stop-all").tap();
      await expect
        .poll(
          async () =>
            (await listErrands(api, taskId)).find((row) => row.errand_id === spareErrand.errand_id)
              ?.status,
        )
        .toBe("cancelled");
    } else {
      await expect(final.getByTestId("errands-stop-all")).toHaveCount(0);
    }
  } finally {
    await page.context().setOffline(false);
    for (const gate of [update, clear, finish, spare]) await gate.release();
    await restore();
  }
}

async function persistedContext(api: ApiClient, taskId: string, errandId: string) {
  const data = await api.wsRequest<ErrandInspection>("errand.inspect", {
    task_id: taskId,
    child_session_id: errandId,
  });
  return data.errand.metadata?.context_window;
}

function contextRow(panel: Locator, errandId: string) {
  return panel.getByTestId(`errands-panel-row-${errandId}`);
}

async function activateContextControl(control: Locator, phone: boolean) {
  if (phone) await control.tap();
  else await control.click();
}

async function assertPhoneContextControls(panel: Locator, rows: Locator[]) {
  const [measured, unmeasured, unknown] = rows;
  await expect(measured).toHaveAttribute("aria-describedby", /.+/);
  await expect(unmeasured).toHaveAttribute("aria-describedby", /.+/);
  await expect(unknown).not.toHaveAttribute("aria-describedby", /.+/);
  for (const row of rows) {
    await expect(row).toHaveRole("button");
    await expect(row.locator("button")).toHaveCount(0);
    await expectTouchTarget(row);
  }
  const list = panel.getByTestId("errands-panel-list");
  await expect
    .poll(() => list.evaluate((node) => node.scrollHeight > node.clientHeight))
    .toBe(true);
  const stopAll = panel.getByTestId("errands-stop-all");
  await measured.scrollIntoViewIfNeeded();
  await expect(measured).toBeInViewport();
  await unmeasured.scrollIntoViewIfNeeded();
  await expect(unmeasured).toBeInViewport();
  await expect(stopAll).toBeInViewport();
  const footer = await stopAll.boundingBox();
  await list.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  expect((await stopAll.boundingBox())?.y).toBe(footer?.y);
}
