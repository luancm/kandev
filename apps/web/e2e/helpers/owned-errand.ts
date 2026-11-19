import { expect, type Locator, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { createAutomaticPermissionProfile } from "./automatic-permission-presentation";
import { watchWs } from "./causal-waits";
import {
  expectNoHorizontalOverflow,
  expectOwnedErrandContinuationRetry,
  expectTouchTarget,
  listErrands,
  parentChat,
  readOwnedMessages,
  startErrands,
  waitDelivered,
  waitNewParentTurnSettled,
} from "./errands";

export async function exerciseOwnedErrandDeepLink({
  page,
  api,
  seed,
  backend,
  phone,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  phone: boolean;
}) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 1,
    });
    const errandId = errands[0].errand_id;
    const previousTurns = (await api.listSessionTurns(parentId)).turns.map(({ id }) => id);
    await gates[0].release();
    await waitDelivered(api, taskId, [errandId]);
    await waitNewParentTurnSettled(api, parentId, previousTurns);

    const socket = watchWs(page);
    const inspected = socket.waitForResponse("errand.inspect", {
      requestWhere: (payload) =>
        payload.task_id === taskId && payload.child_session_id === errandId,
    });
    await page.goto(`/t/${taskId}?sessionId=${encodeURIComponent(errandId)}`);
    expect(await inspected).toMatchObject({ type: "response" });
    const conversation = page.getByTestId("errand-conversation");
    await expect(conversation).toHaveAttribute("data-errand-id", errandId);
    await expect(conversation.getByTestId("errand-header")).toHaveAttribute(
      "data-layout",
      phone ? "phone" : "desktop",
    );
    await expect(
      conversation.getByText(/^(?:Queue blocking turn started\s*)?ERRAND_RESULT_1$/),
    ).toBeVisible();
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${errandId}"]`),
    ).toHaveCount(0);
    if (phone) await expectNoHorizontalOverflow(page);

    await returnToErrandParent(conversation, parentChat(page, parentId), phone);
    expect(new URL(page.url()).searchParams.get("sessionId")).toBe(parentId);
    expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);
  } finally {
    await restore();
  }
}

export async function exerciseOwnedErrandView({
  page,
  api,
  seed,
  backend,
  phone,
  stop = false,
  permission = false,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  phone: boolean;
  stop?: boolean;
  permission?: boolean;
}) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let release: (() => Promise<void>) | undefined;
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 1,
      prompts: permission ? ["/e2e:permission-flow"] : undefined,
    });
    release = gates[0]?.release;
    const errandId = errands[0].errand_id;
    try {
      await expect
        .poll(
          async () =>
            (await listErrands(api, taskId)).find((e) => e.errand_id === errandId)?.status,
        )
        .toMatch(permission ? /^(running|needs_attention)$/ : /^running$/);
    } catch (error) {
      const logs = await readFile(backend.logPath, "utf8").catch(() => "");
      const launch = logs
        .split("\n")
        .filter((line) => /errand\.(started|admitted)|launch errand|start_errand/.test(line))
        .slice(-5);
      const parent = await api.listSessionMessages(parentId);
      const receipts = parent.messages.slice(-8).map((message) => ({
        type: message.type,
        content: message.content.slice(0, 250),
        normalized: JSON.stringify(message.metadata?.normalized)?.slice(0, 750),
      }));
      throw new Error(`${String(error)}\n${launch.join("\n")}\n${JSON.stringify(receipts)}`, {
        cause: error,
      });
    }
    expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);

    const socket = watchWs(page);
    const { parent, conversation } = await openOwnedErrandConversation(
      page,
      taskId,
      parentId,
      errandId,
      { phone },
    );
    try {
      await expect(
        permission
          ? conversation.getByTestId("permission-approve")
          : conversation.getByText("Queue blocking turn started", { exact: true }),
      ).toBeVisible();
    } catch (error) {
      const logs = await readFile(backend.logPath, "utf8").catch(() => "");
      const relevant = logs
        .split("\n")
        .filter(
          (line) =>
            line.includes(errandId) || /delivery|agent event|prompt.*(fail|start)/i.test(line),
        )
        .filter((line) => !/authorization|secret/i.test(line))
        .slice(-40);
      throw new Error(`${String(error)}\n${relevant.join("\n")}`, { cause: error });
    }
    if (phone) await expectNoHorizontalOverflow(page);

    if (permission) {
      await expectStalePermissionRefused(api, taskId, errandId);
      await approveOwnedErrand(page, socket, taskId, errandId, phone);
      expect(await readOwnedPermission(api, taskId, errandId)).toMatchObject({
        session_id: "",
        metadata: {
          status: "approved",
          permission_resolution: { option_id: "allow", source: "web", result: "accepted" },
        },
      });
      await waitDelivered(api, taskId, [errandId]);
      expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);
    } else if (stop) {
      await stopOwnedErrand(page, socket, errandId, phone);
      await expect
        .poll(
          async () =>
            (await listErrands(api, taskId)).find((row) => row.errand_id === errandId)?.status,
        )
        .toBe("cancelled");
      await expect(
        conversation.getByText("Queue blocking turn started", { exact: true }),
      ).toBeVisible();
    } else {
      const previousTurns = (await api.listSessionTurns(parentId)).turns.map(({ id }) => id);
      const transcriptChanged = socket.waitForEvent("errand.transcript_changed", {
        where: (payload) => payload.task_id === taskId && payload.errand_id === errandId,
      });
      await release?.();
      await transcriptChanged;
      await expect(
        conversation.getByText(/^(?:Queue blocking turn started\s*)?ERRAND_RESULT_1$/),
      ).toBeVisible();
      await waitDelivered(api, taskId, [errandId]);
      await waitNewParentTurnSettled(api, parentId, previousTurns);
      expect(
        (await api.listSessionMessages(parentId)).messages.some((message) =>
          message.raw_content?.includes("ERRAND_RESULT_1"),
        ),
      ).toBe(true);
    }
    await returnToErrandParent(conversation, parent, phone);
  } finally {
    try {
      await release?.();
    } finally {
      await restore();
    }
  }
}

export async function exerciseOwnedErrandShellOutput({
  page,
  api,
  seed,
  backend,
  phone,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  phone: boolean;
}) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const script = [
      'e2e:shell_stream({"id":"owned","action":"start","command":"inspect-output","seed_bytes":4096})',
      'e2e:shell_stream({"id":"owned","action":"finish","stdout":"OWNED_STDOUT","stderr":"OWNED_STDERR","exit_code":0})',
      'e2e:message("owned output ready")',
    ].join("\n");
    const { taskId, parentId, errands } = await startErrands(api, seed, backend, {
      prompts: [script],
    });
    const errandId = errands[0].errand_id;
    await waitDelivered(api, taskId, [errandId]);
    const reads: string[] = [];
    page.on("request", (request) => {
      if (request.url().endsWith("/shell-output")) reads.push(request.url());
    });
    const { conversation } = await openOwnedErrandConversation(page, taskId, parentId, errandId, {
      phone,
    });
    const disclosure = conversation.getByRole("button", { name: "Show command output" });
    await expect(disclosure).toBeVisible();
    await expect(conversation.getByText("OWNED_STDOUT", { exact: true })).toHaveCount(0);
    expect(reads).toHaveLength(0);
    if (phone) await disclosure.tap();
    else await disclosure.click();
    const output = conversation.getByTestId("tool-execute-output");
    await expect(output).toContainText("OWNED_STDOUT");
    await expect(output).toContainText("OWNED_STDERR");
    expect(reads).toHaveLength(1);
    expect(reads[0]).toContain(`/api/v1/tasks/${taskId}/errands/${errandId}/messages/`);
    if (phone) await expectNoHorizontalOverflow(page);
  } finally {
    await restore();
  }
}

export async function exerciseOwnedErrandAutomaticPermission({
  page,
  api,
  seed,
  backend,
  phone,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  phone: boolean;
}) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const profile = await createAutomaticPermissionProfile(api, seed.agentProfileId);
    const { taskId, parentId, errands } = await startErrands(api, seed, backend, {
      prompts: ["/e2e:permission-flow"],
      errandAgentProfileId: profile.id,
    });
    const errandId = errands[0].errand_id;
    await waitDelivered(api, taskId, [errandId]);
    expect(await readOwnedPermission(api, taskId, errandId)).toMatchObject({
      session_id: "",
      metadata: {
        status: "approved",
        permission_presentation: "automatic",
        permission_resolution: { option_id: "allow", source: "auto_approve", result: "accepted" },
      },
    });
    const { conversation, parent } = await openOwnedErrandConversation(
      page,
      taskId,
      parentId,
      errandId,
      { phone, finished: true },
    );
    await expect(
      conversation.getByText("Permission was granted and command executed."),
    ).toBeVisible();
    await expect(conversation.getByTestId("permission-action-row")).toHaveCount(0);
    await returnToErrandParent(conversation, parent, phone);
  } finally {
    await restore();
  }
}

export async function exerciseOwnedErrandContinuation({
  page,
  api,
  seed,
  backend,
  phone = false,
  browser = false,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  phone?: boolean;
  browser?: boolean;
}) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 1,
    });
    const errandId = errands[0].errand_id;
    await gates[0].release();
    await waitDelivered(api, taskId, [errandId]);
    const request = {
      task_id: taskId,
      errand_id: errandId,
      client_message_id: "owned-errand-follow-up",
      content: 'e2e:message("ERRAND_FOLLOW_UP")',
    };
    if (browser) {
      await sendOwnedErrandFollowUp(page, { taskId, parentId, errandId }, request.content, phone);
    } else {
      await expectOwnedErrandContinuationRetry(api, backend, request);
      // A later generation needs the first follow-up's own close receipt.
      await expectOwnedErrandContinuationRetry(api, backend, {
        ...request,
        client_message_id: "owned-errand-second-follow-up",
        content: 'e2e:message("ERRAND_SECOND_FOLLOW_UP")',
      });
    }

    const outputs = browser
      ? ["ERRAND_FOLLOW_UP"]
      : ["ERRAND_FOLLOW_UP", "ERRAND_SECOND_FOLLOW_UP"];
    await expect
      .poll(async () => readOwnedMessages(api, taskId, errandId))
      .toEqual(
        expect.arrayContaining(
          outputs.map((content) => expect.objectContaining({ content, session_id: "" })),
        ),
      );
    const messages = await readOwnedMessages(api, taskId, errandId);
    expect(messages.filter((message) => message.content === request.content)).toHaveLength(1);
    expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);
    expect(await listErrands(api, taskId)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          errand_id: errandId,
          result_state: "delivered",
          result_preview: expect.stringContaining("ERRAND_RESULT_1"),
        }),
      ]),
    );

    if (!browser) {
      const { conversation } = await openOwnedErrandConversation(page, taskId, parentId, errandId, {
        phone,
        finished: true,
      });
      await expect(conversation.getByText("ERRAND_FOLLOW_UP", { exact: true })).toBeVisible();
    }
  } finally {
    await restore();
  }
}

async function sendOwnedErrandFollowUp(
  page: Page,
  { taskId, parentId, errandId }: { taskId: string; parentId: string; errandId: string },
  content: string,
  phone: boolean,
) {
  const { conversation } = await openOwnedErrandConversation(page, taskId, parentId, errandId, {
    phone,
    finished: true,
  });
  const composer = conversation.getByTestId("errand-continuation");
  await expect(composer).toBeVisible();
  const send = composer.getByRole("button", { name: "Send" });
  if (phone) await expectTouchTarget(send);
  await composer.locator('.tiptap.ProseMirror[contenteditable="true"]').fill(content);
  await send.click();
  await expect(conversation.getByText("ERRAND_FOLLOW_UP", { exact: true })).toBeVisible();
  if (phone) await expectNoHorizontalOverflow(page);
}

async function returnToErrandParent(conversation: Locator, parent: Locator, phone: boolean) {
  const back = conversation.getByTestId("errand-back-to-parent");
  if (phone) await expectTouchTarget(back);
  await (phone ? back.tap() : back.click());
  await expect(conversation).toHaveCount(0);
  await expect(parent).toBeVisible();
}

async function openOwnedErrandConversation(
  page: Page,
  taskId: string,
  parentId: string,
  errandId: string,
  { phone, finished = false }: { phone: boolean; finished?: boolean },
) {
  await page.goto(`/t/${taskId}`);
  const parent = parentChat(page, parentId);
  const chip = parent.getByTestId("errands-chip");
  await (phone ? chip.tap() : chip.click());
  const choices = page.getByTestId(phone ? "errands-drawer-content" : "errands-popover");
  if (finished) {
    const history = choices.getByTestId(`errands-history-toggle-${parentId}`);
    await (phone ? history.tap() : history.click());
  }
  const open = choices.getByTestId(
    phone ? `errands-panel-row-${errandId}` : `errand-open-${errandId}`,
  );
  await expect(open).toBeVisible();
  if (phone) await expectTouchTarget(open);
  await (phone ? open.tap() : open.click());

  const conversation = page.getByTestId("errand-conversation");
  await expect(conversation).toBeVisible();
  await expect(conversation).toHaveAttribute("data-errand-id", errandId);
  await expect(conversation.getByTestId("errand-header")).toHaveAttribute(
    "data-layout",
    phone ? "phone" : "desktop",
  );
  await expect(
    page.locator(`[data-testid="session-chat"][data-session-id="${errandId}"]`),
  ).toHaveCount(0);
  return { parent, conversation };
}

async function readOwnedPermission(api: ApiClient, taskId: string, errandId: string) {
  const messages = await readOwnedMessages(api, taskId, errandId);
  return messages.find((message) => message.type === "permission_request");
}

async function expectStalePermissionRefused(api: ApiClient, taskId: string, errandId: string) {
  const message = await readOwnedPermission(api, taskId, errandId);
  expect(message?.metadata?.pending_id).toEqual(expect.any(String));
  await expect(
    api.wsRequest("errand.permission.respond", {
      task_id: taskId,
      errand_id: errandId,
      request_id: "stale-provider-request",
      pending_id: message?.metadata?.pending_id,
      option_id: "allow",
    }),
  ).rejects.toThrow("permission_stale");
}

async function approveOwnedErrand(
  page: Page,
  socket: ReturnType<typeof watchWs>,
  taskId: string,
  errandId: string,
  phone: boolean,
) {
  const conversation = page.getByTestId("errand-conversation");
  const approve = conversation.getByTestId("permission-approve");
  if (phone) await expectTouchTarget(approve);
  const confirmed = socket.waitForResponse("errand.permission.respond", {
    requestWhere: (payload) =>
      payload.task_id === taskId &&
      payload.errand_id === errandId &&
      typeof payload.request_id === "string" &&
      typeof payload.pending_id === "string" &&
      payload.option_id === "allow" &&
      !("session_id" in payload),
  });
  await (phone ? approve.tap() : approve.click());
  await confirmed;
  await expect(approve).toHaveCount(0);
  await expect(
    conversation.getByText("Permission was granted and command executed."),
  ).toBeVisible();
}

async function stopOwnedErrand(
  page: Page,
  socket: ReturnType<typeof watchWs>,
  errandId: string,
  phone: boolean,
) {
  const conversation = page.getByTestId("errand-conversation");
  if (phone) await conversation.getByTestId("errand-actions-menu").tap();
  const confirmed = socket.waitForResponse("errand.cancel", {
    requestWhere: (payload) => payload.session_id === errandId,
  });
  if (phone) {
    await page.getByRole("menuitem", { name: "Stop", exact: true }).tap();
  } else {
    await conversation.getByTestId("errand-stop-button").click();
  }
  await confirmed;
}
