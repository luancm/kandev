import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { Errand } from "@/lib/types/errands";
import {
  ERRAND_PARENT_ID,
  ERRAND_TASK_ID,
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeErrand,
  makeHistoryPage,
  makeInspection,
} from "./errands.test-helpers";

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
      browserSummaryByTaskId: {},
      browserSummaryReadByTaskId: {},
      browserRevisionByTaskId: {},
      unreadSignatureByTaskId: {},
      historyByOwnerId: {},
      detailBySessionId: {},
      detailOwnersBySessionId: {},
    });
  });
});

function browserObservation(store: ReturnType<typeof createAppStore>, revision?: number) {
  const state = store.getState();
  return {
    connectionGeneration: state.connection.generation ?? 0,
    readScopeIdentity: stateReadScopeIdentity(state),
    taskResetEpoch: state.errands.resetEpochByTaskId[ERRAND_TASK_ID] ?? 0,
    observationRevision: revision ?? state.errands.browserRevisionByTaskId[ERRAND_TASK_ID] ?? 0,
  };
}

it.each(["loading", "loaded", "error"] as const)(
  "publishes a shared summary attempt's %s transition only once",
  (status) => {
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    const observation = browserObservation(store);
    const attempt = "shared-summary";
    const summary = makeBrowserSummary();
    const error = new Error("summary unavailable");
    const publish = () => {
      if (status === "loading") {
        store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, attempt);
      } else if (status === "loaded") {
        store.getState().setErrandBrowserSummary(ERRAND_TASK_ID, summary, observation, attempt);
      } else {
        store.getState().failErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, attempt, error);
      }
    };
    if (status !== "loading") {
      store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, attempt);
    }
    let notifications = 0;
    const unsubscribe = store.subscribe(() => notifications++);
    try {
      publish();
      publish();
      expect(store.getState().errands.browserSummaryReadByTaskId[ERRAND_TASK_ID].status).toBe(
        status,
      );
      expect(notifications).toBe(1);
    } finally {
      unsubscribe();
    }
  },
);

it("keeps authoritative counts independent of loaded outstanding rows", () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const observation = browserObservation(store);
  store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary-1");
  store.getState().setErrandBrowserSummary(
    ERRAND_TASK_ID,
    makeBrowserSummary([makeBrowserErrandRow({ outstanding: true, live: true })], {
      live: 3,
      capacity: 4,
      queued: 2,
      outstanding: 17,
      finished: 900,
      pending_results: 4,
      unread: 8,
    }),
    observation,
    "summary-1",
  );

  expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID].errands).toHaveLength(1);
  expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID].counts).toMatchObject({
    live: 3,
    capacity: 4,
    outstanding: 17,
    finished: 900,
    pending_results: 4,
    unread: 8,
  });
});

it("rejects a browser summary after a newer session-only observation", () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const observation = browserObservation(store);
  store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary-old");
  store.getState().advanceErrandBrowserRevision(ERRAND_TASK_ID, "errand-1");
  store
    .getState()
    .setErrandBrowserSummary(
      ERRAND_TASK_ID,
      makeBrowserSummary([makeBrowserErrandRow()]),
      observation,
      "summary-old",
    );

  expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID]).toBeUndefined();
  expect(store.getState().errands.browserRevisionByTaskId[ERRAND_TASK_ID]).toBe(1);
});

it("retains at most four history pages and advances the visible range when evicting", () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const ownerId = "parent-panel";
  store.getState().openErrandHistory(ownerId, ERRAND_TASK_ID, ERRAND_PARENT_ID);
  const observation = browserObservation(store);

  for (let pageIndex = 0; pageIndex < 5; pageIndex += 1) {
    const cursor = pageIndex === 0 ? null : `cursor-${pageIndex}`;
    const attemptId = `page-${pageIndex}`;
    const rows = Array.from({ length: 25 }, (_, rowIndex) =>
      makeBrowserErrandRow({ errand_id: `errand-${pageIndex * 25 + rowIndex}` }),
    );
    store.getState().beginErrandHistoryRead(ownerId, observation, cursor, attemptId);
    store.getState().setErrandHistoryPage(
      ownerId,
      makeHistoryPage(rows, {
        total: 125,
        hasMore: true,
        cursor: `cursor-${pageIndex + 1}`,
      }),
      observation,
      cursor,
      attemptId,
    );
  }

  const chain = store.getState().errands.historyByOwnerId[ownerId];
  expect(chain.pages).toHaveLength(4);
  expect(chain.pages.flatMap((page) => page.errands)).toHaveLength(100);
  expect(chain.rangeStart).toBe(25);
  expect(chain.pages[0].errands[0].errand_id).toBe("errand-25");

  store.getState().restartErrandHistory(ownerId);
  expect(store.getState().errands.historyByOwnerId[ownerId]).toMatchObject({
    pages: [],
    rangeStart: 0,
    nextCursor: null,
    status: "idle",
  });
});

it("does not let a page response resurrect a deleted errand", () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const ownerId = "parent-panel";
  store.getState().openErrandHistory(ownerId, ERRAND_TASK_ID, ERRAND_PARENT_ID);
  const oldObservation = browserObservation(store);
  store.getState().beginErrandHistoryRead(ownerId, oldObservation, null, "old-page");
  store.getState().removeErrandBrowserSession(ERRAND_TASK_ID, "errand-1");
  store
    .getState()
    .setErrandHistoryPage(
      ownerId,
      makeHistoryPage([makeBrowserErrandRow()]),
      oldObservation,
      null,
      "old-page",
    );
  expect(store.getState().errands.historyByOwnerId[ownerId].pages).toEqual([]);

  const currentObservation = browserObservation(store);
  store.getState().beginErrandHistoryRead(ownerId, currentObservation, null, "new-page");
  store
    .getState()
    .setErrandHistoryPage(
      ownerId,
      makeHistoryPage([makeBrowserErrandRow()]),
      currentObservation,
      null,
      "new-page",
    );
  expect(store.getState().errands.historyByOwnerId[ownerId].pages[0].errands).toEqual([]);
});

it("keeps terminal pending results outstanding until an authoritative receipt update", () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const observation = browserObservation(store);
  const initial = makeBrowserSummary(
    [
      makeBrowserErrandRow({
        status: "running",
        result_state: "none",
        live: true,
        outstanding: true,
      }),
    ],
    { live: 1, outstanding: 1 },
  );
  store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary-1");
  store.getState().setErrandBrowserSummary(ERRAND_TASK_ID, initial, observation, "summary-1");
  store.getState().openErrandHistory("parent-panel", ERRAND_TASK_ID, ERRAND_PARENT_ID);

  store
    .getState()
    .upsertErrand(makeErrand({ status: "done", result_state: "pending", can_send_result: false }));
  expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID].errands[0]).toMatchObject({
    status: "done",
    result_state: "pending",
    live: false,
    outstanding: true,
  });
  expect(store.getState().errands.historyByOwnerId["parent-panel"].stale).toBe(true);

  store
    .getState()
    .upsertErrand(
      makeErrand({ status: "done", result_state: "delivered", can_send_result: false }),
    );
  expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID].errands).toEqual([]);
});

it("releases only child detail that no visible owner retains", () => {
  const store = createAppStore();
  const inspection = makeInspection();
  store.getState().retainErrandInspection("task-chat", inspection);
  store.getState().retainErrandInspection("preview", inspection);
  store.getState().releaseErrandInspection("task-chat");
  expect(store.getState().errands.detailBySessionId["errand-1"]).toEqual(inspection);
  store.getState().releaseErrandInspection("preview");
  expect(store.getState().errands.detailBySessionId["errand-1"]).toBeUndefined();
});
describe("upsertErrand", () => {
  it("stores the errand under its task and errand ID", () => {
    const store = createAppStore();
    const errand = makeErrand();
    store.getState().upsertErrand(errand);

    expect(taskErrands(store)).toEqual({ "errand-1": errand });
  });

  it("does not retain an unseen terminal WebSocket event as full Errand history", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ status: "done" }));

    expect(taskErrands(store)).toEqual({});
  });

  it("replaces an explicitly loaded compatibility row with a newer or equal update", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskErrands(ERRAND_TASK_ID, [makeErrand({ status: "running", updated_at: OLDER })], 0);
    store.getState().upsertErrand(makeErrand({ status: "done", updated_at: NEWER }));
    expect(taskErrands(store)["errand-1"].status).toBe("done");

    store.getState().upsertErrand(makeErrand({ status: "failed", updated_at: NEWER }));
    expect(taskErrands(store)["errand-1"].status).toBe("failed");
  });

  it("ignores an update older than the stored errand", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskErrands(
        ERRAND_TASK_ID,
        [withDeliveryState(makeErrand({ status: "done", updated_at: NEWER }), "uncertain")],
        0,
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
    store
      .getState()
      .setTaskErrands(ERRAND_TASK_ID, [makeErrand({ status: "done", updated_at: NEWER })], 0);
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
      .setTaskErrands(
        ERRAND_TASK_ID,
        [withDeliveryState(makeErrand({ status: "done", updated_at: NEWER }), "uncertain")],
        revision,
      );
    store
      .getState()
      .upsertErrand(makeErrand({ status: "failed", updated_at: "2026-10-26T10:00:10Z" }));
    store
      .getState()
      .setTaskErrands(
        ERRAND_TASK_ID,
        [makeErrand({ status: "running", updated_at: OLDER })],
        revision,
      );

    expect(taskErrands(store)["errand-1"].status).toBe("failed");
    expect(taskErrands(store)["errand-1"]).not.toHaveProperty("delivery_state");
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
