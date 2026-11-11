import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { Errand } from "@/lib/types/errands";
import { ERRAND_TASK_ID, makeErrand } from "./errands.test-helpers";

const OLDER = "2026-10-26T10:00:00Z";
const NEWER = "2026-10-26T10:00:05.250Z";

function withDeliveryState(errand: Errand, deliveryState: "received" | "uncertain"): Errand {
  return { ...errand, delivery_state: deliveryState };
}

function taskErrands(store: ReturnType<typeof createAppStore>, taskId = ERRAND_TASK_ID) {
  return store.getState().errands.byTaskId[taskId] ?? {};
}

describe("errands slice", () => {
  it("starts empty in the root store", () => {
    const store = createAppStore();
    expect(store.getState().errands).toEqual({
      byTaskId: {},
      loadedByTaskId: {},
      loadedConnectionGenerationByTaskId: {},
      loadedReadScopeByTaskId: {},
      resetEpochByTaskId: {},
      liveRevisionByTaskId: {},
      liveRevisionByErrandIdByTaskId: {},
      removedSessionIdsByTaskId: {},
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
    store
      .getState()
      .upsertErrand(
        withDeliveryState(makeErrand({ status: "done", updated_at: NEWER }), "uncertain"),
      );
    store.getState().upsertErrand(makeErrand({ status: "running", updated_at: OLDER }));

    expect(taskErrands(store)["errand-1"].status).toBe("done");
    expect(taskErrands(store)["errand-1"]).toHaveProperty("delivery_state", "uncertain");
  });

  it("applies equal-timestamp receipt updates and clears omitted state", () => {
    const store = createAppStore();
    store.getState().upsertErrand(withDeliveryState(makeErrand(), "uncertain"));
    store.getState().upsertErrand(withDeliveryState(makeErrand(), "received"));

    expect(taskErrands(store)["errand-1"]).toHaveProperty("delivery_state", "received");
    expect(taskErrands(store)["errand-1"].updated_at).toBe(OLDER);

    store.getState().upsertErrand(makeErrand());
    expect(taskErrands(store)["errand-1"]).not.toHaveProperty("delivery_state");
  });

  it("does not let an older list result roll back recovery attention from a newer event", () => {
    const store = createAppStore();
    const requestRevision = store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID] ?? 0;
    const recovery = {
      ...makeErrand({ status: "needs_attention", updated_at: NEWER }),
      attention_reason: "recovery_unresolved",
    } as Errand;
    store.getState().upsertErrand(recovery);

    store.getState().setTaskErrands(
      ERRAND_TASK_ID,
      [
        {
          ...makeErrand({ status: "needs_attention", updated_at: OLDER }),
          attention_reason: "permission",
        } as Errand,
      ],
      requestRevision,
    );

    expect(taskErrands(store)["errand-1"].attention_reason).toBe("recovery_unresolved");
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

describe("errand attention projection", () => {
  it("updates reason transitions and preserves prior result data", () => {
    const store = createAppStore();
    const recovery = {
      ...makeErrand({
        status: "needs_attention",
        result_state: "delivered",
        result_preview: "Earlier result remains authoritative",
        result_truncated: true,
      }),
      attention_reason: "recovery_unresolved",
      delivery_state: "uncertain" as const,
    } as Errand;
    store.getState().upsertErrand(recovery);

    store.getState().upsertErrand({
      ...recovery,
      attention_reason: "permission",
      updated_at: NEWER,
    });
    expect(taskErrands(store)["errand-1"].attention_reason).toBe("permission");

    const running = {
      ...recovery,
      status: "running" as const,
      updated_at: "2026-10-26T10:00:10Z",
    };
    delete running.attention_reason;
    store.getState().upsertErrand(running);

    expect(taskErrands(store)["errand-1"]).not.toHaveProperty("attention_reason");
    expect(taskErrands(store)["errand-1"]).toMatchObject({
      status: "running",
      result_state: "delivered",
      result_preview: "Earlier result remains authoritative",
      result_truncated: true,
      delivery_state: "uncertain",
    });
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
    store
      .getState()
      .upsertErrand(
        withDeliveryState(makeErrand({ status: "done", updated_at: NEWER }), "uncertain"),
      );
    store
      .getState()
      .setTaskErrands(
        ERRAND_TASK_ID,
        [makeErrand({ status: "running", updated_at: OLDER })],
        revision,
      );

    expect(taskErrands(store)["errand-1"].status).toBe("done");
    expect(taskErrands(store)["errand-1"]).toHaveProperty("delivery_state", "uncertain");
  });

  it("applies equal-timestamp list projections and clears omitted state", () => {
    const store = createAppStore();
    store.getState().upsertErrand(withDeliveryState(makeErrand(), "uncertain"));
    const revision = store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID] ?? 0;
    store
      .getState()
      .setTaskErrands(ERRAND_TASK_ID, [withDeliveryState(makeErrand(), "received")], revision);

    expect(taskErrands(store)["errand-1"]).toHaveProperty("delivery_state", "received");

    const nextRevision = store.getState().errands.liveRevisionByTaskId[ERRAND_TASK_ID] ?? 0;
    store.getState().setTaskErrands(ERRAND_TASK_ID, [makeErrand()], nextRevision);
    expect(taskErrands(store)["errand-1"]).not.toHaveProperty("delivery_state");
  });

  it("applies the authoritative reason from a reconnect list and clears an omitted legacy reason", () => {
    const store = createAppStore();
    const reasoned = {
      ...makeErrand({ status: "needs_attention" }),
      attention_reason: "recovery_unresolved",
    } as Errand;
    store.getState().setTaskErrands(ERRAND_TASK_ID, [reasoned], 0);
    expect(taskErrands(store)["errand-1"].attention_reason).toBe("recovery_unresolved");

    const permission = {
      ...makeErrand({ status: "needs_attention", updated_at: NEWER }),
      attention_reason: "permission",
    } as Errand;
    store.getState().setTaskErrands(ERRAND_TASK_ID, [permission], 0);
    expect(taskErrands(store)["errand-1"].attention_reason).toBe("permission");

    const legacySnapshot = makeErrand({
      status: "needs_attention",
      updated_at: "2026-10-26T10:00:08Z",
    });
    store.getState().setTaskErrands(ERRAND_TASK_ID, [legacySnapshot], 0);
    expect(taskErrands(store)["errand-1"]).not.toHaveProperty("attention_reason");
  });

  it("accepts a strictly newer listed row even after a live observation during the request", () => {
    const store = createAppStore();
    store.getState().upsertErrand(withDeliveryState(makeErrand(), "uncertain"));
    const listed = withDeliveryState(makeErrand({ updated_at: NEWER }), "received");
    store.getState().setTaskErrands(ERRAND_TASK_ID, [listed], 0);

    expect(taskErrands(store)["errand-1"]).toEqual(listed);
  });

  it("fences live delivery observations when a legacy store omits row revisions", () => {
    const store = createAppStore();
    store.setState((state) => ({
      errands: { ...state.errands, liveRevisionByErrandIdByTaskId: undefined },
    }));
    const live = withDeliveryState(makeErrand(), "received");
    store.getState().upsertErrand(live);
    store
      .getState()
      .setTaskErrands(ERRAND_TASK_ID, [withDeliveryState(makeErrand(), "uncertain")], 0);

    expect(taskErrands(store)["errand-1"]).toEqual(live);
  });

  it("ignores listed rows that belong to another task", () => {
    const store = createAppStore();
    store.getState().setTaskErrands(ERRAND_TASK_ID, [makeErrand({ task_id: "task-other" })], 0);

    expect(taskErrands(store)).toEqual({});
    expect(taskErrands(store, "task-other")).toEqual({});
  });
});

describe("removeTaskErrands", () => {
  it("discards old row ownership so a later list can update an equal-timestamp row", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand());
    store.getState().removeTaskErrands(ERRAND_TASK_ID);
    store
      .getState()
      .setTaskErrands(ERRAND_TASK_ID, [withDeliveryState(makeErrand(), "uncertain")], 0);
    const received = withDeliveryState(makeErrand(), "received");
    store.getState().setTaskErrands(ERRAND_TASK_ID, [received], 0);

    expect(taskErrands(store)["errand-1"]).toEqual(received);
  });

  it("drops the task's errands and load state, leaving other tasks", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand());
    store.getState().setTaskErrands("task-b", [makeErrand({ task_id: "task-b" })], 0);
    store.getState().removeTaskErrands(ERRAND_TASK_ID);

    const { errands } = store.getState();
    expect(errands.byTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.loadedByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.liveRevisionByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.removedSessionIdsByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(Object.keys(taskErrands(store, "task-b"))).toEqual(["errand-1"]);
  });
});

describe("errand list observations", () => {
  it("preserves reset epochs and invalidates transport freshness on every removal", () => {
    const store = createAppStore();
    store.getState().removeTaskErrands(ERRAND_TASK_ID);
    store.getState().removeTaskErrands(ERRAND_TASK_ID);
    expect(store.getState().errands.resetEpochByTaskId[ERRAND_TASK_ID]).toBe(2);
    store.getState().setTaskErrands(ERRAND_TASK_ID, [makeErrand()], 0);
    expect(
      store.getState().errands.loadedConnectionGenerationByTaskId[ERRAND_TASK_ID],
    ).toBeUndefined();
    expect(store.getState().errands.loadedReadScopeByTaskId[ERRAND_TASK_ID]).toBeUndefined();
  });
});
