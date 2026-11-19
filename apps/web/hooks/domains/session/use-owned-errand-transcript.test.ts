import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { getErrandMessages } from "@/lib/api/domains/errand-api";
import { sessionId, taskId as asTaskId } from "@/lib/types/http";
import { useOwnedErrandTranscript } from "./use-owned-errand-transcript";

const acknowledge = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/errand-api", () => ({
  getErrandMessages: vi.fn(),
  markErrandRead: acknowledge,
}));

const read = vi.mocked(getErrandMessages);
type Page = Awaited<ReturnType<typeof getErrandMessages>>;
const taskId = "task-1";

function page(errandId: string, id: string, content: string, hasMore = false): Page {
  return {
    task_id: taskId,
    errand_id: errandId,
    messages: [
      {
        id,
        content,
        task_id: asTaskId(taskId),
        session_id: sessionId(""),
        type: "message",
        author_type: "agent",
        created_at: "2026-11-18T03:30:00Z",
      },
    ],
    has_more: hasMore,
  };
}

afterEach(() => {
  read.mockReset();
  acknowledge.mockReset();
});

it("loads an older owned page and refreshes the latest messages without losing history", async () => {
  read
    .mockResolvedValueOnce(page("errand-1", "latest", "partial", true))
    .mockResolvedValueOnce(page("errand-1", "older", "previous turn"))
    .mockResolvedValueOnce(page("errand-1", "latest", "full answer", true));
  const { result, rerender } = renderHook(
    (revision) =>
      useOwnedErrandTranscript({
        taskId,
        errandId: "errand-1",
        readScope: "user-1",
        revision,
        connected: true,
      }),
    { initialProps: 1 },
  );
  await waitFor(() => expect(result.current.messages.map((m) => m.content)).toEqual(["partial"]));
  await act(async () => {
    await result.current.loadMore();
  });
  expect(read).toHaveBeenNthCalledWith(
    2,
    taskId,
    "errand-1",
    { beforeMessageId: "latest" },
    expect.any(AbortSignal),
  );
  expect(result.current.hasMore).toBe(false);
  rerender(2);
  await waitFor(() =>
    expect(result.current.messages.map((m) => m.content)).toEqual(["previous turn", "full answer"]),
  );
  expect(result.current.hasMore).toBe(false);
});

it("aborts reads from the previous identity and never shows their results", async () => {
  let finishOld!: (response: Page) => void;
  read
    .mockImplementationOnce(() => new Promise((resolve) => (finishOld = resolve)))
    .mockResolvedValueOnce(page("errand-2", "new", "current answer"));
  const { result, rerender } = renderHook(
    (errandId) =>
      useOwnedErrandTranscript({
        taskId,
        errandId,
        readScope: "user-1",
        revision: 1,
        connected: true,
      }),
    { initialProps: "errand-1" },
  );
  const previousSignal = read.mock.calls[0]?.[3];
  rerender("errand-2");
  expect(previousSignal?.aborted).toBe(true);
  await act(async () => finishOld(page("errand-1", "old", "previous answer")));
  await waitFor(() =>
    expect(result.current.messages.map((m) => m.content)).toEqual(["current answer"]),
  );
});

it("acknowledges the newest loaded message only while the conversation is visible", async () => {
  read.mockResolvedValueOnce(page("errand-1", "latest", "answer"));
  acknowledge.mockResolvedValue(undefined);
  const { result, rerender } = renderHook(
    (visible) => {
      const visibility = { acknowledgeVisible: visible };
      return useOwnedErrandTranscript({
        ...visibility,
        taskId,
        errandId: "errand-1",
        readScope: "user-1",
        revision: 1,
        connected: true,
      });
    },
    { initialProps: false },
  );
  await waitFor(() => expect(result.current.status).toBe("ready"));
  expect(acknowledge).not.toHaveBeenCalled();
  rerender(true);
  await waitFor(() =>
    expect(acknowledge).toHaveBeenCalledWith(taskId, "errand-1", "latest", expect.any(AbortSignal)),
  );
  rerender(false);
  rerender(true);
  expect(acknowledge).toHaveBeenCalledTimes(1);
});
