import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  deliverySnapshots,
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  parentChat,
  seedErrandContextWindow,
  startErrands,
  waitDelivered,
  waitNewParentTurnSettled,
  waitParentIdle,
} from "../../helpers/errands";

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
      .poll(async () => {
        const sessions = new Map(
          (await api.listTaskSessions(taskId)).sessions.map((session) => [
            session.id,
            session.state,
          ]),
        );
        return settledErrands.map((errand) => sessions.get(errand.errand_id));
      })
      .toEqual(settledErrands.map(() => "WAITING_FOR_INPUT"));

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

    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await chat.getByTestId(CHIP).tap();
    const drawer = page.getByTestId(DRAWER);
    const measured = drawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    const unmeasured = drawer.getByTestId(`errands-panel-row-${errands[1].errand_id}`);
    const unknown = drawer.getByTestId(`errands-panel-row-${errands[2].errand_id}`);
    await expect(measured).toContainText("Context window: 68% used");
    await expect(unmeasured).toContainText("Context window: usage not measured");
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
    await expect(measured).toBeInViewport();
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
    await expect(measured).toContainText("Context window: 82% used");
    await page.reload();
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const reloadedDrawer = page.getByTestId(DRAWER);
    const reloadedMeasured = reloadedDrawer.getByTestId(
      `errands-panel-row-${errands[0].errand_id}`,
    );
    await expect(reloadedMeasured).toContainText("Context window: 82% used");
    await expect(
      reloadedDrawer.getByTestId(`errands-panel-row-${errands[1].errand_id}`),
    ).toContainText("Context window: usage not measured");
    await seedErrandContextWindow(api, taskId, errands[0].errand_id, null);
    await expect(reloadedMeasured).not.toContainText("Context window");
    await page.reload();
    await parentChat(page, parentId).getByTestId(CHIP).tap();
    const finalDrawer = page.getByTestId(DRAWER);
    await expect(
      finalDrawer.getByTestId(`errands-panel-row-${errands[0].errand_id}`),
    ).not.toContainText("Context window");
    await finalDrawer.getByTestId("errands-stop-all").tap();
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

test("phone Threads nests child choices and back returns to the parent conversation", async ({
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
    await page.getByTestId("thread-session-picker-trigger").tap();
    const child = page.getByTestId(`thread-session-row-${errands[0].errand_id}`);
    await expect(child).toHaveAttribute("data-parent-session-id", parentId);
    await expectTouchTarget(child);
    await child.tap();
    await expect(parentChat(page, errands[0].errand_id).getByTestId("errand-header")).toBeVisible();
    await parentChat(page, errands[0].errand_id).getByTestId(BACK).tap();
    await expect(parentChat(page, parentId)).toBeVisible();
    await expectNoHorizontalOverflow(page);
  } finally {
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
    await scroller.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
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
