import { beforeEach, expect, it, vi } from "vitest";
import type { Task, TaskSession } from "@/lib/types/http";
import { buildTaskNavigationShellData, fetchTaskNavigationEnrichment } from "./session-page-state";
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

it("keeps an owned Errand deep link on its ordinary parent through hydration", async () => {
  const parent = {
    id: "session-parent",
    task_id: task.id,
    state: "COMPLETED",
    task_environment_id: "environment-1",
    started_at: task.created_at,
    updated_at: task.updated_at,
  } as TaskSession;
  const errandId = "owned-errand";
  const identity = { task, allSessionsResponse: { sessions: [parent], total: 1 } };
  const store = createAppStore();
  const inspection = {
    scope: { task_id: task.id, child_session_id: errandId },
    errand: { task_id: task.id, errand_id: errandId, parent_session_id: parent.id },
    child_session_exists: false,
    owns_conversation: true,
    parent: { available: true, id: parent.id, task_environment_id: parent.task_environment_id },
  };
  mocks.inspectErrand.mockResolvedValue(inspection);
  const admission = await assertRequestedSession(store, identity, task.id, errandId);
  expect(admission?.retain("route")).toBe(true);
  const route = { ...identity, selectedErrandInspection: admission!.inspection };
  const shell = buildTaskNavigationShellData(route, errandId);

  expect(shell.sessionId).toBe(parent.id);
  expect(shell.ownedErrand).toEqual(inspection);
  expect(shell.initialState.tasks?.activeSessionId).toBe(parent.id);
  expect(Object.keys(shell.initialState.taskSessions?.items ?? {})).toEqual([parent.id]);

  mocks.fetchTaskSession.mockResolvedValue({ session: parent });
  mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
  mocks.listAgents.mockResolvedValue({ agents: [] });
  const enriched = await fetchTaskNavigationEnrichment(route, errandId);
  expect(enriched.ownedErrand).toEqual(inspection);
  expect(mocks.fetchTaskSession).toHaveBeenCalledExactlyOnceWith(parent.id, { cache: "no-store" });
});

it("keeps an owned retained transcript addressable when the parent is unavailable", async () => {
  const errandId = "retained-errand";
  const identity = { task, allSessionsResponse: { sessions: [], total: 0 } };
  const inspection = {
    scope: { task_id: task.id, child_session_id: errandId },
    errand: { task_id: task.id, errand_id: errandId, parent_session_id: "missing" },
    child_session_exists: false,
    owns_conversation: true,
    parent: { available: false },
  };
  mocks.inspectErrand.mockResolvedValue(inspection);
  const admission = await assertRequestedSession(createAppStore(), identity, task.id, errandId);
  const route = { ...identity, selectedErrandInspection: admission!.inspection };
  const shell = buildTaskNavigationShellData(route, errandId);

  expect(shell.sessionId).toBeNull();
  expect(shell.ownedErrand).toEqual(inspection);
  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();
});

it("rejects an inspection without an owned conversation", async () => {
  const parent = { id: "session-parent", task_id: task.id } as TaskSession;
  const errandId = "unowned-errand";
  const identity = { task, allSessionsResponse: { sessions: [parent], total: 1 } };
  mocks.inspectErrand.mockResolvedValue({
    scope: { task_id: task.id, child_session_id: errandId },
    errand: { task_id: task.id, errand_id: errandId, parent_session_id: parent.id },
    child_session_exists: true,
    parent: { available: true, id: parent.id },
  });
  await expect(
    assertRequestedSession(createAppStore(), identity, task.id, errandId),
  ).rejects.toMatchObject({ reason: "unavailable" });
  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();
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

it("admits a retained Errand with the feature off and an unavailable parent", async () => {
  const store = createAppStore();
  expect(store.getState().features.errands).toBe(false);
  const identity = { task, allSessionsResponse: { sessions: [], total: 0 } };
  const inspection = {
    scope: { task_id: task.id, child_session_id: "child" },
    errand: { task_id: task.id, errand_id: "child", parent_session_id: "missing" },
    child_session_exists: false,
    owns_conversation: true,
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
  expect(shell.sessionId).toBeNull();
  expect(shell.ownedErrand).toEqual(inspection);
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
