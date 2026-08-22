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
  const target = testPage.getByTestId("workflow-step-Verify");
  await target.hover();

  // The compact popover keeps the direct Move here action; the collapsed
  // Options disclosure expands the one-shot fields inside the same surface.
  await testPage.getByTestId(`workflow-step-${fixture.targetStepId}-move-options`).click();
  await expect(testPage.getByTestId("workflow-move-agent-profile")).toBeVisible();
  await expect(testPage.getByTestId("workflow-move-model")).toHaveCount(0);
  await fillMoveOverrides(testPage, fixture.profileId);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-step-move-here").click();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
      agent_profile_id: fixture.profileId,
    },
  });
  await expect(fixture.session.stepperStep("Verify")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  await expect(fixture.session.chat.getByText(MOVE_INSTRUCTIONS, { exact: false }).first()).toBeVisible(
    { timeout: 30_000 },
  );
});
