import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { Errand } from "../../lib/types/errands";
import { dwell, watchWs } from "./causal-waits";
import { createHistoryScenario, seedHistoryScenarios } from "./errand-history-baseline";

async function historyFaultControl(page: Page) {
  let fail = true;
  let requests = 0;
  let notify: ((row: Errand) => void) | undefined;
  await page.routeWebSocket(/\/ws(?:\?.*)?$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part) as { type?: string; action?: string; id?: string };
        if (frame.type === "request" && frame.action === "errand.history_page") {
          requests += 1;
          if (fail) {
            fail = false;
            socket.send(
              JSON.stringify({
                type: "error",
                action: frame.action,
                id: frame.id,
                payload: { code: "INTERNAL_ERROR", message: "Controlled history failure" },
              }),
            );
            continue;
          }
        }
        server.send(part);
      }
    });
    server.onMessage((message) => socket.send(message));
    notify = (row) =>
      socket.send(JSON.stringify({ type: "notification", action: "errand.updated", payload: row }));
  });
  return {
    requestCount: () => requests,
    notify: (row: Errand) => {
      if (!notify) throw new Error("Browser socket is not connected");
      notify(row);
    },
  };
}

export async function exerciseHistoryAcceptance({
  page,
  api,
  seed,
  backend,
  testInfo,
  mobile,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
}) {
  testInfo.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  const activate = (control: Locator) => (mobile ? control.tap() : control.click());
  try {
    const scenario = await createHistoryScenario(api, seed, "History retry and refresh", 26, true);
    await seedHistoryScenarios(backend, seed, [scenario]);
    const fault = await historyFaultControl(page);
    const watcher = watchWs(page);
    await page.goto(`/t/${scenario.taskId}?sessionId=${scenario.parentId}`);
    const parent = page.locator(
      `[data-testid="session-chat"][data-session-id="${scenario.parentId}"]:visible`,
    );
    const chip = parent.getByTestId("errands-chip");
    await expect(chip).toBeVisible();
    await dwell(
      page,
      300,
      "negative-assertion",
      "History must remain demand-only before disclosure",
    );
    expect(fault.requestCount()).toBe(0);
    await activate(chip);
    const surface = page.getByTestId(mobile ? "errands-drawer-content" : "errands-popover");
    const toggle = surface.getByTestId(`errands-history-toggle-${scenario.parentId}`);
    await activate(toggle);
    const retry = surface.getByTestId(`errands-history-retry-${scenario.parentId}`);
    await expect(retry).toBeVisible();
    expect(fault.requestCount()).toBe(1);
    await expect(surface.getByTestId("errands-stop-all")).toBeEnabled();
    const firstPage = watcher.waitForResponse("errand.history_page");
    await activate(retry);
    await firstPage;
    const rows = surface.getByTestId(`errands-history-rows-${scenario.parentId}`);
    await expect(rows.locator("li")).toHaveCount(25);
    const inventory = await api.wsRequest<{ errands: Errand[] }>("errand.list", {
      task_id: scenario.taskId,
    });
    const finished = inventory.errands.find(
      (row) => row.status === "done" && row.result_state === "delivered",
    );
    expect(finished).toBeDefined();
    fault.notify(finished!);
    const refresh = surface.getByTestId(`errands-history-refresh-${scenario.parentId}`);
    await expect(refresh).toBeVisible();
    await expect(
      surface.getByTestId(`errands-history-load-more-${scenario.parentId}`),
    ).toBeDisabled();
    const beforeRefresh = fault.requestCount();
    await dwell(
      page,
      300,
      "negative-assertion",
      "A changed history chain waits for explicit Refresh",
    );
    expect(fault.requestCount()).toBe(beforeRefresh);
    const refreshed = watcher.waitForResponse("errand.history_page");
    await activate(refresh);
    await refreshed;
    await expect(refresh).toHaveCount(0);
    await expect(rows.locator("li")).toHaveCount(25);
    await expect(rows).toHaveAttribute("data-range-start", "0");
    if (mobile) {
      await surface.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter((animation) =>
              Number.isFinite(animation.effect?.getComputedTiming().iterations),
            )
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      const box = await surface.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.y).toBeGreaterThanOrEqual(0);
      expect(box!.y + box!.height).toBeLessThanOrEqual(page.viewportSize()!.height + 1);
    }
    await page.screenshot({
      path: testInfo.outputPath("history-retry-refresh.png"),
      animations: "disabled",
    });
    const reloadSummary = watcher.waitForResponse("errand.browser_summary", {
      requestWhere: (payload) => payload.task_id === scenario.taskId,
    });
    const beforeReload = fault.requestCount();
    await page.reload();
    await reloadSummary;
    await expect(chip).toBeVisible();
    await dwell(
      page,
      300,
      "negative-assertion",
      "Reload must not reopen or fetch historical pages",
    );
    expect(fault.requestCount()).toBe(beforeReload);
    await activate(chip);
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(parent.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
  } finally {
    await restore();
  }
}
