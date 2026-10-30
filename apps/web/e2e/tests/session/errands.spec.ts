import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  deliverySnapshots,
  listErrands,
  parentChat,
  seedErrandContextWindow,
  startErrands,
  waitDelivered,
  waitNewParentTurnSettled,
  waitParentIdle,
} from "../../helpers/errands";
import { mockTurnGate } from "../../helpers/mock-turn-gate";

const INLINE = "errands-inline-summary";
const CHIP = "errands-chip";
const PANEL = "errands-popover";
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

// @covers AC-TASKS-ERRANDS-CONTEXT-001.1 AC-TASKS-ERRANDS-CONTEXT-001.2 AC-TASKS-ERRANDS-CONTEXT-001.3
test("desktop errand rows read each child's persisted context window passively", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 3,
    });
    await expect
      .poll(
        async () => {
          const sessions = new Map(
            (await api.listTaskSessions(taskId)).sessions.map((session) => [
              session.id,
              session.state,
            ]),
          );
          return errands.map((errand) => sessions.get(errand.errand_id));
        },
        { message: "Child errand turns start before their fixture gates release" },
      )
      .toEqual(errands.map(() => "RUNNING"));
    for (const gate of gates) await gate.release();
    await waitDelivered(
      api,
      taskId,
      errands.map((errand) => errand.errand_id),
    );
    await waitParentIdle(api, taskId, parentId);
    await expect
      .poll(
        async () => {
          const sessions = new Map(
            (await api.listTaskSessions(taskId)).sessions.map((session) => [
              session.id,
              session.state,
            ]),
          );
          return errands.map((errand) => sessions.get(errand.errand_id));
        },
        { message: "Child sessions settle before their persisted context fixtures are updated" },
      )
      .toEqual(errands.map(() => "WAITING_FOR_INPUT"));

    const firstId = errands[0].errand_id;
    const unmeasuredId = errands[1].errand_id;
    const unknownId = errands[2].errand_id;
    const childWindow = (used: number) => ({
      size: 100_000,
      used,
      remaining: 100_000 - used,
      efficiency: (used / 100_000) * 100,
      source: "acp" as const,
    });
    await seedErrandContextWindow(api, taskId, parentId, childWindow(95_000));
    await seedErrandContextWindow(api, taskId, firstId, childWindow(68_000));
    await seedErrandContextWindow(api, taskId, unmeasuredId, childWindow(0));

    const statusesBefore = (await listErrands(api, taskId)).map(({ errand_id, status }) => [
      errand_id,
      status,
    ]);
    await page.goto(`/t/${taskId}`);
    const parent = parentChat(page, parentId);
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${firstId}"]`),
    ).toHaveCount(0);
    await parent.getByTestId("errands-chip").click();

    const panel = page.getByTestId("errands-popover");
    const measuredRow = panel.getByTestId(`errands-panel-row-${firstId}`);
    const unmeasuredRow = panel.getByTestId(`errands-panel-row-${unmeasuredId}`);
    const unknownRow = panel.getByTestId(`errands-panel-row-${unknownId}`);
    await expect(measuredRow).toContainText("Context window: 68% used");
    await expect(unmeasuredRow).toContainText("Context window: usage not measured");
    await expect(unknownRow).not.toContainText("Context window");
    await expect(panel.getByTestId(`errand-open-${firstId}`)).toBeVisible();
    await expect(panel.getByTestId("errands-stop-all")).toHaveCount(0);

    await seedErrandContextWindow(api, taskId, firstId, childWindow(82_000));
    await expect(measuredRow).toContainText("Context window: 82% used");
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${firstId}"]`),
    ).toHaveCount(0);
    expect(
      (await listErrands(api, taskId)).map(({ errand_id, status }) => [errand_id, status]),
    ).toEqual(statusesBefore);

    await page.reload();
    const reloadedParent = parentChat(page, parentId);
    await reloadedParent.getByTestId("errands-chip").click();
    const reloadedPanel = page.getByTestId("errands-popover");
    await expect(reloadedPanel.getByTestId(`errands-panel-row-${firstId}`)).toContainText(
      "Context window: 82% used",
    );
    await expect(reloadedPanel.getByTestId(`errands-panel-row-${unmeasuredId}`)).toContainText(
      "Context window: usage not measured",
    );

    await seedErrandContextWindow(api, taskId, firstId, null);
    await expect(reloadedPanel.getByTestId(`errands-panel-row-${firstId}`)).not.toContainText(
      "Context window",
    );
    await page.reload();
    await parentChat(page, parentId).getByTestId("errands-chip").click();
    await expect(
      page.getByTestId("errands-popover").getByTestId(`errands-panel-row-${firstId}`),
    ).not.toContainText("Context window");
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
    // @covers AC-TASKS-ERRANDS-010.11
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    await expect(chat.getByTestId(CHIP)).toHaveCount(1);
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
    await chat.getByTestId(CHIP).click();
    await page.getByTestId(PANEL).getByTestId(`errand-open-${errands[0].errand_id}`).click();
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
    await expect(chat.getByTestId("errand-start-card")).toHaveCount(2);
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    const snapshots = await deliverySnapshots(api, parentId);
    expect(snapshots).toHaveLength(2);
    await page.reload();
    await expect(parentChat(page, parentId).getByTestId(FINISHED)).toHaveCount(2);
    expect(await deliverySnapshots(api, parentId)).toEqual(snapshots);
    await expect(parentChat(page, parentId).getByTestId("errand-start-card")).toHaveCount(2);
    await parentChat(page, parentId).getByTestId(CHIP).click();
    await expect(page.getByTestId(PANEL).getByTestId("errands-stop-all")).toHaveCount(0);
    for (const row of errands)
      await expect(
        page.getByTestId(PANEL).getByTestId(`errands-panel-row-${row.errand_id}`),
      ).toBeVisible();
    await page.getByTestId(PANEL).getByTestId(`errand-open-${errands[0].errand_id}`).click();
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
    await chat.getByTestId(CHIP).click();
    await page.getByTestId(PANEL).getByTestId("errands-stop-all").click();
    await expect
      .poll(
        async () => (await listErrands(api, taskId)).every((row) => row.status === "cancelled"),
        { timeout: 60_000 },
      )
      .toBe(true);
    await expect(page.getByTestId(PANEL).getByTestId("errands-stop-all")).toHaveCount(0);
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
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
      await parentChat(client, parentId).getByTestId(CHIP).click();
      await expect(
        client.getByTestId(PANEL).getByTestId(`errands-panel-row-${errands[0].errand_id}`),
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
        client.getByTestId(PANEL).getByTestId(`errands-panel-row-${errands[0].errand_id}`),
      ).toHaveCount(0);
    await api.wsRequest("errand.cancel_all", { task_id: taskId });
    await restore();
    restore = null;
    await page.reload();
    await expect(page.getByTestId(INLINE)).toHaveCount(0);
    await expect(page.getByTestId(CHIP)).toHaveCount(0);
    const retained = (await api.listTaskSessions(taskId)).sessions.find(
      (row) => row.id === errands[1].errand_id,
    );
    expect(retained).toMatchObject({ role: "errand", is_primary: false });
  } finally {
    await peer.close();
    if (restore) await restore();
  }
});

test("result delivery retains a scrolled-up reader and follows the bottom when enabled", async ({
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
    await expect
      .poll(async () =>
        (await api.listSessionMessages(parentId)).messages.some(
          (message) =>
            message.author_type === "agent" &&
            message.content.includes("Done. Emitted 70 messages."),
        ),
      )
      .toBe(true);
    await waitParentIdle(api, taskId, parentId);
    await page.goto(`/t/${taskId}`);
    const chat = parentChat(page, parentId);
    const scroller = chat.locator(".chat-message-list");
    await expect
      .poll(() => scroller.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    const readerPosition = await scroller.evaluate((node) => {
      node.scrollTop = (node.scrollHeight - node.clientHeight) / 2;
      node.dispatchEvent(new WheelEvent("wheel", { deltaY: -100, bubbles: true }));
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
      return node.scrollTop;
    });
    await expect(chat.getByTestId(INLINE)).toHaveCount(0);
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    const previousTurnIds = (await api.listSessionTurns(parentId)).turns.map((turn) => turn.id);
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await waitNewParentTurnSettled(api, parentId, previousTurnIds);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId(FINISHED)).toHaveCount(1);
    await expect
      .poll(async () =>
        Math.abs((await scroller.evaluate((node) => node.scrollTop)) - readerPosition),
      )
      .toBeLessThan(3);
    await scroller.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
      node.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await gates[1].release();
    await waitDelivered(api, taskId, [errands[1].errand_id]);
    await waitParentIdle(api, taskId, parentId);
    await expect(chat.getByTestId(FINISHED)).toHaveCount(2);
    await expect
      .poll(() =>
        scroller.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
      )
      .toBeLessThan(8);
  } finally {
    await restore();
  }
});
