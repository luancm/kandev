import { expect, test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  createHistoryScenario,
  installHistoryCapture,
  seedHistoryScenarios,
} from "../../helpers/errand-history-baseline";
import { listErrands } from "../../helpers/errands";

type HistoryCaptureWindow = Window & {
  __historyCapture?: { wsCalls: Array<{ action?: string }> };
};

async function recordedHistoryCalls(page: import("@playwright/test").Page): Promise<number> {
  return page.evaluate(
    () =>
      (window as HistoryCaptureWindow).__historyCapture?.wsCalls.filter(
        (call) => call.action === "errand.history_page",
      ).length ?? 0,
  );
}

test("phone Errand history stays in a bounded drawer with its composer and Stop-all action", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  testInfo.setTimeout(240_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const scenario = await createHistoryScenario(
      api,
      seed,
      "Task03 phone demand history",
      26,
      true,
    );
    await seedHistoryScenarios(backend, seed, [scenario]);
    await page.addInitScript(() => localStorage.setItem("theme", "dark"));
    const capture = await installHistoryCapture(page);
    const ws = watchWs(page);
    const summaryRead = ws.waitForResponse("errand.browser_summary", {
      requestWhere: (payload) => payload.task_id === scenario.taskId,
    });

    await page.goto(`/t/${scenario.taskId}?sessionId=${scenario.parentId}`);
    await summaryRead;
    const parent = page.locator(
      `[data-testid="session-chat"][data-session-id="${scenario.parentId}"]:visible`,
    );
    const composer = parent.locator('.tiptap.ProseMirror[contenteditable="true"]');
    await expect(composer).toBeVisible();
    const chip = parent.getByTestId("errands-chip");
    await expect(chip).toBeVisible();

    const closedHistoryTraffic = await capture.collect(page);
    expect(closedHistoryTraffic.wsCallers.some((call) => call.action === "errand.list")).toBe(
      false,
    );
    expect(await recordedHistoryCalls(page)).toBe(0);

    await chip.tap();
    const drawer = page.getByTestId("errands-drawer-content");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("errands-panel-counts")).toContainText("result pending");
    const header = drawer.getByTestId("errands-panel-header");
    const list = drawer.getByTestId("errands-panel-list");
    const footer = drawer.getByTestId("errands-panel-footer");
    await expect(header).toBeVisible();
    await expect(footer).toBeVisible();
    await expect(composer).toBeVisible();

    const firstHistoryRead = ws.waitForResponse("errand.history_page", {
      requestWhere: (payload) =>
        payload.task_id === scenario.taskId && payload.parent_session_id === scenario.parentId,
    });
    await drawer.getByTestId(`errands-history-toggle-${scenario.parentId}`).tap();
    await firstHistoryRead;
    const historyRows = drawer.getByTestId(`errands-history-rows-${scenario.parentId}`);
    await expect(historyRows.locator("li")).toHaveCount(25);
    await expect(
      drawer.getByTestId(`errands-history-load-more-${scenario.parentId}`),
    ).toBeVisible();

    const loadMore = drawer.getByTestId(`errands-history-load-more-${scenario.parentId}`);
    const loadMoreBox = await loadMore.boundingBox();
    expect(loadMoreBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    const scrollsInternally = await list.evaluate((element) => {
      const node = element as HTMLElement;
      return node.scrollHeight > node.clientHeight;
    });
    expect(scrollsInternally).toBe(true);
    const headerBefore = await header.boundingBox();
    const footerBefore = await footer.boundingBox();
    await list.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    expect(await header.boundingBox()).toEqual(headerBefore);
    expect(await footer.boundingBox()).toEqual(footerBefore);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    const screenshot = testInfo.outputPath("task03-errand-history-phone.png");
    await page.screenshot({ path: screenshot, animations: "disabled" });
    await testInfo.attach("task03-errand-history-phone.png", {
      path: screenshot,
      contentType: "image/png",
    });

    const nextPage = ws.waitForResponse("errand.history_page", {
      requestWhere: (payload) =>
        payload.task_id === scenario.taskId && payload.parent_session_id === scenario.parentId,
    });
    await loadMore.tap();
    await nextPage;
    await expect(historyRows.locator("li")).toHaveCount(26);
    await expect(drawer.getByTestId(`errands-history-end-${scenario.parentId}`)).toBeVisible();

    const stopAll = drawer.getByTestId("errands-stop-all");
    const stopAllBox = await stopAll.boundingBox();
    expect(stopAllBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    const stopRequest = ws.waitForResponse("errand.cancel_all", {
      requestWhere: (payload) => payload.task_id === scenario.taskId,
    });
    await stopAll.tap();
    expect((await stopRequest).payload).toMatchObject({ cancelled: 1 });
    expect(
      (await listErrands(api, scenario.taskId)).find(
        (row) => row.errand_id === `${scenario.parentId}-recovery`,
      )?.status,
    ).toBe("cancelled");
  } finally {
    await restore();
  }
});
