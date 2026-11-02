import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import type { Task, TaskSession } from "@/lib/types/http";
import { resolveTaskRoute } from "./resolve-task-route";

afterEach(() => vi.restoreAllMocks());

it("returns only owned sessions with a matching total and honors the selected conversation", async () => {
  vi.spyOn(api, "fetchTask").mockResolvedValue({
    id: "task",
    primary_session_id: "primary",
  } as Task);
  vi.spyOn(api, "listTaskSessions").mockResolvedValue({
    sessions: [
      { id: "foreign", task_id: "other-task" },
      { id: "primary", task_id: "task" },
      { id: "secondary", task_id: "task" },
    ] as TaskSession[],
    total: 3,
  });
  const resolved = await resolveTaskRoute("task", "secondary");
  expect(resolved.sessionId).toBe("secondary");
  expect(resolved.allSessionsResponse.sessions.map((session) => session.id)).toEqual([
    "primary",
    "secondary",
  ]);
  expect(resolved.allSessionsResponse.total).toBe(2);
});

// @covers AC-TASKS-ERRANDS-005.6
it.each([undefined, "missing", "child"])(
  "filters automatic route candidates with primary %s",
  async (primary) => {
    vi.spyOn(api, "fetchTask").mockResolvedValue({
      id: "task",
      primary_session_id: primary,
    } as Task);
    const sessions = [
      { id: "child", task_id: "task", role: "errand" },
      { id: "normal", task_id: "task" },
    ] as TaskSession[];
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions, total: 2 });
    expect((await resolveTaskRoute("task")).sessionId).toBe("normal");
    expect((await resolveTaskRoute("task", "child")).sessionId).toBe("child");
    vi.mocked(api.listTaskSessions).mockResolvedValue({ sessions: [sessions[0]], total: 1 });
    expect((await resolveTaskRoute("task")).sessionId).toBeUndefined();
    expect((await resolveTaskRoute("task", "child")).allSessionsResponse.total).toBe(1);
  },
);
