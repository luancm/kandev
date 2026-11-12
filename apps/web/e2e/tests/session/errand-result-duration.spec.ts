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
  listErrands,
  parentChat,
  startErrands,
  waitDelivered,
  waitNewParentTurnSettled,
} from "../../helpers/errands";
import { mockTurnGate } from "../../helpers/mock-turn-gate";
import { dwell } from "../../helpers/causal-waits";

test("desktop finished result duration is keyboard accessible and stays immutable after follow-up", async ({
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

    // This is the only timestamp separation: the result producer computes elapsed wall time.
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
    const originalSnapshots = await deliverySnapshots(api, parentId);
    expect(originalSnapshots).toHaveLength(1);

    const longHistory = await seedLongDurationHistory(api, parentId);
    const snapshotsBeforeChildFollowup = await deliverySnapshots(api, parentId);
    await page.goto(`/t/${taskId}?sessionId=${parentId}`);
    const chat = parentChat(page, parentId);
    const deliveryMessages = chat.getByTestId("errand-delivery-message");
    await expect(deliveryMessages).toHaveCount(2);
    const actualMessage = deliveryMessages.nth(0);
    const actualCard = actualMessage.getByTestId("errand-finished-card");
    const disclosure = actualCard.getByTestId("errand-finished-card-disclosure");
    const composer = chat.locator('.tiptap.ProseMirror[contenteditable="true"]');
    const originalLabel = await actualCard
      .getByTestId("errand-finished-summary-duration")
      .innerText();
    await disclosure.scrollIntoViewIfNeeded();
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");

    // Start at the prior agent response's visible Copy control and tab into the following card.
    const parentReturnedMessage = chat
      .getByTestId("agent-message-highlight")
      .filter({ hasText: "Errands parent returned" });
    await expect(parentReturnedMessage).toHaveCount(1);
    const copyButton = parentReturnedMessage.getByRole("button", {
      name: "Copy message to clipboard",
    });
    const rawTextButton = parentReturnedMessage.getByRole("button", { name: "Show raw text" });
    const metadataButton = parentReturnedMessage.getByRole("button", {
      name: "Show message metadata",
    });
    const favoriteButton = parentReturnedMessage.getByRole("button", {
      name: "Mark message as favorite",
    });
    await parentReturnedMessage.hover();
    await expect(copyButton).toBeVisible();
    await copyButton.click();
    await expect(copyButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(rawTextButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(metadataButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(favoriteButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(disclosure).toBeFocused();
    await page.keyboard.press("Space");
    await expect(disclosure).toHaveAttribute("aria-expanded", "true");
    await expect(actualCard.getByTestId("errand-finished-duration")).toHaveText(originalLabel);
    await expect(actualCard.getByTestId("errand-finished-preview")).toContainText(
      "ERRAND_RESULT_1",
    );
    await expect(actualCard.getByTestId(`errand-open-${errand.errand_id}`)).toBeVisible();

    const longMessage = deliveryMessages.nth(1);
    const longCard = longMessage.getByTestId("errand-finished-card");
    const longDisclosure = longCard.getByTestId("errand-finished-card-disclosure");
    await expect(longDisclosure).toHaveAttribute("aria-expanded", "false");
    await longDisclosure.click();
    const longLabel = await longCard.getByTestId("errand-finished-summary-duration").innerText();
    expect(longLabel.length).toBeGreaterThan(8);
    await expect(longDisclosure).toHaveAttribute("aria-expanded", "true");
    await expect(longCard.getByTestId("errand-finished-duration")).toHaveText(longLabel);
    await expect(longCard.getByTestId(`errand-open-${longHistory.errandId}`)).toBeVisible();
    await expectNoHorizontalOverflow(page);

    await actualCard.getByTestId(`errand-open-${errand.errand_id}`).click();
    const child = parentChat(page, errand.errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await child.getByTestId("errand-back-to-parent").click();
    await expect(composer).toBeVisible();
    await composer.fill("desktop duration composer remains usable");
    await expect(composer).toContainText("desktop duration composer remains usable");
    await composer.fill("");

    // Inspecting the retained child must leave its completed runtime closed.
    expect((await api.getTaskSession(errand.errand_id)).session.state).toBe("COMPLETED");
    const childTurnsBeforeFollowup = (await api.listSessionTurns(errand.errand_id)).turns.map(
      (turn) => turn.id,
    );
    await actualCard.getByTestId(`errand-open-${errand.errand_id}`).click();
    const childComposer = child.locator('.tiptap.ProseMirror[contenteditable="true"]');
    await expect(childComposer).toBeVisible();
    await childComposer.fill('e2e:message("DURATION_FOLLOWUP_RESULT")');
    await child.getByTestId("submit-message-button").click();
    await waitNewTurnSettled(api, errand.errand_id, childTurnsBeforeFollowup);
    await expect
      .poll(async () => (await api.getTaskSession(errand.errand_id)).session.state)
      .toBe("COMPLETED");
    await child.getByTestId("errand-back-to-parent").click();
    await actualCard.getByTestId(`errand-open-${errand.errand_id}`).click();
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
    await child.getByTestId("errand-back-to-parent").click();
    await expect(chat).toBeVisible();
    await expect(actualCard.getByTestId("errand-finished-summary-duration")).toHaveText(
      originalLabel,
    );

    await page.reload();
    const reloadedChat = parentChat(page, parentId);
    await expect(reloadedChat.getByTestId("errand-delivery-message")).toHaveCount(2);
    const reloadedActual = reloadedChat
      .getByTestId("errand-delivery-message")
      .nth(0)
      .getByTestId("errand-finished-card");
    await expect(reloadedActual.getByTestId("errand-finished-summary-duration")).toHaveText(
      originalLabel,
    );
    await expect(reloadedActual.getByTestId("errand-finished-card-disclosure")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshotsBeforeChildFollowup);
    await expectNoHorizontalOverflow(page);
    await page.screenshot({ path: testInfo.outputPath("errand-result-duration-desktop.png") });
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
});
