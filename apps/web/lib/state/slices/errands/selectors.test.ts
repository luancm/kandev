import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { ERRAND_TASK_ID, makeErrand } from "./errands.test-helpers";
import { selectErrandBySessionId } from "./selectors";

describe("selectErrandBySessionId", () => {
  it("finds the owned Errand by task and Errand ID", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ errand_id: "b", status: "needs_attention" }));
    const state = store.getState();
    expect(selectErrandBySessionId(state, ERRAND_TASK_ID, "b")?.title).toBe("API audit");
    expect(selectErrandBySessionId(state, ERRAND_TASK_ID, "session-parent")).toBeUndefined();
    expect(selectErrandBySessionId(state, null, "b")).toBeUndefined();
  });
});
