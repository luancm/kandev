import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { ERRAND_PARENT_ID, ERRAND_TASK_ID, makeErrand } from "./errands.test-helpers";
import {
  isErrandSession,
  selectErrandBySessionId,
  selectErrandsByParent,
  selectLiveErrandCount,
  selectParentErrands,
  selectTaskErrands,
} from "./selectors";

function seededStore() {
  const store = createAppStore();
  store.getState().setTaskErrands(
    ERRAND_TASK_ID,
    [
      makeErrand({ errand_id: "c", created_at: "2026-10-26T10:00:03Z", status: "done" }),
      makeErrand({ errand_id: "a", created_at: "2026-10-26T10:00:01Z", status: "queued" }),
      makeErrand({
        errand_id: "b",
        created_at: "2026-10-26T10:00:02Z",
        status: "needs_attention",
        parent_session_id: "session-other",
      }),
      makeErrand({ errand_id: "d", created_at: "2026-10-26T10:00:04Z", status: "cancelled" }),
    ],
    0,
  );
  return store;
}

describe("selectTaskErrands", () => {
  it("lists a task's errands in creation order", () => {
    const state = seededStore().getState();
    expect(selectTaskErrands(state, ERRAND_TASK_ID).map((e) => e.errand_id)).toEqual([
      "a",
      "b",
      "c",
      "d",
    ]);
  });

  it("breaks creation-time ties by errand ID", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ errand_id: "z" }));
    store.getState().upsertErrand(makeErrand({ errand_id: "y" }));

    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID).map((e) => e.errand_id)).toEqual([
      "y",
      "z",
    ]);
  });

  it("returns the same array until the task's errands change", () => {
    const store = seededStore();
    const first = selectTaskErrands(store.getState(), ERRAND_TASK_ID);
    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)).toBe(first);

    store
      .getState()
      .upsertErrand(makeErrand({ errand_id: "e", created_at: "2026-10-26T10:00:05Z" }));
    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)).not.toBe(first);
  });

  it("returns one shared empty list for unknown or missing tasks", () => {
    const state = createAppStore().getState();
    expect(selectTaskErrands(state, "task-unknown")).toEqual([]);
    expect(selectTaskErrands(state, null)).toBe(selectTaskErrands(state, "task-unknown"));
  });
});

describe("selectErrandsByParent", () => {
  it("groups a task's errands by parent session in creation order", () => {
    const state = seededStore().getState();
    const groups = selectErrandsByParent(state, ERRAND_TASK_ID);

    expect(Object.keys(groups).sort()).toEqual(["session-other", ERRAND_PARENT_ID]);
    expect(groups[ERRAND_PARENT_ID].map((e) => e.errand_id)).toEqual(["a", "c", "d"]);
    expect(groups["session-other"].map((e) => e.errand_id)).toEqual(["b"]);
  });

  it("is stable between reads and exposes one parent's errands", () => {
    const state = seededStore().getState();
    expect(selectErrandsByParent(state, ERRAND_TASK_ID)).toBe(
      selectErrandsByParent(state, ERRAND_TASK_ID),
    );
    expect(
      selectParentErrands(state, ERRAND_TASK_ID, "session-other").map((e) => e.errand_id),
    ).toEqual(["b"]);
    expect(selectParentErrands(state, ERRAND_TASK_ID, "session-none")).toEqual([]);
  });
});

describe("selectLiveErrandCount", () => {
  it("counts queued, running, and needs-attention errands only", () => {
    const store = seededStore();
    expect(selectLiveErrandCount(store.getState(), ERRAND_TASK_ID)).toBe(2);

    store
      .getState()
      .upsertErrand(
        makeErrand({ errand_id: "a", status: "done", updated_at: "2026-10-26T10:01:00Z" }),
      );
    expect(selectLiveErrandCount(store.getState(), ERRAND_TASK_ID)).toBe(1);
  });

  it("is zero for a task without errands", () => {
    expect(selectLiveErrandCount(createAppStore().getState(), ERRAND_TASK_ID)).toBe(0);
  });
});

describe("selectErrandBySessionId", () => {
  it("finds the errand whose ID is the session ID", () => {
    const state = seededStore().getState();
    expect(selectErrandBySessionId(state, ERRAND_TASK_ID, "b")?.title).toBe("API audit");
    expect(selectErrandBySessionId(state, ERRAND_TASK_ID, "session-parent")).toBeUndefined();
    expect(selectErrandBySessionId(state, null, "b")).toBeUndefined();
  });
});

describe("isErrandSession", () => {
  const session = (overrides: Partial<TaskSession>) => ({ id: "s-1", ...overrides }) as TaskSession;

  it("recognizes the errand role and treats a missing role as ordinary", () => {
    const state = createAppStore().getState();
    expect(isErrandSession(state, ERRAND_TASK_ID, session({ role: "errand" }))).toBe(true);
    expect(isErrandSession(state, ERRAND_TASK_ID, session({}))).toBe(false);
  });

  it("recognizes a session whose role has not arrived but whose errand row has", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ errand_id: "s-1" }));

    expect(isErrandSession(store.getState(), ERRAND_TASK_ID, session({}))).toBe(true);
    expect(isErrandSession(store.getState(), "task-other", session({}))).toBe(false);
  });
});
