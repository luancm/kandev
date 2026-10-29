import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  deliverySnapshots,
  listErrands,
  parentChat,
  startErrands,
  waitDelivered,
  waitParentIdle,
} from "../../helpers/errands";
import { mockTurnGate } from "../../helpers/mock-turn-gate";

const INLINE = "errands-inline-summary";
const DISCLOSURE = "errands-inline-disclosure";
const FINISHED = "errand-finished-card";

test("desktop Threads shows bounded counts, nested children and column-owned back navigation", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend);
    await page.goto(
      `/threads?workspace=${seed.workspaceId}&taskId=${taskId}&sessionId=${parentId}`,
    );
    await expect(page.getByTestId("thread-errands-summary").first()).toContainText("2 errands");
    await page.getByTestId("thread-session-picker-trigger").click();
    const child = page.getByTestId(`thread-session-row-${errands[0].errand_id}`);
    await expect(child).toHaveAttribute("data-parent-session-id", parentId);
    await child.click();
    await expect(parentChat(page, errands[0].errand_id).getByTestId("errand-header")).toBeVisible();
    await parentChat(page, errands[0].errand_id).getByTestId("errand-back-to-parent").click();
    await expect(parentChat(page, parentId)).toBeVisible();
  } finally {
    await restore();
  }
});

test("conversational parent, explicit Open, follow-up and immutable delivered cards survive reload", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const fixture = await startErrands(api, seed, backend);
    const { taskId, parentId, errands, gates } = fixture;
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(2);
    await expect(chat.getByTestId(INLINE)).toHaveCount(1);
    await expect(chat.getByTestId(DISCLOSURE)).toHaveAttribute("aria-expanded", "false");
    for (const row of errands)
      await expect(
        page.locator(`[data-testid="session-chat"][data-session-id="${row.errand_id}"]`),
      ).toHaveCount(0);
    const session = new SessionPage(page);
    await session.sendMessage("/e2e:errands-status");
    await expect
      .poll(async () =>
        (await api.listSessionMessages(parentId)).messages.some((message) =>
          message.content.includes("ERRAND_STATUS_BLOCKS=1"),
        ),
      )
      .toBe(true);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByText("ERRAND_STATUS_BLOCKS=1", { exact: false })).toBeVisible();
    await chat.getByTestId(DISCLOSURE).click();
    await chat.getByTestId(INLINE).getByTestId(`errand-open-${errands[0].errand_id}`).click();
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await expect(child.getByTestId(INLINE)).toHaveCount(0);
    expect((await api.listTaskSessions(taskId)).sessions.find((row) => row.is_primary)?.id).toBe(
      parentId,
    );
    await child.getByTestId("errand-back-to-parent").click();
    await gates[0].release();
    await gates[1].release();
    await waitDelivered(
      api,
      taskId,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId(FINISHED)).toHaveCount(2);
    const snapshots = await deliverySnapshots(api, parentId);
    expect(snapshots).toHaveLength(2);
    await page.reload();
    await expect(parentChat(page, parentId).getByTestId(FINISHED)).toHaveCount(2);
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshots);
    await parentChat(page, parentId).getByTestId(DISCLOSURE).click();
    await parentChat(page, parentId)
      .getByTestId(INLINE)
      .getByTestId(`errand-open-${errands[0].errand_id}`)
      .click();
    await api.addUserMessage(taskId, errands[0].errand_id, 'e2e:message("USER_FOLLOWUP_RESULT")');
    await waitParentIdle(api, taskId, errands[0].errand_id);
    await expect(child.getByTestId("errand-send-result-button")).toBeVisible();
    await child.getByTestId("errand-send-result-button").click();
    await expect.poll(async () => (await deliverySnapshots(api, parentId)).length).toBe(3);
    expect((await deliverySnapshots(api, parentId)).slice(0, 2)).toEqual(snapshots);
  } finally {
    await restore();
  }
});

test("Auto-run off supports manual Send now and next-turn delivery without duplicate notifications", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend);
    const identity = await api.getQueueSessionIdentity(taskId, parentId);
    await api.setQueueAutoRun(identity, false);
    await page.goto(`/t/${taskId}`);
    await gates[0].release();
    await expect
      .poll(async () => (await api.getQueueStatus(identity)).count, { timeout: 60_000 })
      .toBe(1);
    expect(await deliverySnapshots(api, parentId)).toHaveLength(0);
    await api.wsRequest(
      "message.queue.send_now",
      {
        task_id: taskId,
        session_id: parentId,
        session_incarnation_id: identity.sessionIncarnationId,
        scope: "all",
      },
      40_000,
    );
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    await api.setQueueAutoRun(identity, false);
    await gates[1].release();
    await expect
      .poll(async () => (await api.getQueueStatus(identity)).count, { timeout: 60_000 })
      .toBe(1);
    await api.addUserMessage(taskId, parentId, "/e2e:errands-status");
    await waitDelivered(api, taskId, [errands[1].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    expect((await api.getQueueStatus(identity)).count).toBe(0);
    await expect(parentChat(page, parentId).getByTestId(FINISHED)).toHaveCount(2);
    expect(await deliverySnapshots(api, parentId)).toHaveLength(2);
  } finally {
    await restore();
  }
});

test("optional await joins the IDs actually returned by start", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      await: true,
    });
    await page.goto(`/t/${taskId}`);
    await expect(parentChat(page, parentId).getByTestId("errand-await-card")).toBeVisible();
    for (const row of errands)
      await expect(page.getByTestId(`errand-await-row-${row.errand_id}`)).toBeVisible();
    await gates[0].release();
    await gates[1].release();
    await waitDelivered(
      api,
      taskId,
      errands.map((row) => row.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect(
      parentChat(page, parentId).getByText("Errands parent returned", { exact: false }),
    ).toBeVisible();
  } finally {
    await restore();
  }
});

test("five-live cap, parent turn cancellation and task-wide Stop remain independent", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend, { count: 6 });
    expect(errands).toHaveLength(5);
    const gate = mockTurnGate(backend.tmpDir, "PARENT_CANCEL_SHOULD_NOT_COMPLETE");
    await api.addUserMessage(taskId, parentId, gate.prompt);
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(taskId)).sessions.find((row) => row.id === parentId)?.state,
      )
      .toBe("RUNNING");
    await api.wsRequest("agent.cancel", { task_id: taskId, session_id: parentId }, 40_000);
    await waitParentIdle(api, taskId, parentId);
    expect(
      (await listErrands(api, taskId)).filter((row) =>
        ["queued", "running", "needs_attention"].includes(row.status),
      ),
    ).toHaveLength(5);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    await chat.getByTestId("errands-stop-all").click();
    await expect
      .poll(
        async () => (await listErrands(api, taskId)).every((row) => row.status === "cancelled"),
        { timeout: 60_000 },
      )
      .toBe(true);
    await expect(chat.getByTestId(DISCLOSURE)).toHaveAttribute("aria-expanded", "false");
  } finally {
    await restore();
  }
});

test("two clients remove deleted errands and flag-off restart retains detached sessions", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  let restore: (() => Promise<void>) | null = await backend.useEnv({
    KANDEV_FEATURES_ERRANDS: "true",
  });
  const peer = await page.context().newPage();
  try {
    const { taskId, parentId, errands } = await startErrands(api, seed, backend);
    for (const client of [page, peer]) {
      await client.goto(`/t/${taskId}`);
      await parentChat(client, parentId).getByTestId(DISCLOSURE).click();
      await expect(
        parentChat(client, parentId).getByTestId(`errands-panel-row-${errands[0].errand_id}`),
      ).toBeVisible();
    }
    await api.wsRequest("errand.cancel", { session_id: errands[0].errand_id });
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(taskId)).sessions.find(
            (row) => row.id === errands[0].errand_id,
          )?.state,
      )
      .not.toMatch(/^(RUNNING|STARTING)$/);
    await api.wsRequest("session.delete", { task_id: taskId, session_id: errands[0].errand_id });
    for (const client of [page, peer])
      await expect(
        parentChat(client, parentId).getByTestId(`errands-panel-row-${errands[0].errand_id}`),
      ).toHaveCount(0);
    await api.wsRequest("errand.cancel_all", { task_id: taskId });
    await restore();
    restore = null;
    await page.reload();
    await expect(page.getByTestId(INLINE)).toHaveCount(0);
    const retained = (await api.listTaskSessions(taskId)).sessions.find(
      (row) => row.id === errands[1].errand_id,
    );
    expect(retained).toMatchObject({ role: "errand", is_primary: false });
  } finally {
    await peer.close();
    if (restore) await restore();
  }
});

test("inline expansion retains a scrolled-up reader and follows the bottom when enabled", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, gates, errands } = await startErrands(api, seed, backend);
    await api.addUserMessage(taskId, parentId, "/bulk:70");
    await waitParentIdle(api, taskId, parentId);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    const scroller = chat.locator(".chat-message-list");
    await expect
      .poll(() => scroller.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    await scroller.evaluate((node) => {
      node.scrollTop = 0;
      node.dispatchEvent(new WheelEvent("wheel", { deltaY: -100, bubbles: true }));
    });
    await chat.getByTestId(DISCLOSURE).evaluate((node: HTMLButtonElement) => node.click());
    await expect.poll(() => scroller.evaluate((node) => node.scrollTop)).toBeLessThan(3);
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await expect.poll(() => scroller.evaluate((node) => node.scrollTop)).toBeLessThan(3);
    await scroller.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await chat.getByTestId(DISCLOSURE).click();
    await chat.getByTestId(DISCLOSURE).click();
    await expect
      .poll(() =>
        scroller.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
      )
      .toBeLessThan(8);
  } finally {
    await restore();
  }
});
