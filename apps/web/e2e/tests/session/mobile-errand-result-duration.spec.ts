import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  expectRecordedDuration,
  recordedDurationSnapshot,
  recordedDurationSnapshotsForErrand,
  seedLongDurationHistory,
  waitNewTurnSettled,
} from "../../helpers/errand-result-duration";
import {
  deliverySnapshots,
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  parentChat,
  startErrands,
  waitDelivered,
  waitNewParentTurnSettled,
  waitParentIdle,
} from "../../helpers/errands";
import { mockTurnGate } from "../../helpers/mock-turn-gate";
import { dwell } from "../../helpers/causal-waits";

test("phone finished result duration survives reload and long history fits around Open and the composer", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let gates: Array<ReturnType<typeof mockTurnGate>> = [];
  try {
    const fixture = await startErrands(api, seed, backend, { count: 1 });
    const { taskId, parentId, errands } = fixture;
    gates = fixture.gates;
    const errand = errands[0];
    const beforeTurns = (await api.listSessionTurns(parentId)).turns.map((turn) => turn.id);

    await dwell(
      page,
      1_100,
      "clock-separation",
      "The recorded-result fixture needs a positive creation-to-settlement interval",
    );
    await gates[0].release();
    await waitDelivered(api, taskId, [errand.errand_id]);
    await waitNewParentTurnSettled(api, parentId, beforeTurns);

    const settledErrand = (await listErrands(api, taskId)).find(
      (row) => row.errand_id === errand.errand_id,
    );
    if (!settledErrand) throw new Error("Expected the first errand to remain in task history");
    const originalSnapshot = await recordedDurationSnapshot(api, parentId, errand.errand_id);
    expectRecordedDuration(originalSnapshot, settledErrand);
    const longHistory = await seedLongDurationHistory(api, parentId);
    const snapshotsBeforeChildFollowup = await deliverySnapshots(api, parentId);

    await page.goto(`/t/${taskId}?sessionId=${parentId}`);
    const chat = parentChat(page, parentId);
    await expect(chat.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
    const initialMessages = chat.getByTestId("errand-delivery-message");
    await expect(initialMessages).toHaveCount(2);
    const initialMessage = initialMessages.nth(0);
    const actualCard = initialMessage.getByTestId("errand-finished-card");
    const originalLabel = await actualCard
      .getByTestId("errand-finished-summary-duration")
      .innerText();
    const disclosure = actualCard.getByTestId("errand-finished-card-disclosure");
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");
    await expectTouchTarget(disclosure);
    await disclosure.tap();
    await expect(disclosure).toHaveAttribute("aria-expanded", "true");
    await expect(actualCard.getByTestId("errand-finished-duration")).toHaveText(originalLabel);

    const open = actualCard.getByTestId(`errand-open-${errand.errand_id}`);
    await expectTouchTarget(open);
    await open.tap();
    const child = parentChat(page, errand.errand_id);
    await expect(child.getByTestId("errand-header")).toHaveAttribute("data-layout", "phone");
    const back = child.getByTestId("errand-back-to-parent");
    await expectTouchTarget(back);
    await back.tap();
    const composer = chat.locator('.tiptap.ProseMirror[contenteditable="true"]');
    await expect(composer).toBeVisible();
    await composer.fill("phone duration composer remains usable");
    await expect(composer).toContainText("phone duration composer remains usable");
    await composer.fill("");

    const childTurnsBeforeFollowup = (await api.listSessionTurns(errand.errand_id)).turns.map(
      (turn) => turn.id,
    );
    await api.addUserMessage(taskId, errand.errand_id, 'e2e:message("DURATION_FOLLOWUP_RESULT")');
    await waitNewTurnSettled(api, errand.errand_id, childTurnsBeforeFollowup);
    await waitParentIdle(api, taskId, errand.errand_id);
    await open.tap();
    const followedUpErrand = (await listErrands(api, taskId)).find(
      (row) => row.errand_id === errand.errand_id,
    );
    if (!followedUpErrand) throw new Error("Expected the followed-up errand to remain in history");
    await expect(
      child.getByText("DURATION_FOLLOWUP_RESULT", { exact: false }).last(),
    ).toBeVisible();
    expect(await recordedDurationSnapshotsForErrand(api, parentId, errand.errand_id)).toEqual([
      originalSnapshot,
    ]);
    await child.getByTestId("errand-back-to-parent").tap();
    await expect(chat).toBeVisible();
    await expect(actualCard.getByTestId("errand-finished-summary-duration")).toHaveText(
      originalLabel,
    );

    await page.reload();
    const reloadedChat = parentChat(page, parentId);
    const reloadedDeliveryMessages = reloadedChat.getByTestId("errand-delivery-message");
    await expect(reloadedDeliveryMessages).toHaveCount(2);
    const reloadedActualMessage = reloadedDeliveryMessages.nth(0);
    const reloadedActual = reloadedActualMessage.getByTestId("errand-finished-card");
    await expect(reloadedActual.getByTestId("errand-finished-summary-duration")).toHaveText(
      originalLabel,
    );
    await expect(reloadedActual.getByTestId("errand-finished-card-disclosure")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshotsBeforeChildFollowup);

    const longMessage = reloadedDeliveryMessages.nth(1);
    await expect(
      longMessage.locator(
        `[data-testid="errand-finished-card"][data-errand-id="${longHistory.errandId}"]`,
      ),
    ).toHaveCount(1);
    const longCard = longMessage.getByTestId("errand-finished-card");
    const longDisclosure = longCard.getByTestId("errand-finished-card-disclosure");
    const longLabel = await longCard.getByTestId("errand-finished-summary-duration").innerText();
    expect(longLabel.length).toBeGreaterThan(8);
    await longCard.scrollIntoViewIfNeeded();
    await longDisclosure.scrollIntoViewIfNeeded();
    await expectTouchTarget(longDisclosure);
    await expect(longDisclosure).toHaveAttribute("aria-expanded", "false");
    await longDisclosure.tap();
    await expect(longDisclosure).toHaveAttribute("aria-expanded", "true");
    await expect(longCard.getByTestId("errand-finished-duration")).toHaveText(longLabel);
    const longOpen = longCard.getByTestId(`errand-open-${longHistory.errandId}`);
    await expectTouchTarget(longOpen);
    await expect(longOpen).toBeInViewport();
    await expectNoHorizontalOverflow(page);
    await expect(reloadedChat.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath("errand-result-duration-phone-long.png") });
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});
