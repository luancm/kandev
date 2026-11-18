import { expect, test } from "../../fixtures/test-base";
import { listErrands, startErrands, waitDelivered, waitParentIdle } from "../../helpers/errands";

test("the ordinary parent sends an owned Errand a new instruction and receives its result", async ({
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  try {
    const { taskId, parentId, errands, gates } = await startErrands(api, seed, backend, {
      count: 1,
    });
    const errandId = errands[0].errand_id;
    await gates[0].release();
    await waitDelivered(api, taskId, [errandId]);
    await waitParentIdle(api, taskId, parentId);

    const marker = "ERRAND_PARENT_FOLLOW_UP";
    const instruction = {
      task_id: taskId,
      session_id: errandId,
      prompt: `e2e:message("${marker}")`,
    };
    await api.addUserMessage(
      taskId,
      parentId,
      `e2e:mcp:kandev:message_task_kandev(${JSON.stringify(instruction)})`,
    );
    await expect
      .poll(
        async () => (await listErrands(api, taskId)).find((row) => row.errand_id === errandId),
        {
          message: "The authenticated parent's instruction produces a new delivered result",
          timeout: 30_000,
        },
      )
      .toEqual(
        expect.objectContaining({
          result_state: "delivered",
          result_preview: expect.stringContaining(marker),
        }),
      );
    expect((await api.listTaskSessions(taskId)).sessions.map(({ id }) => id)).toEqual([parentId]);
  } finally {
    await restore();
  }
});
