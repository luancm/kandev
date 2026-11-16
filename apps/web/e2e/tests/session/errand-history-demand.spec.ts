import { expect, test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  createHistoryScenario,
  installHistoryCapture,
  seedHistoryScenarios,
} from "../../helpers/errand-history-baseline";

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

test("desktop Errand history loads on demand and retains at most four pages", async ({
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
      "Task03 desktop demand history",
      125,
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
    await expect(parent).toBeVisible();
    const chip = parent.getByTestId("errands-chip");
    await expect(chip).toBeVisible();

    const closedHistoryTraffic = await capture.collect(page);
    expect(closedHistoryTraffic.wsCallers.some((call) => call.action === "errand.list")).toBe(
      false,
    );
    expect(await recordedHistoryCalls(page)).toBe(0);

    await chip.focus();
    await expect(chip).toBeFocused();
    await page.keyboard.press("Enter");
    const surface = page.getByTestId("errands-popover");
    await expect(surface).toBeVisible();
    await expect(surface.getByTestId("errands-panel-counts")).toContainText("Capacity");
    await expect(surface.getByTestId("errands-panel-counts")).toContainText("result pending");

    const firstHistoryRead = ws.waitForResponse("errand.history_page", {
      requestWhere: (payload) =>
        payload.task_id === scenario.taskId && payload.parent_session_id === scenario.parentId,
    });
    const historyToggle = surface.getByTestId(`errands-history-toggle-${scenario.parentId}`);
    await historyToggle.focus();
    await expect(historyToggle).toBeFocused();
    await page.keyboard.press("Enter");
    await firstHistoryRead;

    const historyRows = surface.getByTestId(`errands-history-rows-${scenario.parentId}`);
    await expect(historyRows.locator("li")).toHaveCount(25);
    await expect(historyRows).toHaveAttribute("data-range-start", "0");
    await expect(surface.getByTestId(`errands-history-range-${scenario.parentId}`)).toContainText(
      "125",
    );

    for (const expectedRetainedRows of [50, 75, 100, 100]) {
      const nextPage = ws.waitForResponse("errand.history_page", {
        requestWhere: (payload) =>
          payload.task_id === scenario.taskId && payload.parent_session_id === scenario.parentId,
      });
      await surface.getByTestId(`errands-history-load-more-${scenario.parentId}`).click();
      await nextPage;
      await expect(historyRows.locator("li")).toHaveCount(expectedRetainedRows);
    }

    await expect(historyRows).toHaveAttribute("data-range-start", "25");
    await expect(
      surface.getByTestId(`errands-history-start-again-${scenario.parentId}`),
    ).toBeVisible();
    await expect(surface.getByTestId(`errands-history-end-${scenario.parentId}`)).toBeVisible();
    const restartRead = ws.waitForResponse("errand.history_page", {
      requestWhere: (payload) =>
        payload.task_id === scenario.taskId &&
        payload.parent_session_id === scenario.parentId &&
        payload.cursor === undefined,
    });
    await surface.getByTestId(`errands-history-start-again-${scenario.parentId}`).click();
    await restartRead;
    await expect(historyRows.locator("li")).toHaveCount(25);
    await expect(historyRows).toHaveAttribute("data-range-start", "0");
    expect(await recordedHistoryCalls(page)).toBe(6);

    const summaryScreenshot = testInfo.outputPath("task03-errand-history-desktop-summary.png");
    await page.screenshot({ path: summaryScreenshot, animations: "disabled" });
    await testInfo.attach("task03-errand-history-desktop-summary.png", {
      path: summaryScreenshot,
      contentType: "image/png",
    });

    const panelList = surface.getByTestId("errands-panel-list");
    await panelList.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(surface.getByTestId(`errands-history-range-${scenario.parentId}`)).toBeVisible();
    const screenshot = testInfo.outputPath("task03-errand-history-desktop.png");
    await page.screenshot({ path: screenshot, animations: "disabled" });
    await testInfo.attach("task03-errand-history-desktop.png", {
      path: screenshot,
      contentType: "image/png",
    });
  } finally {
    await restore();
  }
});
