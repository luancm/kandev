import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  parentChat,
  startErrands,
} from "../../helpers/errands";

const DISCLOSURE = "errands-inline-disclosure";
const INLINE = "errands-inline-summary";
const CHIP = "errands-chip";
const BACK = "errand-back-to-parent";

test("phone errands expand inline, open full screen, return and stop through touch controls", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await expect(chat.getByTestId(DISCLOSURE)).toHaveAttribute("aria-expanded", "false");
    await expectTouchTarget(chat.getByTestId(DISCLOSURE));
    await chat.getByTestId(DISCLOSURE).tap();
    const open = chat.getByTestId(INLINE).getByTestId(`errand-open-${errands[0].errand_id}`);
    await expectTouchTarget(open);
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("errands-phone-inline.png") });
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
    const drawer = page.getByTestId("errands-drawer");
    await expect(drawer).toBeVisible();
    const stopAll = drawer.getByTestId("errands-stop-all");
    await expectTouchTarget(stopAll);
    const bounds = await drawer.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
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
  test("keeps the inline disclosure and Open targets at least 44px", async ({
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
      await expectTouchTarget(chat.getByTestId(DISCLOSURE));
      await chat.getByTestId(DISCLOSURE).click();
      await expectTouchTarget(
        chat.getByTestId(INLINE).getByTestId(`errand-open-${errands[0].errand_id}`),
      );
      await expectTouchTarget(chat.getByTestId(INLINE).getByTestId("errands-stop-all"));
      await expectNoHorizontalOverflow(page);
    } finally {
      await restore();
    }
  });
});
