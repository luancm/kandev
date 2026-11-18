import { readFile } from "node:fs/promises";
import type { Message } from "../../../lib/types/http";
import { expect, test } from "../../fixtures/test-base";
import { listErrands, startErrands, waitNewParentTurnSettled } from "../../helpers/errands";

test("an owned Worktree Errand delivers its original turn after a backend restart", async ({
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(180_000);
  const restore = await backend.useEnv({
    KANDEV_FEATURES_ERRANDS: "true",
    KANDEV_FEATURES_AGENT_SURVIVAL: "true",
  });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 1,
    });
    const errandId = errands[0].errand_id;
    const transcript = async () =>
      (
        await api.wsRequest<{ messages: Message[] }>("errand.messages", {
          task_id: taskId,
          errand_id: errandId,
        })
      ).messages;
    const startText = "Queue blocking turn started";
    await expect
      .poll(async () => (await transcript()).filter((message) => message.content === startText))
      .toHaveLength(1);
    const [prompt] = (await transcript()).filter((message) => message.author_type === "user");
    const turnId = prompt?.turn_id;
    expect(turnId).toBeTruthy();
    const previousParentMessages = new Set(
      (await api.listSessionMessages(parentId)).messages.map(({ id }) => id),
    );
    const previousParentTurns = (await api.listSessionTurns(parentId)).turns.map(({ id }) => id);

    await backend.restart();
    await gates[0].release();
    try {
      await expect
        .poll(
          async () => (await listErrands(api, taskId)).find((row) => row.errand_id === errandId),
          {
            timeout: 60_000,
            message: "The retained owned turn finishes and delivers after backend restart",
          },
        )
        .toEqual(
          expect.objectContaining({
            result_state: "delivered",
            result_preview: expect.stringContaining("ERRAND_RESULT_1"),
          }),
        );
    } catch (error) {
      const logs = await readFile(backend.logPath, "utf8").catch(() => "");
      const recovery = logs
        .split("\n")
        .filter(
          (line) =>
            line.includes(errandId) ||
            /startup session recovery summary|failed to (connect to updates stream|load durable delivery replay cursor)|durable agent event processing failed/.test(
              line,
            ),
        )
        .filter((line) => !/authorization|secret/i.test(line))
        .slice(-32);
      const retained = (await transcript()).map(({ author_type, content, turn_id, type }) => ({
        author_type,
        content,
        turn_id,
        type,
      }));
      throw new Error(`${String(error)}\n${JSON.stringify(retained)}\n${recovery.join("\n")}`, {
        cause: error,
      });
    }
    const messages = await transcript();
    const assistantText = messages
      .filter((message) => message.author_type === "agent")
      .map((message) => message.content)
      .join("\n");
    expect(assistantText.split(startText)).toHaveLength(2);
    expect(messages.filter((message) => message.author_type === "user")).toHaveLength(1);
    expect(
      messages.find(
        (message) => message.content.includes("ERRAND_RESULT_1") && message.author_type === "agent",
      )?.turn_id,
    ).toBe(turnId);
    await waitNewParentTurnSettled(api, parentId, previousParentTurns);
    const parentMessages = await api.listSessionMessages(parentId);
    expect(
      parentMessages.messages.filter(
        (message) =>
          !previousParentMessages.has(message.id) &&
          message.raw_content?.includes("ERRAND_RESULT_1"),
      ),
    ).toHaveLength(1);
    expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);
  } finally {
    await restore();
  }
});
