import { beforeEach, expect, it, vi } from "vitest";
import type { Task, TaskSession } from "@/lib/types/http";
import { buildTaskNavigationShellData } from "./session-page-state";
import { assertRequestedSession } from "@/src/task-session-admission";
import { createAppStore } from "@/lib/state/store";

const mocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskSession: vi.fn(),
  inspectErrand: vi.fn(),
  listAgents: vi.fn(),
  listRepositories: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listOrdinaryTaskSessions: vi.fn(),
  listWorkflowSteps: vi.fn(),
  listWorkflows: vi.fn(),
  listWorkspaces: vi.fn(),
}));

vi.mock("@/lib/api", () => mocks);
vi.mock("@/lib/api/domains/workflow-api", () => ({ listWorkflowSteps: mocks.listWorkflowSteps }));
vi.mock("@/lib/api/domains/session-api", () => ({ listSessionTurns: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => ({ inspectErrand: mocks.inspectErrand }));
vi.mock("@/lib/api/domains/user-shell-api", () => ({ fetchTerminals: vi.fn() }));

const task = {
  id: "task-1",
  workspace_id: "workspace-1",
  workflow_id: "",
  workflow_step_id: "",
  position: 0,
  title: "Task",
  description: "",
  state: "TODO",
  priority: "medium",
  created_at: "2026-07-16T12:00:00Z",
  updated_at: "2026-07-16T12:00:00Z",
} as Task;

beforeEach(() => vi.resetAllMocks());

it("keeps a directly requested child selected outside ordinary membership", async () => {
  const parent = {
    id: "session-parent",
    task_id: task.id,
    state: "COMPLETED",
    started_at: task.created_at,
    updated_at: task.updated_at,
  } as TaskSession;
  const childId = "errand-child";
  mocks.fetchTask.mockResolvedValue(task);
  mocks.listOrdinaryTaskSessions.mockResolvedValue({ sessions: [parent], total: 1 });
  mocks.inspectErrand.mockResolvedValue({
    scope: { task_id: task.id, child_session_id: childId },
    errand: { task_id: task.id, errand_id: childId, parent_session_id: parent.id },
    child_session_exists: true,
    parent: { task_id: task.id, task_environment_id: "environment-1" },
  });

  const identity = { task, allSessionsResponse: await mocks.listOrdinaryTaskSessions(task.id) };
  const admission = await assertRequestedSession(createAppStore(), identity, task.id, childId);
  const result = buildTaskNavigationShellData(
    { ...identity, selectedErrandInspection: admission!.inspection },
    childId,
  );

  expect(result.sessionId).toBe(childId);
  expect(result.initialState.taskSessionsByTask?.itemsByTaskId[task.id]).toEqual([parent]);
  expect(Object.keys(result.initialState.taskSessions?.items ?? {})).toEqual([parent.id]);
  expect(mocks.inspectErrand).toHaveBeenCalledExactlyOnceWith(
    task.id,
    childId,
    expect.any(AbortSignal),
  );
});

it("admits an ordinary session without any inspection and excludes foreign sessions", async () => {
  const ordinary = { id: "normal", task_id: task.id } as TaskSession;
  const foreign = { id: "foreign", task_id: "other" } as TaskSession;
  const identity = { task, allSessionsResponse: { sessions: [foreign, ordinary], total: 2 } };
  const admission = await assertRequestedSession(createAppStore(), identity, task.id, ordinary.id);
  expect(admission).toBeUndefined();
  expect(mocks.inspectErrand).not.toHaveBeenCalled();
  const shell = buildTaskNavigationShellData(identity, ordinary.id);
  expect(shell.sessionId).toBe(ordinary.id);
  expect(shell.initialState.taskSessionsByTask?.itemsByTaskId[task.id]).toEqual([ordinary]);
});

it("admits a retained child with the feature off and an unavailable parent", async () => {
  const store = createAppStore();
  expect(store.getState().features.errands).toBe(false);
  const identity = { task, allSessionsResponse: { sessions: [], total: 0 } };
  const inspection = {
    scope: { task_id: task.id, child_session_id: "child" },
    errand: { task_id: task.id, errand_id: "child", parent_session_id: "missing" },
    child_session_exists: true,
    parent: { available: false },
  };
  mocks.inspectErrand.mockResolvedValue(inspection);
  const admission = await assertRequestedSession(store, identity, task.id, "child");
  expect(admission?.retain("route")).toBe(true);
  expect(store.getState().errands.detailBySessionId.child).toEqual(inspection);
  const shell = buildTaskNavigationShellData(
    { ...identity, selectedErrandInspection: admission!.inspection },
    "child",
  );
  expect(shell.sessionId).toBe("child");
  expect(shell.initialState.taskSessionsByTask?.itemsByTaskId[task.id]).toEqual([]);
});

it("does not fall back to a parent when the requested child does not exist", async () => {
  const parent = { id: "parent", task_id: task.id } as TaskSession;
  const identity = { task, allSessionsResponse: { sessions: [parent], total: 1 } };
  mocks.inspectErrand.mockResolvedValue({
    scope: { task_id: task.id, child_session_id: "child" },
    errand: { task_id: task.id, errand_id: "child", parent_session_id: parent.id },
    child_session_exists: false,
  });
  await expect(
    assertRequestedSession(createAppStore(), identity, task.id, "child"),
  ).rejects.toMatchObject({ reason: "unavailable" });
});
