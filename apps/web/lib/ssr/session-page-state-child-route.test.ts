import { expect, it, vi } from "vitest";
import type { Task, TaskSession } from "@/lib/types/http";
import { fetchTaskNavigationData } from "./session-page-state";

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

  const result = await fetchTaskNavigationData(task.id, childId);

  expect(result.sessionId).toBe(childId);
  expect(result.initialState.taskSessionsByTask?.itemsByTaskId[task.id]).toEqual([parent]);
  expect(Object.keys(result.initialState.taskSessions?.items ?? {})).toEqual([parent.id]);
  expect(mocks.inspectErrand).toHaveBeenCalledExactlyOnceWith(task.id, childId);
});
