import { expect, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { watchWs } from "./causal-waits";
import {
  expectNoHorizontalOverflow,
  expectTouchTarget,
  listErrands,
  parentChat,
  startErrands,
  waitDelivered,
  waitNewParentTurnSettled,
} from "./errands";

export async function exerciseOwnedErrandView({
  page,
  api,
  seed,
  backend,
  phone,
  stop = false,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  phone: boolean;
  stop?: boolean;
}) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let release: (() => Promise<void>) | undefined;
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 1,
    });
    release = gates[0].release;
    const errandId = errands[0].errand_id;
    try {
      await expect
        .poll(
          async () =>
            (await listErrands(api, taskId)).find((e) => e.errand_id === errandId)?.status,
        )
        .toBe("running");
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
    await page.goto(`/t/${taskId}`);
    const parent = parentChat(page, parentId);
    const chip = parent.getByTestId("errands-chip");
    await (phone ? chip.tap() : chip.click());
    const choices = page.getByTestId(phone ? "errands-drawer-content" : "errands-popover");
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
    try {
      await expect(
        conversation.getByText("Queue blocking turn started", { exact: true }),
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

    if (stop) {
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
      await release();
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
    const back = conversation.getByTestId("errand-back-to-parent");
    if (phone) await expectTouchTarget(back);
    await (phone ? back.tap() : back.click());
    await expect(conversation).toHaveCount(0);
    await expect(parent).toBeVisible();
  } finally {
    try {
      await release?.();
    } finally {
      await restore();
    }
  }
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
