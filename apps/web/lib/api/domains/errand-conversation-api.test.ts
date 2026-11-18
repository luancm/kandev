import { afterEach, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request }) }));

import { getErrandMessages } from "./errand-api";

const taskId = "task-1";
const errandId = "errand-1";
const page = {
  task_id: taskId,
  errand_id: errandId,
  messages: [
    {
      id: "answer",
      task_id: taskId,
      session_id: "",
      turn_id: "turn-1",
      author_type: "agent",
      type: "message",
      content: "owned answer",
    },
  ],
  has_more: true,
};

afterEach(() => {
  request.mockReset();
});

it("loads the selected Errand page by owned identity", async () => {
  const signal = new AbortController().signal;
  request.mockResolvedValue(page);
  await expect(
    getErrandMessages(taskId, errandId, { beforeMessageId: "older", limit: 20 }, signal),
  ).resolves.toEqual(page);
  expect(request).toHaveBeenCalledWith(
    "errand.messages",
    { task_id: taskId, errand_id: errandId, before_message_id: "older", limit: 20 },
    undefined,
    signal,
  );
});

it("rejects unrelated Errand pages and task-session messages", async () => {
  request.mockResolvedValueOnce({ ...page, errand_id: "other" }).mockResolvedValueOnce({
    ...page,
    messages: [{ ...page.messages[0], session_id: "ordinary-session" }],
  });
  await expect(getErrandMessages(taskId, errandId)).rejects.toThrow();
  await expect(getErrandMessages(taskId, errandId)).rejects.toThrow();
});
