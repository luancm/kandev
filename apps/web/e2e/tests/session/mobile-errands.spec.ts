import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  exerciseErrandInterruptedContinuation,
  exerciseErrandRecoveryRefusal,
} from "../../helpers/errand-recovery";
import {
  deliverySnapshots,
  expectNormalContextSources,
  expectDurableAwaitRecovery,
  expectMixedParentOrdering,
  expectAwaitSnapshot,
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  parentChat,
  routeErrandBrowserSocket,
  routeErrandUpdateBrowserSocket,
  seedErrandContextWindow,
  seedTimedOutAwait,
  startErrands,
  startMixedParentErrands,
  waitDelivered,
  waitNewParentTurnSettled,
  waitParentIdle,
} from "../../helpers/errands";
import { mockTurnGate } from "../../helpers/mock-turn-gate";

const INLINE = "errands-inline-summary";
const CHIP = "errands-chip";
const BACK = "errand-back-to-parent";
const DRAWER = "errands-drawer-content";

test("phone pill opens the scrolling drawer, full-screen child, Back and Stop controls", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await page.setViewportSize({ width: 393, height: 600 });
    const { taskId, parentId, errands } = await startErrands(api, seed, backend, { count: 5 });
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    // @covers AC-TASKS-ERRANDS-011.2 AC-TASKS-ERRANDS-011.7
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(5);
    await expect(chat.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
    await expectTouchTarget(chat.getByTestId(CHIP));
    await chat.getByTestId(CHIP).tap();
    const drawer = page.getByTestId(DRAWER);
    await expect(drawer).toHaveCount(1);
    const list = drawer.getByTestId("errands-panel-list");
    await expect
      .poll(() => list.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    const lastRow = drawer.getByTestId(`errands-panel-row-${errands[4].errand_id}`);
    await lastRow.scrollIntoViewIfNeeded();
    await expect(lastRow).toBeInViewport();
    await expect(drawer.getByTestId("errands-stop-all")).toBeInViewport();
    const open = drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    await open.scrollIntoViewIfNeeded();
    await expectTouchTarget(open);
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("errands-phone-drawer.png") });
    await open.tap();
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toHaveAttribute("data-layout", "phone");
    await expect(page.locator(".chat-message-list:visible")).toHaveCount(1);
    await expectTouchTarget(child.getByTestId(BACK));
    await page.screenshot({ path: testInfo.outputPath("errands-phone-child.png") });
    await child.getByTestId("errand-actions-menu").tap();
    const stop = page.getByRole("menuitem", { name: "Stop", exact: true });
    await expectTouchTarget(stop);
    await stop.tap();
    await expect
      .poll(
        async () =>
          (await listErrands(api, taskId)).find((row) => row.errand_id === errands[0].errand_id)
            ?.status,
      )
      .toBe("cancelled");
    await child.getByTestId(BACK).tap();
    await expect(chat).toBeVisible();
    await chat.getByTestId(CHIP).tap();
    await expect(drawer).toBeVisible();
    const stopAll = drawer.getByTestId("errands-stop-all");
    await expectTouchTarget(stopAll);
    await expect
      .poll(async () => {
        const bounds = await drawer.boundingBox();
        const viewport = page.viewportSize()!;
        return (
          bounds !== null &&
          bounds.x >= 0 &&
          bounds.y >= 0 &&
          bounds.x + bounds.width <= viewport.width + 1 &&
          bounds.y + bounds.height <= viewport.height + 1
        );
      })
      .toBe(true);
    await expectNoHorizontalOverflow(page);
    await stopAll.tap();
    await expect
      .poll(
        async () => (await listErrands(api, taskId)).every((row) => row.status === "cancelled"),
        { timeout: 60_000 },
      )
      .toBe(true);
    await page.keyboard.press("Escape");
    await expectNoHorizontalOverflow(page);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-CONTEXT-001.1 AC-TASKS-ERRANDS-CONTEXT-001.2 AC-TASKS-ERRANDS-CONTEXT-001.4
test("phone errand rows expose passive context through one accessible Open target", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await page.setViewportSize({ width: 393, height: 600 });
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 5,
    });
    await expect
      .poll(async () => {
        const sessions = new Map(
          (await api.listTaskSessions(taskId)).sessions.map((session) => [
            session.id,
            session.state,
          ]),
        );
        return errands.map((errand) => sessions.get(errand.errand_id));
      })
      .toEqual(errands.map(() => "RUNNING"));
    const settledErrands = errands.slice(0, -1);
    for (const gate of gates.slice(0, -1)) await gate.release();
    await waitDelivered(
      api,
      taskId,
      settledErrands.map((errand) => errand.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect
      .poll(
        async () => {
          const sessions = new Map(
            (await api.listTaskSessions(taskId)).sessions.map((session) => [
              session.id,
              session.state,
            ]),
          );
          return settledErrands.map((errand) => sessions.get(errand.errand_id));
        },
        { message: "Settled child runtimes close before context seeding", timeout: 60_000 },
      )
      .toEqual(settledErrands.map(() => "COMPLETED"));

    const childWindow = (used: number) => ({
      size: 100_000,
      used,
      remaining: 100_000 - used,
      efficiency: (used / 100_000) * 100,
      source: "acp" as const,
    });
    await seedErrandContextWindow(api, taskId, parentId, childWindow(95_000));
    await seedErrandContextWindow(api, taskId, errands[0].errand_id, childWindow(68_000));
    await seedErrandContextWindow(api, taskId, errands[1].errand_id, childWindow(0));

    const firstId = errands[0].errand_id;
    const siblingId = errands[1].errand_id;
    const detailReads = new Set<string>();
    page.on("request", (request) => {
      const path = new URL(request.url()).pathname;
      for (const childId of [firstId, siblingId]) {
        if (path === `/api/v1/tasks/${taskId}/sessions/${childId}`) detailReads.add(childId);
      }
    });
    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    const chat = parentChat(page, parentId);
    await chat.getByTestId(CHIP).tap();
    const drawer = page.getByTestId(DRAWER);
    await drawer.getByTestId("errands-completed-toggle").tap();
    const measured = drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    const unmeasured = drawer.getByTestId(`errands-panel-row-${errands[1].errand_id}`);
    const unknown = drawer.getByTestId(`errands-panel-row-${errands[2].errand_id}`);
    await expect(measured).toContainText("Context window: 68.0K of 100.0K tokens");
    await expect(unmeasured).toContainText(
      "Context window: usage not measured (N/A of 100.0K tokens)",
    );
    await expect(unknown).not.toContainText("Context window");
    await expect(measured).toHaveAttribute("aria-describedby", /.+/);
    await expect(unmeasured).toHaveAttribute("aria-describedby", /.+/);
    await expect(unknown).not.toHaveAttribute("aria-describedby", /.+/);
    for (const row of [measured, unmeasured, unknown]) {
      await expect(row).toHaveRole("button");
      await expect(row.locator("button")).toHaveCount(0);
      await expectTouchTarget(row);
    }

    const list = drawer.getByTestId("errands-panel-list");
    await expect
      .poll(() => list.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    const stopAll = drawer.getByTestId("errands-stop-all");
    await measured.scrollIntoViewIfNeeded();
    await expect(measured).toBeInViewport();
    await unmeasured.scrollIntoViewIfNeeded();
    await expect(unmeasured).toBeInViewport();
    await expect(stopAll).toBeInViewport();
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("errands-phone-context-window.png") });
    await list.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await expect(stopAll).toBeInViewport();
    await expectNoHorizontalOverflow(page);
    await list.evaluate((node) => {
      node.scrollTop = 0;
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
    });

    await seedErrandContextWindow(api, taskId, errands[0].errand_id, childWindow(82_000));
    await expect(measured).toContainText("Context window: 82.0K of 100.0K tokens");
    await page.reload();
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const reloadedDrawer = page.getByTestId(DRAWER);
    await reloadedDrawer.getByTestId("errands-completed-toggle").tap();
    const reloadedMeasured = reloadedDrawer.getByTestId(
      `errands-panel-row-${errands[0].errand_id}`,
    );
    await expect(reloadedMeasured).toContainText("Context window: 82.0K of 100.0K tokens");
    await expect(
      reloadedDrawer.getByTestId(`errands-panel-row-${errands[1].errand_id}`),
    ).toContainText("Context window: usage not measured (N/A of 100.0K tokens)");
    await seedErrandContextWindow(api, taskId, errands[0].errand_id, null);
    await expect(reloadedMeasured).not.toContainText("Context window");
    await page.reload();
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const finalDrawer = page.getByTestId(DRAWER);
    await finalDrawer.getByTestId("errands-completed-toggle").tap();
    await expect(
      finalDrawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`),
    ).not.toContainText("Context window");

    await seedErrandContextWindow(api, taskId, firstId, childWindow(68_000));
    await seedErrandContextWindow(api, taskId, siblingId, childWindow(23_000));
    const browserSocket = await routeErrandBrowserSocket(page);
    await page.reload();
    await browserSocket.waitForInitialConnection();
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const missedEventDrawer = page.getByTestId(DRAWER);
    await missedEventDrawer.getByTestId("errands-completed-toggle").tap();
    const missedClearRow = missedEventDrawer.getByTestId(`errands-panel-row-${firstId}`);
    const siblingRow = missedEventDrawer.getByTestId(`errands-panel-row-${siblingId}`);
    await expect(missedClearRow).toContainText("Context window: 68.0K of 100.0K tokens");
    await expect(siblingRow).toContainText("Context window: 23.0K of 100.0K tokens");

    await browserSocket.setOfflineAndClose();
    await seedErrandContextWindow(api, taskId, firstId, null);

    const reconnectSocket = browserSocket.waitForReconnect();
    const authoritativeList = page.waitForResponse((response) => {
      const request = response.request();
      return (
        request.method() === "GET" &&
        new URL(response.url()).pathname === `/api/v1/tasks/${taskId}/sessions`
      );
    });
    await page.context().setOffline(false);
    await reconnectSocket;
    const listResponse = await authoritativeList;
    expect(listResponse.ok()).toBe(true);
    const refreshedSessions = (await listResponse.json()) as {
      sessions: Array<{ id: string; metadata?: Record<string, unknown> | null }>;
    };
    const clearedSessionSnapshot = refreshedSessions.sessions.find(
      (session) => session.id === firstId,
    );
    expect(clearedSessionSnapshot).toBeDefined();
    expect(clearedSessionSnapshot?.metadata?.context_window).toBeNull();
    await expect(missedClearRow).not.toContainText("Context window");
    await expect(siblingRow).toContainText("Context window: 23.0K of 100.0K tokens");
    expect(detailReads).toEqual(new Set());
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${firstId}"]`),
    ).toHaveCount(0);
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${siblingId}"]`),
    ).toHaveCount(0);
    await expectNoHorizontalOverflow(page);
    await missedClearRow.scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath("errands-phone-missed-context-clear.png") });

    await page.reload();
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const reconnectedDrawer = page.getByTestId(DRAWER);
    await reconnectedDrawer.getByTestId("errands-completed-toggle").tap();
    await expect(reconnectedDrawer.getByTestId(`errands-panel-row-${firstId}`)).not.toContainText(
      "Context window",
    );
    await expect(reconnectedDrawer.getByTestId(`errands-panel-row-${siblingId}`)).toContainText(
      "Context window: 23.0K of 100.0K tokens",
    );
    await expectNoHorizontalOverflow(page);
    const reloadedClearRow = reconnectedDrawer.getByTestId(`errands-panel-row-${firstId}`);
    await reloadedClearRow.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath("errands-phone-missed-context-clear-reload.png"),
    });
    await reconnectedDrawer.getByTestId("errands-stop-all").tap();
    await expect
      .poll(
        async () =>
          (await listErrands(api, taskId)).find(
            (errand) => errand.errand_id === errands[4].errand_id,
          )?.status,
      )
      .toBe("cancelled");
    await expectNoHorizontalOverflow(page);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-004.13
test("phone errand drawer shows uncertainty and keeps transcript recovery reachable", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  const gates: Array<ReturnType<typeof mockTurnGate>> = [];
  const deliveryEvents = await routeErrandUpdateBrowserSocket(page);
  try {
    await page.setViewportSize({ width: 393, height: 650 });
    const fixture = await startErrands(api, seed, backend, { count: 1 });
    gates.push(...fixture.gates);
    const { taskId, parentId, errands } = fixture;
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);

    const taskList = await api.listTasks(seed.workspaceId);
    expect(
      taskList.tasks.some((task) => task.id === taskId),
      "Settled phone errand parent remains addressable from the workspace task list",
    ).toBe(true);
    expect((await api.getTask(taskId)).id).toBe(taskId);
    const taskRouteDiagnostics: Promise<string>[] = [];
    page.on("response", (response) => {
      const request = response.request();
      const url = new URL(response.url());
      if (
        request.method() === "GET" &&
        (url.pathname === `/api/v1/tasks/${taskId}` ||
          url.pathname === `/api/v1/tasks/${taskId}/sessions`)
      ) {
        taskRouteDiagnostics.push(
          response
            .text()
            .then((body) => `${response.status()} ${url.pathname}: ${body.slice(0, 500)}`)
            .catch((error: unknown) => `${response.status()} ${url.pathname}: ${String(error)}`),
        );
      }
    });
    page.on("console", (message) => {
      if (message.type() === "error" || message.type() === "warning") {
        taskRouteDiagnostics.push(Promise.resolve(`browser ${message.type()}: ${message.text()}`));
      }
    });
    page.on("pageerror", (error) => {
      taskRouteDiagnostics.push(Promise.resolve(`browser page error: ${error.message}`));
    });
    page.on("requestfailed", (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith(`/api/v1/tasks/${taskId}`)) {
        taskRouteDiagnostics.push(
          Promise.resolve(
            `failed request ${request.method()} ${url.pathname}: ${request.failure()?.errorText}`,
          ),
        );
      }
    });
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    try {
      await expect(chat).toBeVisible({ timeout: 15_000 });
    } catch (error) {
      throw new Error(
        `${error instanceof Error ? error.message : String(error)}\n` +
          `Browser task route diagnostics: ${JSON.stringify(await Promise.all(taskRouteDiagnostics))}`,
      );
    }
    await expect.poll(deliveryEvents.isConnected, { timeout: 15_000 }).toBe(true);
    await chat.getByTestId(CHIP).tap();
    const drawer = page.getByTestId(DRAWER);
    await drawer.getByTestId("errands-completed-toggle").tap();
    const row = drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    const current = (await listErrands(api, taskId)).find(
      (errand) => errand.errand_id === errands[0].errand_id,
    );
    if (!current) throw new Error("Expected the settled errand to remain in its task list");
    deliveryEvents.notifyErrandUpdated(
      Object.assign({}, current, { delivery_state: "uncertain" as const }),
    );
    const warning = row.getByTestId("errand-delivery-uncertain");
    await expect(warning).toHaveText("Result delivery uncertain");
    await expect(row).toHaveAttribute("aria-describedby", /.+/);
    await expectTouchTarget(row);
    await expectNoHorizontalOverflow(page);
    const viewport = page.viewportSize();
    const drawerBox = await drawer.boundingBox();
    const rowBox = await row.boundingBox();
    if (!viewport || !drawerBox || !rowBox) {
      throw new Error("Expected the phone errand row and drawer to have rendered bounds");
    }
    const edgeTolerance = 1;
    expect(drawerBox.x).toBeGreaterThanOrEqual(-edgeTolerance);
    expect(drawerBox.y).toBeGreaterThanOrEqual(-edgeTolerance);
    expect(drawerBox.x + drawerBox.width).toBeLessThanOrEqual(viewport.width + edgeTolerance);
    expect(drawerBox.y + drawerBox.height).toBeLessThanOrEqual(viewport.height + edgeTolerance);
    expect(rowBox.x).toBeGreaterThanOrEqual(drawerBox.x - edgeTolerance);
    expect(rowBox.y).toBeGreaterThanOrEqual(drawerBox.y - edgeTolerance);
    expect(rowBox.x + rowBox.width).toBeLessThanOrEqual(
      drawerBox.x + drawerBox.width + edgeTolerance,
    );
    expect(rowBox.y + rowBox.height).toBeLessThanOrEqual(
      drawerBox.y + drawerBox.height + edgeTolerance,
    );
    await page.screenshot({
      path: testInfo.outputPath("errand-delivery-uncertain-phone.png"),
    });

    await row.tap();
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toHaveAttribute("data-layout", "phone");
    await expect(child.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
    await expectTouchTarget(child.getByTestId(BACK));
    await expectNoHorizontalOverflow(page);
    await child.getByTestId(BACK).tap();
    await expect(chat).toBeVisible();
    await expectNoHorizontalOverflow(page);
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});

test("phone real Errand recovery refusal preserves the admitted conversation", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await exerciseErrandRecoveryRefusal({ page, api, seed, backend, testInfo, mobile: true });
  } finally {
    await restore();
  }
});

test("phone explicit Errand continuation follows an exact interrupted receipt", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await exerciseErrandInterruptedContinuation({
      page,
      api,
      seed,
      backend,
      testInfo,
      mobile: true,
    });
  } finally {
    await restore();
  }
});

test("phone Threads opens errands from the parent panel and Back restores its conversation", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend);
    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    await expect(page.getByTestId("thread-errands-summary").first()).toContainText("2 errands");
    await expect(page.getByTestId(`thread-session-row-${errands[0].errand_id}`)).toHaveCount(0);
    await parentChat(page, parentId).evaluate((node) =>
      node.setAttribute("data-task29-parent", "retained"),
    );
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const child = page.getByTestId(DRAWER).getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    await expectTouchTarget(child);
    await child.tap();
    await expect(parentChat(page, errands[0].errand_id).getByTestId("errand-header")).toBeVisible();
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${parentId}"]`),
    ).toHaveAttribute("data-task29-parent", "retained");
    await parentChat(page, errands[0].errand_id).getByTestId(BACK).tap();
    await expect(parentChat(page, parentId)).toBeVisible();
    await expectNoHorizontalOverflow(page);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-010.6 AC-TASKS-ERRANDS-011.1
test("phone Threads announces only the snapped conversation and baselines a hidden errand before follow-up", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  const gates: Array<ReturnType<typeof mockTurnGate>> = [];
  try {
    const first = await startErrands(api, seed, backend, { count: 1 });
    const second = await startErrands(api, seed, backend, { count: 1 });
    gates.push(...first.gates, ...second.gates);
    const secondReadCursorBefore =
      (await api.getTaskSession(second.parentId)).session.last_read_message_id ?? null;

    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${first.taskId}&sessionId=${first.parentId}`,
    );
    const board = page.getByTestId("threads-board");
    const firstColumn = board.getByTestId(`thread-column-${first.taskId}`);
    const secondColumn = board.getByTestId(`thread-column-${second.taskId}`);
    await expect(firstColumn).toHaveCount(1);
    await expect(secondColumn).toHaveCount(1);
    const firstChat = firstColumn.locator(
      `[data-testid="session-chat"][data-session-id="${first.parentId}"]`,
    );
    await expect(firstChat).toHaveCount(1);
    await expect(firstChat).toHaveAttribute("data-detail-active", "true");
    await expect(secondColumn.getByTestId("session-chat")).toHaveCount(0);
    const firstAnnouncer = firstChat.getByTestId("errand-status-announcer");
    await expect(firstAnnouncer).toHaveCount(1);
    await expect(firstAnnouncer).toHaveText("");
    await expect(board.getByTestId("errand-status-announcer")).toHaveCount(1);

    await second.gates[0].release();
    await waitDelivered(api, second.taskId, [second.errands[0].errand_id]);
    await waitParentIdle(api, second.taskId, second.parentId);
    await expect(secondColumn.getByTestId("session-chat")).toHaveCount(0);
    await expect(firstAnnouncer).toHaveText("");
    await expect(board.getByTestId("errand-status-announcer")).toHaveCount(1);

    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${second.taskId}&sessionId=${second.parentId}`,
    );
    const secondChat = secondColumn.locator(
      `[data-testid="session-chat"][data-session-id="${second.parentId}"]`,
    );
    await expect(secondChat).toHaveCount(1);
    await expect(secondChat).toHaveAttribute("data-detail-active", "true");
    await expect(firstColumn.getByTestId("session-chat")).toHaveCount(0);
    const secondAnnouncer = secondChat.getByTestId("errand-status-announcer");
    await expect(secondAnnouncer).toHaveCount(1);
    await expect(secondAnnouncer).toHaveText("");
    await expect(board.getByTestId("errand-status-announcer")).toHaveCount(1);

    const followupGate = mockTurnGate(backend.tmpDir, "ERRAND_FOLLOWUP_RESULT");
    gates.push(followupGate);
    const followupRequest = JSON.stringify({
      profile: seed.agentProfileId,
      prompts: [followupGate.prompt],
    });
    await api.addUserMessage(second.taskId, second.parentId, `/e2e:errands ${followupRequest}`);
    await expect.poll(async () => (await listErrands(api, second.taskId)).length).toBe(2);
    const followupErrand = (await listErrands(api, second.taskId)).find(
      (row) => row.errand_id !== second.errands[0].errand_id,
    );
    if (!followupErrand) throw new Error("Parent follow-up did not create its errand");
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(second.taskId)).sessions.find(
            (row) => row.id === followupErrand.errand_id,
          )?.state,
      )
      .toBe("RUNNING");
    await waitParentIdle(api, second.taskId, second.parentId);
    await expect(secondChat.getByTestId("errand-start-card")).toHaveCount(2);
    await expect(secondAnnouncer).toHaveText("");

    await followupGate.release();
    await waitDelivered(api, second.taskId, [followupErrand.errand_id]);
    await waitParentIdle(api, second.taskId, second.parentId);
    await expect(secondAnnouncer).toContainText(followupErrand.title);
    await expect(secondAnnouncer).toContainText("Done");
    await expect(board.getByTestId("errand-status-announcer")).toHaveCount(1);
    expect((await api.getTaskSession(second.parentId)).session.last_read_message_id ?? null).toBe(
      secondReadCursorBefore,
    );
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});

test.describe("narrow fine-pointer phone layout", () => {
  test("keeps the pill, drawer Open and Stop-all targets at least 44px", async ({
    desktopTestPage: page,
    apiClient: api,
    seedData: seed,
    backend,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 767, height: 850 });
    const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
    try {
      const { taskId, parentId, errands } = await startErrands(api, seed, backend, { count: 1 });
      await page.goto(`/t/${taskId}`);
      const chat = parentChat(page, parentId);
      await expectTouchTarget(chat.getByTestId(CHIP));
      await chat.getByTestId(CHIP).click();
      const drawer = page.getByTestId(DRAWER);
      await expect(drawer).toBeVisible();
      await expectTouchTarget(drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`));
      await expectTouchTarget(drawer.getByTestId("errands-stop-all"));
      await expectNoHorizontalOverflow(page);
    } finally {
      await restore();
    }
  });
});

// @covers AC-TASKS-ERRANDS-010.7 AC-TASKS-ERRANDS-011.7
test("phone retains start and result history, completed drawer access and reader scrolling", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend);
    const history =
      Array.from(
        { length: 50 },
        (_, index) =>
          `History paragraph ${index}: earlier conversation to read while errands finish.`,
      ).join("\n\n") + "\n\nEND_READER_HISTORY";
    await api.addUserMessage(taskId, parentId, `e2e:message(${JSON.stringify(history)})`);
    await expect
      .poll(async () =>
        (await api.listSessionMessages(parentId)).messages.some(
          (message) =>
            message.author_type === "agent" && message.content.includes("END_READER_HISTORY"),
        ),
      )
      .toBe(true);
    await waitParentIdle(api, taskId, parentId);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    const scroller = chat.locator(".chat-message-list");
    await expect(scroller).toHaveCount(1);
    await expect
      .poll(() => scroller.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    const readerPosition = await scroller.evaluate((node) => {
      node.scrollTop = (node.scrollHeight - node.clientHeight) / 2;
      const touch = (clientY: number) =>
        new Touch({ identifier: 1, target: node, clientX: 100, clientY });
      node.dispatchEvent(new TouchEvent("touchstart", { touches: [touch(100)], bubbles: true }));
      node.dispatchEvent(new TouchEvent("touchmove", { touches: [touch(200)], bubbles: true }));
      node.dispatchEvent(new TouchEvent("touchend", { touches: [], bubbles: true }));
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
      return node.scrollTop;
    });
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    await chat.getByTestId(CHIP).tap();
    await page.getByTestId(DRAWER).getByTestId(`errands-panel-row-${errands[0].errand_id}`).tap();
    await parentChat(page, errands[0].errand_id).getByTestId(BACK).tap();
    await expect(chat).toBeVisible();
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    const previousTurnIds = (await api.listSessionTurns(parentId)).turns.map((turn) => turn.id);
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await waitNewParentTurnSettled(api, parentId, previousTurnIds);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId("errand-finished-card")).toHaveCount(1);
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    const latest = chat.getByTestId("jump-to-latest-button");
    await expectTouchTarget(latest);
    await latest.tap();
    await expect
      .poll(() =>
        scroller.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
      )
      .toBeLessThan(3);
    await gates[1].release();
    await waitDelivered(api, taskId, [errands[1].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId("errand-finished-card")).toHaveCount(2);
    await expect
      .poll(() =>
        scroller.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
      )
      .toBeLessThan(8);
    const snapshots = await deliverySnapshots(api, parentId);
    await page.reload();
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(2);
    await expect(chat.getByTestId("errand-finished-card")).toHaveCount(2);
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshots);
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    await expect(chat.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("errands-phone-history.png") });
    await chat.getByTestId(CHIP).tap();
    const drawer = page.getByTestId(DRAWER);
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("errands-stop-all")).toHaveCount(0);
    await drawer.getByTestId("errands-completed-toggle").tap();
    for (const row of errands)
      await expect(drawer.getByTestId(`errands-panel-row-${row.errand_id}`)).toBeVisible();
    await drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`).tap();
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toHaveAttribute("data-layout", "phone");
    await child.getByTestId(BACK).tap();
    await expect(chat).toBeVisible();
  } finally {
    await restore();
  }
});

test("phone await shows the elapsed wait and returned results after reload", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      await: true,
    });
    await page.goto(`/t/${taskId}`);
    await expect(parentChat(page, parentId).getByTestId("errand-await-card")).toBeVisible();
    for (const gate of gates) await gate.release();
    await waitDelivered(
      api,
      taskId,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    const card = parentChat(page, parentId).getByTestId("errand-await-card");
    await expect(card).toContainText("All finished");
    await expect(card.getByTestId("errand-await-disclosure")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expectTouchTarget(card.getByTestId("errand-await-disclosure"));
    await card.getByTestId("errand-await-disclosure").tap();
    await expect(card).toContainText("Result returned");
    await expect(card).toContainText(/\d+s/);
    await expectNoHorizontalOverflow(page);
    await page.reload();
    const reloaded = parentChat(page, parentId).getByTestId("errand-await-card");
    await expect(reloaded).toContainText("All finished");
    await reloaded.getByTestId("errand-await-disclosure").tap();
    await expect(reloaded).toContainText("Result returned");
    await expect(reloaded).toContainText(/\d+s/);
    await expectNoHorizontalOverflow(page);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRAND-AWAIT-DURATION-001.4 AC-TASKS-ERRANDS-010.7 AC-TASKS-ERRANDS-010.11
test("phone completed await disclosure keeps saved outcomes static through tap, Open and reload", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend);
    const { messageId } = await seedTimedOutAwait(api, parentId, errands);
    const recorded = (await api.listSessionMessages(parentId)).messages.find(
      (message) => message.id === messageId,
    )?.metadata;
    expect(recorded).toBeDefined();
    await page.goto(`/t/${taskId}`);
    const card = parentChat(page, parentId).getByTestId("errand-await-card");
    await expect(card).toHaveCount(1);
    const disclosure = card.getByTestId("errand-await-disclosure");
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");
    await expect(disclosure).toContainText("Waited 30s for 2 errands");
    await expect(disclosure).toContainText("Timed out");
    await expect(card.getByRole("list")).toHaveCount(0);
    for (const errand of errands) await expect(card).not.toContainText(errand.title);
    await expectTouchTarget(disclosure);
    await card.scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath("errands-await-phone-collapsed.png") });
    await disclosure.scrollIntoViewIfNeeded();
    expect(
      await disclosure.evaluate((node) => {
        const bounds = node.getBoundingClientRect();
        const hit = document.elementFromPoint(
          bounds.x + bounds.width / 2,
          bounds.y + bounds.height / 2,
        );
        return hit !== null && node.contains(hit);
      }),
    ).toBe(true);
    await disclosure.tap();
    await expectAwaitSnapshot(card, errands);
    await page.screenshot({ path: testInfo.outputPath("errands-await-phone-expanded.png") });
    const open = card.getByTestId(`errand-open-${errands[0].errand_id}`);
    await expectTouchTarget(open);
    const [rowBounds, openBounds] = await Promise.all([
      card.getByTestId(`errand-await-row-${errands[0].errand_id}`).boundingBox(),
      open.boundingBox(),
    ]);
    expect(rowBounds).not.toBeNull();
    expect(openBounds).not.toBeNull();
    expect(openBounds!.x).toBeGreaterThanOrEqual(rowBounds!.x);
    expect(openBounds!.x + openBounds!.width).toBeLessThanOrEqual(rowBounds!.x + rowBounds!.width);
    await expectNoHorizontalOverflow(page);
    await open.tap();
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toHaveAttribute("data-layout", "phone");
    await child.getByTestId(BACK).tap();
    for (const gate of gates) await gate.release();
    await waitDelivered(
      api,
      taskId,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect
      .poll(async () => (await listErrands(api, taskId)).map((row) => row.status))
      .toEqual(["done", "done"]);
    if ((await disclosure.getAttribute("aria-expanded")) === "false") await disclosure.tap();
    await expectAwaitSnapshot(card, errands);
    await page.reload();
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");
    await expectTouchTarget(disclosure);
    await disclosure.tap();
    await expectAwaitSnapshot(card, errands);
    await expect(page.locator(".chat-message-list:visible")).toHaveCount(1);
    await expectNoHorizontalOverflow(page);
    expect(
      (await api.listSessionMessages(parentId)).messages.find((message) => message.id === messageId)
        ?.metadata,
    ).toEqual(recorded);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-011.1 AC-TASKS-ERRANDS-011.2 AC-TASKS-ERRANDS-011.7
test("task29 phone separates normal conversations and parent errands with temporary child navigation and quiet history", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await page.setViewportSize({ width: 393, height: 650 });
    const { taskId, parentId, errands, secondParentId, secondErrands, spareId } =
      await startMixedParentErrands(api, seed, backend);
    const primaryBefore = (await api.listTaskSessions(taskId)).sessions.find(
      (row) => row.is_primary,
    )?.id;
    await page.goto(`/t/${taskId}?sessionId=${parentId}`);
    const chat = parentChat(page, parentId);
    await page.getByTestId("mobile-sessions-pill").tap();
    for (const id of [parentId, secondParentId, spareId])
      await expect(page.getByTestId(`mobile-session-row-${id}`)).toBeVisible();
    for (const row of [...errands, ...secondErrands]) {
      await expect(page.getByTestId(`mobile-errand-row-${row.errand_id}`)).toHaveCount(0);
      await expect(page.getByTestId(`mobile-session-row-${row.errand_id}`)).toHaveCount(0);
    }
    await page.getByTestId(`mobile-session-row-${parentId}`).tap();
    const recordedStart = chat.locator(
      `[data-testid="errand-start-card"][data-errand-id="${errands[0].errand_id}"]`,
    );
    await expect(recordedStart).not.toContainText(errands[0].title);
    const startToggle = recordedStart.getByRole("button", { expanded: false });
    await expectTouchTarget(startToggle);
    await startToggle.tap();
    await expect(recordedStart).toContainText(errands[0].title);
    await expect(recordedStart.locator(".spinner-grid")).toHaveCount(0);
    const finished = chat.locator(
      `[data-testid="errand-finished-card"][data-errand-id="${errands[0].errand_id}"]`,
    );
    await expect(finished).not.toContainText(errands[0].title);
    await finished.getByRole("button", { expanded: false }).tap();
    await expect(finished).toContainText("ERRAND_RESULT_");
    await chat.getByTestId(CHIP).tap();
    let drawer = page.getByTestId(DRAWER);
    const completed = drawer.getByTestId("errands-completed-toggle");
    await expectTouchTarget(completed);
    await expect(completed).toHaveAttribute("aria-expanded", "false");
    await expect(drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`)).toHaveCount(0);
    await completed.tap();
    await expect
      .poll(() =>
        drawer
          .getByTestId("errands-panel-list")
          .evaluate((node) => node.scrollHeight > node.clientHeight),
      )
      .toBe(true);
    const lastRow = drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    await lastRow.scrollIntoViewIfNeeded();
    await expect(lastRow).toBeInViewport();
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("task29-phone-history.png") });
    const firstRow = drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    await firstRow.scrollIntoViewIfNeeded();
    await expectTouchTarget(firstRow);
    await firstRow.tap();
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId(BACK)).toContainText("Back to parent");
    await expectTouchTarget(child.getByTestId(BACK));
    const retainedParent = page.locator(
      `[data-testid="session-chat"][data-session-id="${parentId}"]`,
    );
    await expect(retainedParent).toHaveCount(1);
    await expect(retainedParent).toBeHidden();
    await expect(retainedParent).toHaveAttribute("data-detail-active", "false");
    await expect(child).toHaveAttribute("data-detail-active", "true");
    await expect(page.locator(".chat-message-list:visible")).toHaveCount(1);
    expect(new URL(page.url()).searchParams.get("sessionId")).toBe(errands[0].errand_id);
    await page.reload();
    await expect(child).toBeVisible();
    await child.getByTestId(BACK).tap();
    await expect(chat).toBeVisible();
    await page.getByTestId("mobile-sessions-pill").tap();
    await page.getByTestId(`mobile-session-row-${secondParentId}`).tap();
    const second = parentChat(page, secondParentId);
    await second.getByTestId(CHIP).tap();
    drawer = page.getByTestId(DRAWER);
    await expect(
      drawer.getByTestId(`errands-panel-row-${secondErrands[0].errand_id}`),
    ).toBeVisible();
    await expect(drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`)).toHaveCount(0);
    await drawer.getByTestId(`errands-panel-row-${secondErrands[0].errand_id}`).tap();
    await expect(parentChat(page, secondErrands[0].errand_id)).toBeVisible();
    await page.goto(`/t/${taskId}?sessionId=${secondErrands[0].errand_id}`);
    await expect(parentChat(page, secondErrands[0].errand_id)).toBeVisible();
    await expect(page.locator(".chat-message-list:visible")).toHaveCount(1);
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("task29-phone-child.png") });
    expect((await api.listTaskSessions(taskId)).sessions.find((row) => row.is_primary)?.id).toBe(
      primaryBefore,
    );
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRAND-ORDER-001.1 AC-TASKS-ERRAND-ORDER-001.2 AC-TASKS-ERRAND-ORDER-001.3 AC-TASKS-ERRANDS-010.4 AC-TASKS-ERRANDS-010.8 AC-TASKS-ERRANDS-011.2 AC-TASKS-ERRANDS-011.4 AC-TASKS-ERRANDS-011.7
test("phone parent drawer preserves scoped ordering through settlement and reload", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await page.setViewportSize({ width: 393, height: 600 });
    await expectMixedParentOrdering(page, api, seed, backend, true);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-004.13
test("phone durable await recovery clears uncertainty only after a matching receipt", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await expectDurableAwaitRecovery(page, api, seed, backend, true);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-005.7
test("phone normal context sources exclude errands", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  await expectNormalContextSources(testPage, apiClient, seedData, backend, true);
});

test("phone unresolved errand recovery explains the refusal and preserves the draft", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let gates: Array<ReturnType<typeof mockTurnGate>> = [];
  try {
    await page.setViewportSize({ width: 393, height: 600 });
    const fixture = await startErrands(api, seed, backend, {
      count: 1,
      waitForParentIdle: false,
    });
    const { taskId, parentId, errands } = fixture;
    gates = fixture.gates;
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);

    const browserSocket = await routeErrandUpdateBrowserSocket(page);
    await page.goto(`/t/${taskId}`);
    await expect.poll(() => browserSocket.isConnected()).toBe(true);
    const errand = errands[0];
    browserSocket.notifyErrandUpdated({
      ...errand,
      status: "needs_attention",
      attention_reason: "recovery_unresolved",
      updated_at: new Date(Date.now() + 60_000).toISOString(),
    });

    const parent = parentChat(page, parentId);
    await parent.getByTestId(CHIP).tap();
    const drawer = page.getByTestId(DRAWER);
    const recoveryRow = drawer.getByTestId(`errands-panel-row-${errand.errand_id}`);
    await expectTouchTarget(recoveryRow);
    await expect(recoveryRow).toContainText("Recovery needs verification");
    await expect(recoveryRow).toContainText(
      "Earlier work and possible external effects are unknown",
    );
    await expectNoHorizontalOverflow(page);
    await expect(
      page.getByTestId("toast-message").filter({ hasText: "Kandev update available" }),
    ).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath("errands-phone-recovery-drawer.png") });
    await recoveryRow.tap();

    const child = parentChat(page, errand.errand_id);
    await expect(child.getByTestId("errand-header")).toHaveAttribute("data-layout", "phone");
    await expect(child.getByTestId("errand-header")).toContainText(
      "Earlier work and possible external effects are unknown",
    );
    const transcript = child.locator(".chat-message-list");
    await expect(transcript).toBeVisible();
    await expect(transcript).toContainText("ERRAND_RESULT_1");
    await expectTouchTarget(child.getByTestId(BACK));
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("errands-phone-recovery-child.png") });

    const messageCount = (await api.listSessionMessages(errand.errand_id)).messages.length;
    const turnCount = (await api.listSessionTurns(errand.errand_id)).turns.length;
    const session = new SessionPage(page);
    browserSocket.refuseNextPrompt();
    await session.sendMessageViaButton("Check whether the earlier operation completed.");
    const refusal = page
      .getByTestId("toast-message")
      .filter({ hasText: "Recovery must verify what happened before this errand can continue." });
    await expect(refusal).toContainText(
      "Earlier work and possible external effects remain unknown. Recovery must verify what happened before this errand can continue.",
    );
    const editor = child.locator('.tiptap.ProseMirror[contenteditable="true"]');
    await expect(editor).toContainText("Check whether the earlier operation completed.");
    await expect(session.submitButton()).toBeEnabled();
    expect(browserSocket.refusedActions()).toEqual(["message.add"]);
    expect((await api.listSessionMessages(errand.errand_id)).messages).toHaveLength(messageCount);
    expect((await api.listSessionTurns(errand.errand_id)).turns).toHaveLength(turnCount);
    await child.getByTestId(BACK).tap();
    await expect(parent).toBeVisible();
    await expectNoHorizontalOverflow(page);
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});
