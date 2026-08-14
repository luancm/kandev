import { expect, test } from "../../fixtures/test-base";
import {
  fillMoveOverrides,
  MOVE_INSTRUCTIONS,
  seedMoveOverrideFixture,
  waitForMoveRequest,
} from "./workflow-step-move-overrides-helpers";

test("moves immediately with one-shot options from the desktop stepper", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Desktop Move Override",
  );
  const target = fixture.session.stepperStep("Verify");
  await target.hover();
  await testPage.getByTestId(`workflow-step-${fixture.targetStepId}-move-options`).click();

  const form = testPage.getByTestId("workflow-move-options");
  await expect(form).toBeVisible();
  await fillMoveOverrides(testPage, fixture.profileId);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-move-submit").click();

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
});
