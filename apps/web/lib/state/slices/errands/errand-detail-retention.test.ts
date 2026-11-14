import { expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/ids";
import { makeBrowserErrandRow, makeInspection } from "./errands.test-helpers";

const TIMESTAMP = "2026-01-01T00:00:00Z";

function loadChild(store: ReturnType<typeof createAppStore>, id: string) {
  const row = makeBrowserErrandRow({ errand_id: id });
  store.getState().retainErrandInspection("reader", makeInspection(row));
  store.getState().setTaskSession({
    id: sessionId(id),
    task_id: taskId(row.task_id),
    role: "errand",
    parent_session_id: sessionId(row.parent_session_id),
    state: "COMPLETED",
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  });
  store.getState().addMessage({
    id: `message-${id}`,
    session_id: sessionId(id),
    task_id: taskId(row.task_id),
    author_type: "agent",
    type: "message",
    content: "Retained child transcript",
    created_at: TIMESTAMP,
  });
  return makeInspection(row);
}

it("releases inspected child session and transcript caches after the final reader leaves", () => {
  const store = createAppStore();
  const inspection = loadChild(store, "child");
  store.getState().retainErrandInspection("second-reader", inspection);
  store.getState().releaseErrandInspection("reader");
  expect(store.getState().taskSessions.items.child).toBeDefined();
  expect(store.getState().messages.bySession.child).toHaveLength(1);
  store.getState().releaseErrandInspection("second-reader");
  expect(store.getState().errands.detailBySessionId.child).toBeUndefined();
  expect(store.getState().taskSessions.items.child).toBeUndefined();
  expect(store.getState().messages.bySession.child).toBeUndefined();
});

it("does not accumulate full child caches when inspecting many different finished errands", () => {
  const store = createAppStore();
  for (let index = 0; index < 100; index++) {
    loadChild(store, `child-${index}`);
    store.getState().releaseErrandInspection("reader");
  }
  expect(Object.keys(store.getState().errands.detailBySessionId)).toHaveLength(0);
  expect(Object.keys(store.getState().taskSessions.items)).toHaveLength(0);
  expect(Object.keys(store.getState().messages.bySession)).toHaveLength(0);
});

it("does not materialize unseen child details from background session events", () => {
  const store = createAppStore();
  for (let index = 0; index < 100; index++) {
    store.getState().upsertTaskSessionFromEvent("task-1", {
      id: sessionId(`unseen-child-${index}`),
      task_id: taskId("task-1"),
      role: "errand",
      parent_session_id: sessionId("session-parent"),
      state: "COMPLETED",
      started_at: TIMESTAMP,
      updated_at: TIMESTAMP,
    });
  }
  expect(Object.keys(store.getState().taskSessions.items)).toHaveLength(0);
  expect(store.getState().taskSessionsByTask.itemsByTaskId["task-1"] ?? []).toHaveLength(0);
});

it("preserves ordinary transcript and shared environment data when releasing a child", () => {
  const store = createAppStore();
  store.getState().setTaskSession({
    id: sessionId("parent"),
    task_id: taskId("task-1"),
    state: "WAITING_FOR_INPUT",
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  });
  store.getState().registerSessionEnvironment("parent", "shared");
  store.getState().appendShellOutput("parent", "ordinary shell output");
  loadChild(store, "child");
  store.getState().registerSessionEnvironment("child", "shared");
  store.getState().releaseErrandInspection("reader");
  expect(store.getState().taskSessions.items.parent).toBeDefined();
  expect(store.getState().environmentIdBySessionId.parent).toBe("shared");
  expect(store.getState().shell.outputs.shared).toContain("ordinary shell output");
  expect(store.getState().errands.removedSessionIdsByTaskId["task-1"]).toBeUndefined();
});
