import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { exerciseErrandRecoveryRefusal } from "../../helpers/errand-recovery";
import {
  deliverySnapshots,
  expectNormalContextSources,
  openDefaultErrandParent,
  expectDurableAwaitRecovery,
  expectMixedParentOrdering,
  expectAwaitSnapshot,
  expectNoHorizontalOverflow,
  listErrands,
  parentChat,
  routeErrandUpdateBrowserSocket,
  seedTimedOutAwait,
  startErrands,
  startMixedParentErrands,
  waitDelivered,
  waitNewParentTurnSettled,
  waitParentIdle,
  expectOwnedErrandContinuationRetry,
  ownedErrandChat,
  readOwnedMessages,
} from "../../helpers/errands";
import { exerciseOwnedErrandContext } from "../../helpers/owned-errand-context";
import { mockTurnGate } from "../../helpers/mock-turn-gate";
import { waitForFiniteAnimations } from "../../helpers/animations";

const INLINE = "errands-inline-summary";
const CHIP = "errands-chip";
const PANEL = "errands-popover";
const FINISHED = "errand-finished-card";
const BACK = "errand-back-to-parent";

test("desktop Threads opens errands through the parent panel and keeps column-owned back navigation", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend);
    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    await expect(page.getByTestId("thread-errands-summary").first()).toContainText("2 errands");
    await expect(page.getByTestId(`thread-session-tab-${errands[0].errand_id}`)).toHaveCount(0);
    await parentChat(page, parentId).evaluate((node) =>
      node.setAttribute("data-task29-parent", "retained"),
    );
    await parentChat(page, parentId).getByTestId(CHIP).click();
    await page.getByTestId(PANEL).getByTestId(`errand-open-${errands[0].errand_id}`).click();
    await expect(
      ownedErrandChat(page, errands[0].errand_id).getByTestId("errand-header"),
    ).toBeVisible();
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${parentId}"]`),
    ).toHaveAttribute("data-task29-parent", "retained");
    await ownedErrandChat(page, errands[0].errand_id).getByTestId("errand-back-to-parent").click();
    await expect(parentChat(page, parentId)).toBeVisible();
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-010.6
test("desktop Threads announces a visible errand transition without marking the parent read", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let gates: Array<ReturnType<typeof mockTurnGate>> = [];
  try {
    const fixture = await startErrands(api, seed, backend, { count: 1 });
    const { taskId, parentId, errands } = fixture;
    gates = fixture.gates;
    await expect
      .poll(
        async () =>
          (await listErrands(api, taskId)).find((row) => row.errand_id === errands[0].errand_id)
            ?.status,
      )
      .toBe("running");
    const readCursorBefore =
      (await api.getTaskSession(parentId)).session.last_read_message_id ?? null;

    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    const board = page.getByTestId("threads-board");
    const chat = board.locator(`[data-testid="session-chat"][data-session-id="${parentId}"]`);
    await expect(chat).toHaveCount(1);
    await expect(chat).toHaveAttribute("data-detail-active", "true");
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(1);
    const announcer = chat.getByTestId("errand-status-announcer");
    await expect(announcer).toHaveCount(1);
    await expect(announcer).toHaveAttribute("role", "status");
    await expect(announcer).toHaveAttribute("aria-live", "polite");
    await expect(announcer).toHaveText("");

    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    await expect(announcer).toContainText(errands[0].title);
    await expect(announcer).toContainText("Done");
    await expect(board.getByTestId("errand-status-announcer")).toHaveCount(1);
    expect((await api.getTaskSession(parentId)).session.last_read_message_id ?? null).toBe(
      readCursorBefore,
    );
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-CONTEXT-001.1 AC-TASKS-ERRANDS-CONTEXT-001.2 AC-TASKS-ERRANDS-CONTEXT-001.3
test("desktop errand rows read each child's persisted context window passively", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  await exerciseOwnedErrandContext({ page, api, seed, backend, testInfo, phone: false });
});

// @covers AC-TASKS-ERRANDS-004.13
test("desktop errand panel shows only explicit await delivery uncertainty", async ({
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
    const fixture = await startErrands(api, seed, backend, { count: 2, await: true });
    gates.push(...fixture.gates);
    const { taskId, parentId, errands } = fixture;
    for (const gate of gates) await gate.release();
    await waitDelivered(
      api,
      taskId,
      errands.map((errand) => errand.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);

    const taskList = await api.listTasks(seed.workspaceId);
    expect(
      taskList.tasks.some((task) => task.id === taskId),
      "Settled errand parent remains addressable from the workspace task list",
    ).toBe(true);
    expect((await api.getTask(taskId)).id).toBe(taskId);
    await openDefaultErrandParent(page, api, taskId, parentId);
    const chat = parentChat(page, parentId);
    await expect(chat).toBeVisible({ timeout: 15_000 });
    await expect.poll(deliveryEvents.isConnected, { timeout: 15_000 }).toBe(true);
    const historicalAwait = chat.getByTestId("errand-await-card");
    await expect(historicalAwait).toBeVisible();
    const historicalAwaitText = await historicalAwait.innerText();
    await chat.getByTestId(CHIP).click();
    const panel = page.getByTestId(PANEL);
    await panel.getByTestId(/^errands-history-toggle-/).click();
    const row = panel.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    const sibling = panel.getByTestId(`errands-panel-row-${errands[1].errand_id}`);
    const currentRows = await listErrands(api, taskId);
    const current = currentRows.find((errand) => errand.errand_id === errands[0].errand_id);
    const siblingCurrent = currentRows.find((errand) => errand.errand_id === errands[1].errand_id);
    if (!current) throw new Error("Expected the settled errand to remain in its task list");
    if (!siblingCurrent) throw new Error("Expected the sibling errand to remain in its task list");
    expect(current.delivery_state).toBe("uncertain");
    expect(siblingCurrent.delivery_state).toBe("uncertain");
    await expect(row.getByTestId("errand-delivery-uncertain")).toHaveText(
      "Result delivery uncertain",
    );
    await expect(sibling.getByTestId("errand-delivery-uncertain")).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("errand-delivery-uncertain-desktop.png"),
    });

    deliveryEvents.notifyErrandUpdated(
      Object.assign({}, siblingCurrent, { delivery_state: "received" as const }),
    );
    await expect(sibling.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    await expect(row.getByTestId("errand-delivery-uncertain")).toBeVisible();

    const omitted = Object.fromEntries(
      Object.entries(current).filter(([key]) => key !== "delivery_state"),
    ) as typeof current;
    const uncertain = Object.assign({}, current, { delivery_state: "uncertain" as const });
    deliveryEvents.notifyErrandUpdated(
      Object.assign({}, current, { delivery_state: "received" as const }),
    );
    await expect(row.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    await expect(sibling.getByTestId("errand-delivery-uncertain")).toHaveCount(0);

    deliveryEvents.notifyErrandUpdated(uncertain);
    await expect(row.getByTestId("errand-delivery-uncertain")).toBeVisible();
    await expect(sibling.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    deliveryEvents.notifyErrandUpdated(omitted);
    await expect(row.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    expect(await historicalAwait.innerText()).toBe(historicalAwaitText);
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});

test("conversational parent, explicit Open, follow-up and immutable delivered cards survive reload", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const fixture = await startErrands(api, seed, backend);
    const { taskId, parentId, errands, gates } = fixture;
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(2);
    // @covers AC-TASKS-ERRANDS-010.11
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    await expect(chat.getByTestId(CHIP)).toHaveCount(1);
    for (const row of errands)
      await expect(
        page.locator(`[data-testid="session-chat"][data-session-id="${row.errand_id}"]`),
      ).toHaveCount(0);
    const session = new SessionPage(page);
    await session.sendMessage("/e2e:errands-status");
    await expect
      .poll(async () =>
        (await api.listSessionMessages(parentId)).messages.some((message) =>
          message.content.includes("ERRAND_STATUS_BLOCKS=1"),
        ),
      )
      .toBe(true);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByText("ERRAND_STATUS_BLOCKS=1", { exact: false })).toBeVisible();
    await chat.getByTestId(CHIP).click();
    await page.getByTestId(PANEL).getByTestId(`errand-open-${errands[0].errand_id}`).click();
    const child = ownedErrandChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await expect(child.getByTestId(INLINE)).toHaveCount(0);
    expect((await api.listTaskSessions(taskId)).sessions.find((row) => row.is_primary)?.id).toBe(
      parentId,
    );
    await child.getByTestId("errand-back-to-parent").click();
    await gates[0].release();
    await gates[1].release();
    await waitDelivered(
      api,
      taskId,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId(FINISHED)).toHaveCount(2);
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(2);
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    const snapshots = await deliverySnapshots(api, parentId);
    expect(snapshots).toHaveLength(2);
    await page.reload();
    await expect(parentChat(page, parentId).getByTestId(FINISHED)).toHaveCount(2);
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshots);
    await expect(parentChat(page, parentId).getByTestId("errand-start-card")).toHaveCount(2);
    await parentChat(page, parentId).getByTestId(CHIP).click();
    await expect(page.getByTestId(PANEL).getByTestId("errands-stop-all")).toHaveCount(0);
    await page
      .getByTestId(PANEL)
      .getByTestId(/^errands-history-toggle-/)
      .click();
    for (const row of errands)
      await expect(
        page.getByTestId(PANEL).getByTestId(`errands-panel-row-${row.errand_id}`),
      ).toBeVisible();
    await page.getByTestId(PANEL).getByTestId(`errand-open-${errands[0].errand_id}`).click();
    const content = 'e2e:message("USER_FOLLOWUP_RESULT")';
    await expectOwnedErrandContinuationRetry(api, backend, {
      task_id: taskId,
      errand_id: errands[0].errand_id,
      client_message_id: "desktop-reload-follow-up",
      content,
    });
    await expect(child.getByText("USER_FOLLOWUP_RESULT", { exact: true })).toBeVisible();
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshots);
    const send = child.getByTestId("errand-send-result-button");
    await expect(send).toBeVisible();
    await send.click();
    await expect.poll(async () => (await deliverySnapshots(api, parentId)).length).toBe(3);
    expect((await deliverySnapshots(api, parentId)).slice(0, 2)).toEqual(snapshots);
  } finally {
    await restore();
  }
});

test("Auto-run off supports manual Send now and next-turn delivery without duplicate notifications", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend);
    const identity = await api.getQueueSessionIdentity(taskId, parentId);
    await api.setQueueAutoRun(identity, false);
    await page.goto(`/t/${taskId}`);
    await gates[0].release();
    await expect
      .poll(async () => (await api.getQueueStatus(identity)).count, { timeout: 60_000 })
      .toBe(1);
    expect(await deliverySnapshots(api, parentId)).toHaveLength(0);
    await api.wsRequest(
      "message.queue.send_now",
      {
        task_id: taskId,
        session_id: parentId,
        session_incarnation_id: identity.sessionIncarnationId,
        scope: "all",
      },
      40_000,
    );
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    await api.setQueueAutoRun(identity, false);
    await gates[1].release();
    await expect
      .poll(async () => (await api.getQueueStatus(identity)).count, { timeout: 60_000 })
      .toBe(1);
    await api.addUserMessage(taskId, parentId, "/e2e:errands-status");
    await waitDelivered(api, taskId, [errands[1].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    expect((await api.getQueueStatus(identity)).count).toBe(0);
    await expect(parentChat(page, parentId).getByTestId(FINISHED)).toHaveCount(2);
    expect(await deliverySnapshots(api, parentId)).toHaveLength(2);
  } finally {
    await restore();
  }
});

test("optional await joins the IDs actually returned by start", async ({
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
    for (const row of errands)
      await expect(page.getByTestId(`errand-await-row-${row.errand_id}`)).toBeVisible();
    await gates[0].release();
    await gates[1].release();
    await waitDelivered(
      api,
      taskId,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect(
      parentChat(page, parentId).getByText("Errands parent returned", { exact: false }),
    ).toBeVisible();
    const awaitCard = parentChat(page, parentId).getByTestId("errand-await-card");
    await expect(awaitCard).toContainText("All finished");
    await expect(awaitCard).toContainText(/\d+s/);
    await expect(awaitCard.getByTestId("errand-await-disclosure")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(awaitCard.getByTestId(`errand-await-row-${errands[0].errand_id}`)).toHaveCount(0);
    await awaitCard.getByTestId("errand-await-disclosure").click();
    await expect(awaitCard).toContainText("Result returned");
    await page.reload();
    const reloadedAwait = parentChat(page, parentId).getByTestId("errand-await-card");
    await expect(reloadedAwait).toContainText("All finished");
    await expect(reloadedAwait).toContainText(/\d+s/);
    await reloadedAwait.getByTestId("errand-await-disclosure").click();
    await expect(reloadedAwait).toContainText("Result returned");
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRAND-AWAIT-DURATION-001.4 AC-TASKS-ERRANDS-010.7 AC-TASKS-ERRANDS-010.11
test("completed await history stays quiet and historical through keyboard Open, child completion and reload", async ({
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
    await expect(card.locator(".spinner-grid")).toHaveCount(0);
    await card.scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath("errands-await-desktop-collapsed.png") });

    await disclosure.click();
    await expect(disclosure).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Space");
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");
    await page.keyboard.press("Enter");
    await expectAwaitSnapshot(card, errands);
    await page.screenshot({ path: testInfo.outputPath("errands-await-desktop-expanded.png") });
    await page.keyboard.press("Tab");
    await expect(card.getByTestId(`errand-open-${errands[0].errand_id}`)).toBeFocused();
    await page.keyboard.press("Enter");
    const child = ownedErrandChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await child.getByTestId("errand-back-to-parent").click();
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
    if ((await disclosure.getAttribute("aria-expanded")) === "false") await disclosure.click();
    await expectAwaitSnapshot(card, errands);
    await expectNoHorizontalOverflow(page);

    await page.reload();
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");
    await expect(disclosure).toContainText("Waited 30s for 2 errands");
    await expect(disclosure).toContainText("Timed out");
    await disclosure.click();
    await expectAwaitSnapshot(card, errands);
    expect(
      (await api.listSessionMessages(parentId)).messages.find((message) => message.id === messageId)
        ?.metadata,
    ).toEqual(recorded);
  } finally {
    await restore();
  }
});

test("five-live cap, parent turn cancellation and task-wide Stop remain independent", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend, { count: 6 });
    expect(errands).toHaveLength(5);
    const gate = mockTurnGate(backend.tmpDir, "PARENT_CANCEL_SHOULD_NOT_COMPLETE");
    await api.addUserMessage(taskId, parentId, gate.prompt);
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(taskId)).sessions.find((row) => row.id === parentId)?.state,
      )
      .toBe("RUNNING");
    await api.wsRequest("agent.cancel", { task_id: taskId, session_id: parentId }, 40_000);
    await waitParentIdle(api, taskId, parentId);
    expect(
      (await listErrands(api, taskId)).filter((row) =>
        ["queued", "running", "needs_attention"].includes(row.status),
      ),
    ).toHaveLength(5);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await chat.getByTestId(CHIP).click();
    await page.getByTestId(PANEL).getByTestId("errands-stop-all").click();
    await expect
      .poll(
        async () => (await listErrands(api, taskId)).every((row) => row.status === "cancelled"),
        { timeout: 60_000 },
      )
      .toBe(true);
    await expect(page.getByTestId(PANEL).getByTestId("errands-stop-all")).toHaveCount(0);
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-010.9 AC-TASKS-ERRANDS-010.12
test("desktop panel Stop cancels one errand and keeps its transcript and sibling", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let gates: ReturnType<typeof mockTurnGate>[] = [];
  try {
    const fixture = await startErrands(api, seed, backend, { count: 2 });
    const { taskId, parentId, errands } = fixture;
    gates = fixture.gates;
    await expect
      .poll(async () => {
        const rows = new Map(
          (await listErrands(api, taskId)).map((row) => [row.errand_id, row.status]),
        );
        return errands.map((errand) => rows.get(errand.errand_id));
      })
      .toEqual(errands.map(() => "running"));

    const transcriptBefore = await readOwnedMessages(api, taskId, errands[0].errand_id);
    expect(transcriptBefore.length).toBeGreaterThan(0);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await chat.getByTestId(CHIP).click();
    await expect(chat.getByTestId(CHIP)).toContainText("2 running");
    const panel = page.getByTestId(PANEL);
    const stop = panel.getByTestId(`errand-stop-${errands[0].errand_id}`);
    await expect(stop).toBeVisible();
    await expect(panel.getByTestId(`errand-open-${errands[0].errand_id}`)).toBeVisible();
    await waitForFiniteAnimations(panel);
    const stopBox = await stop.boundingBox();
    expect(stopBox).not.toBeNull();
    expect(stopBox!.height).toBeGreaterThanOrEqual(27);
    expect(stopBox!.height).toBeLessThanOrEqual(29);
    await page.screenshot({ path: testInfo.outputPath("errands-desktop-panel-stop.png") });

    await stop.click();
    await expect
      .poll(
        async () =>
          (await listErrands(api, taskId)).find((row) => row.errand_id === errands[0].errand_id)
            ?.status,
      )
      .toBe("cancelled");
    await expect
      .poll(
        async () =>
          (await listErrands(api, taskId)).find((row) => row.errand_id === errands[1].errand_id)
            ?.status,
      )
      .toBe("running");
    await expect(chat.getByTestId(CHIP)).toContainText("1 running");
    const transcriptAfter = await readOwnedMessages(api, taskId, errands[0].errand_id);
    const retainedIds = new Set(transcriptAfter.map((message) => message.id));
    expect(transcriptBefore.every((message) => retainedIds.has(message.id))).toBe(true);

    await panel.getByTestId(/^errands-history-toggle-/).click();
    const cancelledRow = panel.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    await expect(cancelledRow).toContainText("Cancelled");
    await expect(cancelledRow.getByTestId(`errand-stop-${errands[0].errand_id}`)).toHaveCount(0);
    await cancelledRow.getByTestId(`errand-open-${errands[0].errand_id}`).click();
    const child = ownedErrandChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await child.getByTestId("errand-back-to-parent").click();
    await expect(chat).toBeVisible();
    expect((await api.listTaskSessions(taskId)).sessions.map((session) => session.id)).toEqual([
      parentId,
    ]);
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});

test("two clients observe cancellation and a flag-off restart retains the owned transcript", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  let restore: (() => Promise<void>) | null = await backend.useEnv({
    KANDEV_FEATURES_ERRANDS: "true",
  });
  const peer = await page.context().newPage();
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend);
    for (const client of [page, peer]) {
      await client.goto(`/t/${taskId}`);
      await parentChat(client, parentId).getByTestId(CHIP).click();
      await expect(
        client.getByTestId(PANEL).getByTestId(`errands-panel-row-${errands[0].errand_id}`),
      ).toBeVisible();
    }
    const errandId = errands[0].errand_id;
    await api.wsRequest("errand.cancel", { session_id: errandId });
    for (const client of [page, peer]) {
      const panel = client.getByTestId(PANEL);
      await panel.getByTestId(/^errands-history-toggle-/).click();
      await expect(panel.getByTestId(`errands-panel-row-${errandId}`)).toContainText("Cancelled");
    }
    await api.wsRequest("errand.cancel_all", { task_id: taskId });
    await restore();
    restore = null;
    await page.reload();
    await expect(page.getByTestId(INLINE)).toHaveCount(0);
    await expect(page.getByTestId(CHIP)).toHaveCount(0);
    expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);
    await page.goto(`/t/${taskId}?sessionId=${errandId}`);
    const child = ownedErrandChat(page, errandId);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await expect(child.getByText(errands[0].objective, { exact: false })).toBeVisible();
    await expect(child.getByTestId(BACK)).toContainText("Back to parent");
    await expect(page.getByTestId(`session-tab-${errandId}`)).toHaveCount(0);
    await page.getByTestId("dockview-add-panel-btn").first().click();
    const reopenMenu = page.locator("[role='menu']:visible").last();
    const reopenRows = reopenMenu.locator(
      '[data-testid^="reopen-session-"]:not([data-testid^="reopen-session-seq-"])',
    );
    await expect(reopenRows).toHaveCount(1);
    await expect(reopenMenu.getByTestId(`reopen-session-${parentId}`)).toBeVisible();
    await expect(reopenMenu.getByTestId(`reopen-session-${errandId}`)).toHaveCount(0);
    await page.keyboard.press("Escape");
    await child.getByTestId(BACK).click();
    await expect(parentChat(page, parentId)).toBeVisible();
  } finally {
    await peer.close();
    if (restore) await restore();
  }
});

test("result delivery retains a scrolled-up reader and follows the bottom when enabled", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, gates, errands } = await startErrands(api, seed, backend);
    await api.addUserMessage(taskId, parentId, "/bulk:70");
    await expect
      .poll(async () =>
        (await api.listSessionMessages(parentId)).messages.some(
          (message) =>
            message.author_type === "agent" &&
            message.content.includes("Done. Emitted 70 messages."),
        ),
      )
      .toBe(true);
    await waitParentIdle(api, taskId, parentId);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    const scroller = chat.locator(".chat-message-list");
    await expect
      .poll(() => scroller.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    const readerPosition = await scroller.evaluate((node) => {
      node.scrollTop = (node.scrollHeight - node.clientHeight) / 2;
      node.dispatchEvent(new WheelEvent("wheel", { deltaY: -100, bubbles: true }));
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
      return node.scrollTop;
    });
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    await chat.getByTestId(CHIP).click();
    await page.getByTestId(PANEL).getByTestId(`errand-open-${errands[0].errand_id}`).click();
    await ownedErrandChat(page, errands[0].errand_id).getByTestId(BACK).click();
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
    await expect(chat.getByTestId(FINISHED)).toHaveCount(1);
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    await chat.getByTestId("jump-to-latest-button").click();
    await expect
      .poll(() =>
        scroller.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
      )
      .toBeLessThan(3);
    await gates[1].release();
    await waitDelivered(api, taskId, [errands[1].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId(FINISHED)).toHaveCount(2);
    await expect
      .poll(() =>
        scroller.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
      )
      .toBeLessThan(8);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-010.2 AC-TASKS-ERRANDS-010.4 AC-TASKS-ERRANDS-010.7
// The mounted menu proves normal-only ordering while a persisted child is selected.
test("task29 normal conversations keep tabs while parent-owned errands open temporarily and history stays collapsed", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, secondParentId, secondErrands, spareId } =
      await startMixedParentErrands(api, seed, backend);
    const primaryBefore = (await api.listTaskSessions(taskId)).sessions.find(
      (row) => row.is_primary,
    )?.id;
    await page.goto(`/t/${taskId}?sessionId=${parentId}`);
    const chat = parentChat(page, parentId);
    await expect(chat).toBeVisible();
    await expect(page.getByTestId(`session-tab-${spareId}`)).toBeVisible();
    await expect(page.getByTestId(`session-tab-${secondParentId}`)).toBeVisible();
    await expect(
      page.getByTestId(`session-tab-${secondParentId}`).getByText("2", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByTestId(`session-tab-${spareId}`).getByText("3", { exact: true }),
    ).toBeVisible();
    for (const row of [...errands, ...secondErrands])
      await expect(page.getByTestId(`session-tab-${row.errand_id}`)).toHaveCount(0);
    const recordedStart = chat.locator(
      `[data-testid="errand-start-card"][data-errand-id="${errands[0].errand_id}"]`,
    );
    await expect(recordedStart).toHaveCount(1);
    await expect(recordedStart).not.toContainText(errands[0].title);
    await recordedStart.getByRole("button", { expanded: false }).click();
    await expect(recordedStart).toContainText(errands[0].title);
    await expect(recordedStart.locator(".spinner-grid")).toHaveCount(0);
    const finished = chat.locator(
      `[data-testid="errand-finished-card"][data-errand-id="${errands[0].errand_id}"]`,
    );
    await expect(finished).not.toContainText(errands[0].title);
    await finished.getByRole("button", { expanded: false }).click();
    await expect(finished).toContainText("ERRAND_RESULT_");
    await chat.getByTestId(CHIP).click();
    let panel = page.getByTestId(PANEL);
    await expect(panel.getByTestId(/^errands-history-toggle-/)).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(panel.getByTestId(`errands-panel-row-${errands[0].errand_id}`)).toHaveCount(0);
    await expect(panel.getByTestId(`errands-panel-row-${secondErrands[0].errand_id}`)).toHaveCount(
      0,
    );
    await panel.getByTestId(/^errands-history-toggle-/).click();
    await expect(panel.getByTestId("errands-panel-row-" + errands[4].errand_id)).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath("task29-desktop-history.png") });
    await panel.getByTestId(`errand-open-${errands[0].errand_id}`).click();
    const child = ownedErrandChat(page, errands[0].errand_id);
    await expect(child).toHaveCount(1);
    await expect(child.getByTestId(BACK)).toContainText("Back to parent");
    await expect(page.getByTestId(`session-tab-${errands[0].errand_id}`)).toHaveCount(0);
    const retainedParent = page.locator(
      `[data-testid="session-chat"][data-session-id="${parentId}"]`,
    );
    await expect(retainedParent).toHaveCount(1);
    await expect(retainedParent).toBeHidden();
    await expect(retainedParent).toHaveAttribute("data-detail-active", "false");
    await expect(page.locator(".chat-message-list:visible")).toHaveCount(1);
    await page.getByTestId("dockview-add-panel-btn").first().click();
    const reopenMenu = page.locator("[role='menu']:visible").last();
    const reopenRows = reopenMenu.locator(
      '[data-testid^="reopen-session-"]:not([data-testid^="reopen-session-seq-"])',
    );
    await expect(reopenRows).toHaveCount(3);
    for (const [sessionId, ordinal] of [
      [parentId, 1],
      [secondParentId, 2],
      [spareId, 3],
    ] as const) {
      const row = reopenMenu.getByTestId(`reopen-session-${sessionId}`);
      await expect(row).toBeVisible();
      await expect(row.getByTestId(`reopen-session-seq-${ordinal}`)).toHaveText(`#${ordinal}`);
    }
    for (const row of [...errands, ...secondErrands])
      await expect(reopenMenu.getByTestId(`reopen-session-${row.errand_id}`)).toHaveCount(0);
    await page.keyboard.press("Escape");
    expect(new URL(page.url()).searchParams.get("sessionId")).toBe(errands[0].errand_id);
    await page.reload();
    await expect(child).toBeVisible();
    await child.getByTestId(BACK).click();
    await expect(chat).toBeVisible();
    await page.getByTestId(`session-tab-${secondParentId}`).click();
    const second = parentChat(page, secondParentId);
    await second.getByTestId(CHIP).click();
    panel = page.getByTestId(PANEL);
    await expect(
      panel.getByTestId(`errands-panel-row-${secondErrands[0].errand_id}`),
    ).toBeVisible();
    await expect(panel.getByTestId(`errands-panel-row-${errands[0].errand_id}`)).toHaveCount(0);
    await panel.getByTestId(`errand-open-${secondErrands[0].errand_id}`).click();
    await expect(ownedErrandChat(page, secondErrands[0].errand_id)).toBeVisible();
    await page.getByTestId(`session-tab-${spareId}`).click();
    await expect(parentChat(page, spareId)).toBeVisible();
    await page.getByTestId(`session-tab-${parentId}`).click();
    await expect(chat).toBeVisible();
    await page.goto(`/t/${taskId}?sessionId=${secondErrands[0].errand_id}`);
    await expect(ownedErrandChat(page, secondErrands[0].errand_id)).toBeVisible();
    await expect(page.getByTestId(`session-tab-${secondErrands[0].errand_id}`)).toHaveCount(0);
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("task29-desktop-child.png") });
    expect((await api.listTaskSessions(taskId)).sessions.find((row) => row.is_primary)?.id).toBe(
      primaryBefore,
    );
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRAND-ORDER-001.1 AC-TASKS-ERRAND-ORDER-001.2 AC-TASKS-ERRAND-ORDER-001.3 AC-TASKS-ERRANDS-010.4 AC-TASKS-ERRANDS-010.8 AC-TASKS-ERRANDS-011.2 AC-TASKS-ERRANDS-011.4 AC-TASKS-ERRANDS-011.7
test("desktop parent panels isolate errands and preserve ordering through settlement and switching", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await expectMixedParentOrdering(page, api, seed, backend, false);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-004.13
test("desktop durable await recovery clears uncertainty only after a matching receipt", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await expectDurableAwaitRecovery(page, api, seed, backend, false);
  } finally {
    await restore();
  }
});

// @covers AC-TASKS-ERRANDS-005.7
test("desktop normal context sources exclude errands", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  await expectNormalContextSources(testPage, apiClient, seedData, backend, false);
});

test("desktop unresolved errand recovery explains the refusal and preserves the draft", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let gates: Array<ReturnType<typeof mockTurnGate>> = [];
  try {
    const fixture = await startErrands(api, seed, backend, {
      count: 1,
      waitForParentIdle: false,
    });
    const { taskId, parentId, errands } = fixture;
    gates = fixture.gates;
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);

    const browserSocket = await routeErrandUpdateBrowserSocket(page);
    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    await expect.poll(() => browserSocket.isConnected()).toBe(true);
    const errand = errands[0];
    browserSocket.notifyErrandUpdated({
      ...errand,
      status: "needs_attention",
      attention_reason: "recovery_unresolved",
      updated_at: new Date(Date.now() + 60_000).toISOString(),
    });

    const parent = parentChat(page, parentId);
    await parent.getByTestId(CHIP).click();
    const panel = page.getByTestId(PANEL);
    const recoveryRow = panel.getByTestId(`errands-panel-row-${errand.errand_id}`);
    await expect(recoveryRow).toContainText("Recovery needs verification");
    await expect(recoveryRow).toContainText(
      "Earlier work and possible external effects are unknown",
    );
    await expect(
      page.getByTestId("toast-message").filter({ hasText: "Kandev update available" }),
    ).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath("errands-desktop-recovery-row.png") });

    await panel.getByTestId(`errand-open-${errand.errand_id}`).click();
    const child = ownedErrandChat(page, errand.errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await expect(child.getByTestId("errand-header")).toContainText(
      "Earlier work and possible external effects are unknown",
    );
    const transcript = child.locator(".chat-message-list");
    await expect(transcript).toBeVisible();
    await expect(transcript).toContainText("ERRAND_RESULT_1");
    await page.screenshot({ path: testInfo.outputPath("errands-desktop-recovery-child.png") });

    const messages = await readOwnedMessages(api, taskId, errand.errand_id);
    const continuation = child.getByTestId("errand-continuation");
    const editor = continuation.locator('.tiptap.ProseMirror[contenteditable="true"]');
    const send = continuation.getByRole("button", { name: "Send" });
    const draft = "Check whether the earlier operation completed.";
    await editor.fill(draft);
    browserSocket.refuseNextPrompt();
    await new SessionPage(page).submitMessageWithKeyboard(editor);
    await expect(continuation.getByRole("alert")).toContainText(
      "Earlier work and possible external effects remain unknown. Recovery must verify what happened before this errand can continue.",
    );
    await expect(editor).toContainText(draft);
    await expect(send).toBeEnabled();
    expect(browserSocket.refusedActions()).toEqual(["errand.continue"]);
    expect(await readOwnedMessages(api, taskId, errand.errand_id)).toEqual(messages);
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});

test("desktop real Errand recovery refusal preserves the admitted conversation", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    await exerciseErrandRecoveryRefusal({ page, api, seed, backend, testInfo, mobile: false });
  } finally {
    await restore();
  }
});
