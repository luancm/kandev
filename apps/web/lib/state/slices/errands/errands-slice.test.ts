import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { ERRAND_TASK_ID, makeErrand } from "./errands.test-helpers";

const OLDER = "2026-10-26T10:00:00Z";
const NEWER = "2026-10-26T10:00:05.250Z";

function taskErrands(store: ReturnType<typeof createAppStore>, taskId = ERRAND_TASK_ID) {
  return store.getState().errands.byTaskId[taskId] ?? {};
}

describe("errands slice", () => {
  it("starts empty in the root store", () => {
    const store = createAppStore();
    expect(store.getState().errands).toEqual({
      byTaskId: {},
      loadedByTaskId: {},
      liveRevisionByTaskId: {},
    });
  });
});

describe("upsertErrand", () => {
  it("stores the errand under its task and errand ID", () => {
    const store = createAppStore();
    const errand = makeErrand();
    store.getState().upsertErrand(errand);

    expect(taskErrands(store)).toEqual({ "errand-1": errand });
  });

  it("replaces an errand with a newer or equal update", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ status: "running", updated_at: OLDER }));
    store.getState().upsertErrand(makeErrand({ status: "done", updated_at: NEWER }));
    expect(taskErrands(store)["errand-1"].status).toBe("done");

    store.getState().upsertErrand(makeErrand({ status: "failed", updated_at: NEWER }));
    expect(taskErrands(store)["errand-1"].status).toBe("failed");
  });

  it("ignores an update older than the stored errand", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ status: "done", updated_at: NEWER }));
    store.getState().upsertErrand(makeErrand({ status: "running", updated_at: OLDER }));

    expect(taskErrands(store)["errand-1"].status).toBe("done");
  });

  it("accepts an update when a timestamp cannot be compared", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ status: "done", updated_at: NEWER }));
    store.getState().upsertErrand(makeErrand({ status: "cancelled", updated_at: "2026-02-30" }));

    expect(taskErrands(store)["errand-1"].status).toBe("cancelled");
  });

  it("keeps tasks independent", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ task_id: "task-a" }));
    store.getState().upsertErrand(makeErrand({ task_id: "task-b", errand_id: "errand-2" }));

    expect(Object.keys(taskErrands(store, "task-a"))).toEqual(["errand-1"]);
    expect(Object.keys(taskErrands(store, "task-b"))).toEqual(["errand-2"]);
  });

  it("advances the task's live revision", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand());
    store.getState().upsertErrand(makeErrand({ updated_at: NEWER }));

    expect(store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID]).toBe(2);
  });
});

describe("setTaskErrands", () => {
  it("replaces the task's errands and marks the task loaded", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ errand_id: "gone" }));
    const revision = store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID];
    store.getState().setTaskErrands(ERRAND_TASK_ID, [makeErrand()], revision);

    expect(Object.keys(taskErrands(store))).toEqual(["errand-1"]);
    expect(store.getState().errands.loadedByTaskId[ERRAND_TASK_ID]).toBe(true);
  });

  it("keeps errands upserted live while the list request was in flight", () => {
    const store = createAppStore();
    const revision = store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID] ?? 0;
    store.getState().upsertErrand(makeErrand({ errand_id: "started-live" }));
    store.getState().setTaskErrands(ERRAND_TASK_ID, [makeErrand()], revision);

    expect(Object.keys(taskErrands(store)).sort()).toEqual(["errand-1", "started-live"]);
  });

  it("never lets a listed row roll back a newer live update", () => {
    const store = createAppStore();
    const revision = store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID] ?? 0;
    store.getState().upsertErrand(makeErrand({ status: "done", updated_at: NEWER }));
    store
      .getState()
      .setTaskErrands(
        ERRAND_TASK_ID,
        [makeErrand({ status: "running", updated_at: OLDER })],
        revision,
      );

    expect(taskErrands(store)["errand-1"].status).toBe("done");
  });

  it("ignores listed rows that belong to another task", () => {
    const store = createAppStore();
    store.getState().setTaskErrands(ERRAND_TASK_ID, [makeErrand({ task_id: "task-other" })], 0);

    expect(taskErrands(store)).toEqual({});
    expect(taskErrands(store, "task-other")).toEqual({});
  });
});

describe("removeTaskErrands", () => {
  it("drops the task's errands and load state, leaving other tasks", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand());
    store.getState().setTaskErrands("task-b", [makeErrand({ task_id: "task-b" })], 0);
    store.getState().removeTaskErrands(ERRAND_TASK_ID);

    const { errands } = store.getState();
    expect(errands.byTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.loadedByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.liveRevisionByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(Object.keys(taskErrands(store, "task-b"))).toEqual(["errand-1"]);
  });
});
