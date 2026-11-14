import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import * as errandApi from "@/lib/api/domains/errand-api";
import type { Task, TaskSession } from "@/lib/types/http";
import { resolveTaskRoute } from "./resolve-task-route";

afterEach(() => vi.restoreAllMocks());

it("returns only owned sessions with a matching total and honors the selected conversation", async () => {
  const inspect = vi.spyOn(errandApi, "inspectErrand");
  vi.spyOn(api, "fetchTask").mockResolvedValue({
    id: "task",
    primary_session_id: "primary",
  } as Task);
  vi.spyOn(api, "listOrdinaryTaskSessions").mockResolvedValue({
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
  expect(inspect).not.toHaveBeenCalled();
});

// @covers AC-TASKS-ERRANDS-005.6
it.each([undefined, "missing", "child"])(
  "filters automatic route candidates with primary %s",
  async (primary) => {
    vi.spyOn(api, "fetchTask").mockResolvedValue({
      id: "task",
      primary_session_id: primary,
    } as Task);
    const normal = { id: "normal", task_id: "task" } as TaskSession;
    vi.spyOn(api, "listOrdinaryTaskSessions").mockResolvedValue({ sessions: [normal], total: 1 });
    expect((await resolveTaskRoute("task")).sessionId).toBe("normal");
    expect((await resolveTaskRoute("task", "normal")).sessionId).toBe("normal");
    expect((await resolveTaskRoute("task")).allSessionsResponse.total).toBe(1);
  },
);

it("resolves one directly requested child without adding it to ordinary membership", async () => {
  vi.spyOn(api, "fetchTask").mockResolvedValue({
    id: "task",
    primary_session_id: "parent",
  } as Task);
  vi.spyOn(api, "listOrdinaryTaskSessions").mockResolvedValue({
    sessions: [{ id: "parent", task_id: "task" }] as TaskSession[],
    total: 1,
  });
  const inspection = {
    scope: { task_id: "task", child_session_id: "child" },
    errand: { task_id: "task", errand_id: "child", parent_session_id: "parent" },
    child_session_exists: true,
    parent: { task_id: "task", task_environment_id: "environment" },
  } as unknown as Awaited<ReturnType<typeof errandApi.inspectErrand>>;
  vi.spyOn(errandApi, "inspectErrand").mockResolvedValue(inspection);

  const resolved = await resolveTaskRoute("task", "child");

  expect(resolved.sessionId).toBe("child");
  expect(resolved.selectedErrandInspection).toEqual(inspection);
  expect(resolved.allSessionsResponse.sessions.map((session) => session.id)).toEqual(["parent"]);
  expect(errandApi.inspectErrand).toHaveBeenCalledExactlyOnceWith("task", "child");
});

it("does not select primary when a requested child inspection is unavailable", async () => {
  vi.spyOn(api, "fetchTask").mockResolvedValue({
    id: "task",
    primary_session_id: "parent",
  } as Task);
  vi.spyOn(api, "listOrdinaryTaskSessions").mockResolvedValue({
    sessions: [{ id: "parent", task_id: "task" }] as TaskSession[],
    total: 1,
  });
  vi.spyOn(errandApi, "inspectErrand").mockResolvedValue({
    scope: { task_id: "task", child_session_id: "child" },
    errand: { task_id: "task", errand_id: "child", parent_session_id: "parent" },
    child_session_exists: false,
    parent: { task_id: "task", task_environment_id: "environment" },
  } as unknown as Awaited<ReturnType<typeof errandApi.inspectErrand>>);

  await expect(resolveTaskRoute("task", "child")).rejects.toThrow(
    "requested task session is unavailable",
  );
});
