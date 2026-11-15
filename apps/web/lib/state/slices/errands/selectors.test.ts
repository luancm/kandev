import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { ERRAND_TASK_ID, makeErrand } from "./errands.test-helpers";
import { isErrandSession, selectErrandBySessionId } from "./selectors";

describe("selectErrandBySessionId", () => {
  it("finds the errand whose ID is the session ID", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ errand_id: "b", status: "needs_attention" }));
    const state = store.getState();
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
