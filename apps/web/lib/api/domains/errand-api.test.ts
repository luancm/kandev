import { beforeEach, describe, expect, it, vi } from "vitest";
import { makeErrand } from "@/lib/state/slices/errands/errands.test-helpers";

const request = vi.hoisted(() => vi.fn());
const client = vi.hoisted(() => ({ current: { request } as { request: typeof request } | null }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => client.current }));

import { cancelAllErrands, cancelErrand, listErrands, sendErrandResult } from "./errand-api";

const ERRAND_SESSION_ID = "session-errand";

describe("errand API", () => {
  beforeEach(() => {
    request.mockReset();
    client.current = { request };
  });

  it("lists a task's errands", async () => {
    request.mockResolvedValue({ errands: [makeErrand()] });

    await expect(listErrands("task-1")).resolves.toEqual([makeErrand()]);
    expect(request).toHaveBeenCalledWith("errand.list", { task_id: "task-1" });
  });

  it("treats a response without an errand list as empty", async () => {
    request.mockResolvedValue({ errands: null });
    await expect(listErrands("task-1")).resolves.toEqual([]);
  });

  it("cancels one errand by its session ID", async () => {
    request.mockResolvedValue({});
    await cancelErrand(ERRAND_SESSION_ID);

    expect(request).toHaveBeenCalledWith(
      "errand.cancel",
      { session_id: ERRAND_SESSION_ID },
      expect.any(Number),
    );
  });

  it("cancels every live errand of a task and returns the count", async () => {
    request.mockResolvedValue({ cancelled: 3 });

    await expect(cancelAllErrands("task-1")).resolves.toBe(3);
    expect(request).toHaveBeenCalledWith(
      "errand.cancel_all",
      { task_id: "task-1" },
      expect.any(Number),
    );
  });

  it("sends an errand's latest result to its parent", async () => {
    request.mockResolvedValue({});
    await sendErrandResult(ERRAND_SESSION_ID);

    expect(request).toHaveBeenCalledWith(
      "errand.send_result",
      { session_id: ERRAND_SESSION_ID },
      expect.any(Number),
    );
  });

  it("propagates a backend rejection", async () => {
    request.mockRejectedValue(new Error("errand_result_not_eligible"));
    await expect(sendErrandResult(ERRAND_SESSION_ID)).rejects.toThrow("errand_result_not_eligible");
  });

  it("fails without a WebSocket client", async () => {
    client.current = null;
    await expect(listErrands("task-1")).rejects.toThrow();
    expect(request).not.toHaveBeenCalled();
  });
});
