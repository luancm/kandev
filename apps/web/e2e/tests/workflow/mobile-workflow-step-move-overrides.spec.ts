import { expect, test } from "../../fixtures/test-base";
import {
  fillMoveOverrides,
  MOVE_INSTRUCTIONS,
  seedMoveOverrideFixture,
  waitForMoveRequest,
} from "./workflow-step-move-overrides-helpers";

test("moves immediately with one-shot options from the mobile stepper Drawer", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Mobile Move Override",
  );
  await fixture.session.stepperStep("Verify").tap();
  await testPage.getByTestId(`workflow-step-${fixture.targetStepId}-move-options`).tap();

  const form = testPage.getByTestId("workflow-move-options");
  await expect(form).toBeVisible();
  const bounds = await form.boundingBox();
  const viewport = testPage.viewportSize();
  expect(bounds).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport!.width);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport!.height);
  await expect(testPage.getByTestId("workflow-move-submit")).toHaveCSS("min-height", "44px");

  await fillMoveOverrides(testPage, fixture.profileId);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-move-submit").tap();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
      agent_profile_id: fixture.profileId,
      model: "mock-smart",
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  await expect(fixture.session.chat.getByText(MOVE_INSTRUCTIONS, { exact: false }).first()).toBeVisible(
    { timeout: 30_000 },
  );
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
