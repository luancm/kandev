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
      resetEpochByTaskId: {},
      removedSessionIdsByTaskId: {},
      browserSummaryByTaskId: {},
      browserSummaryReadByTaskId: {},
      browserRevisionByTaskId: {},
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
describe("upsertErrand operational retention", () => {
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

  it("applies newer and equal updates to retained inspection and evicts terminal operational rows", () => {
    const store = createAppStore();
    const running = makeErrand({ status: "running", updated_at: OLDER });
    store.getState().upsertErrand(running);
    store.getState().retainErrandInspection(
      "child",
      makeInspection(
        makeBrowserErrandRow({
          status: "running",
          result_state: "none",
          live: true,
          outstanding: true,
          updated_at: OLDER,
        }),
      ),
    );
    expect(taskErrands(store)["errand-1"]).toEqual(running);

    store.getState().upsertErrand(makeErrand({ status: "done", updated_at: NEWER }));
    expect(store.getState().errands.detailBySessionId["errand-1"].errand).toMatchObject({
      status: "done",
      updated_at: NEWER,
      live: false,
    });
    expect(taskErrands(store)["errand-1"]).toBeUndefined();

    store.getState().upsertErrand(makeErrand({ status: "failed", updated_at: NEWER }));
    expect(store.getState().errands.detailBySessionId["errand-1"].errand).toMatchObject({
      status: "failed",
      updated_at: NEWER,
      live: false,
    });
    expect(taskErrands(store)["errand-1"]).toBeUndefined();
  });

  it("ignores an update older than the retained outstanding errand", () => {
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    const observation = browserObservation(store);
    const pending = makeBrowserErrandRow({
      status: "done",
      result_state: "pending",
      outstanding: true,
      delivery_state: "uncertain",
      updated_at: NEWER,
    });
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary");
    store.getState().setErrandBrowserSummary(
      ERRAND_TASK_ID,
      makeBrowserSummary([pending], {
        live: 0,
        outstanding: 1,
        pending_results: 1,
      }),
      observation,
      "summary",
    );
    store.getState().upsertErrand(makeErrand({ status: "running", updated_at: OLDER }));

    expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID].errands).toEqual([
      pending,
    ]);
    expect(taskErrands(store)["errand-1"]).toBeUndefined();
  });
});

describe("upsertErrand observation ordering", () => {
  it("applies equal-timestamp receipt updates and clears omitted state", () => {
    const store = createAppStore();
    store.getState().upsertErrand(withDeliveryState(makeErrand(), "uncertain"));
    store.getState().upsertErrand(withDeliveryState(makeErrand(), "received"));

    expect(taskErrands(store)["errand-1"]).toHaveProperty("delivery_state", "received");
    expect(taskErrands(store)["errand-1"].updated_at).toBe(OLDER);

    store.getState().upsertErrand(makeErrand());
    expect(taskErrands(store)["errand-1"]).not.toHaveProperty("delivery_state");
  });

  it("does not let an older summary roll back recovery attention from a newer event", () => {
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    const observation = browserObservation(store);
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary");
    const recovery = makeErrand({
      status: "needs_attention",
      updated_at: NEWER,
      attention_reason: "recovery_unresolved",
    });
    store.getState().upsertErrand(recovery);
    store.getState().setErrandBrowserSummary(
      ERRAND_TASK_ID,
      makeBrowserSummary([
        makeBrowserErrandRow({
          status: "needs_attention",
          live: true,
          outstanding: true,
          updated_at: OLDER,
          attention_reason: "permission",
        }),
      ]),
      observation,
      "summary",
    );

    expect(taskErrands(store)["errand-1"].attention_reason).toBe("recovery_unresolved");
    expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID]).toBeUndefined();
  });

  it.each([
    [NEWER, "2026-02-30"],
    [NEWER, "not-a-timestamp"],
    ["2026-02-30", OLDER],
  ])("accepts an update when timestamps %s and %s cannot be compared", (existingAt, incomingAt) => {
    const store = createAppStore();
    store.getState().retainErrandInspection(
      "child",
      makeInspection(
        makeBrowserErrandRow({
          status: "done",
          updated_at: existingAt,
        }),
      ),
    );
    store.getState().upsertErrand(makeErrand({ status: "cancelled", updated_at: incomingAt }));

    expect(store.getState().errands.detailBySessionId["errand-1"].errand).toMatchObject({
      status: "cancelled",
      updated_at: incomingAt,
    });
    expect(taskErrands(store)["errand-1"]).toBeUndefined();
  });

  it("keeps tasks independent", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand({ task_id: "task-a" }));
    store.getState().upsertErrand(makeErrand({ task_id: "task-b", errand_id: "errand-2" }));

    expect(Object.keys(taskErrands(store, "task-a"))).toEqual(["errand-1"]);
    expect(Object.keys(taskErrands(store, "task-b"))).toEqual(["errand-2"]);
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

describe("removeTaskErrands", () => {
  // @covers AC-TASKS-ERRAND-HISTORY-001.6
  it("rejects bounded reads from a previous reset epoch and admits a fresh summary", () => {
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    store.getState().upsertErrand(makeErrand());
    const oldObservation = browserObservation(store);
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, oldObservation, "old-summary");
    store.getState().openErrandHistory("panel", ERRAND_TASK_ID, ERRAND_PARENT_ID);
    store.getState().beginErrandHistoryRead("panel", oldObservation, null, "old-page");
    store.getState().retainErrandInspection("child", makeInspection());

    store.getState().removeTaskErrands(ERRAND_TASK_ID);
    store.getState().removeTaskErrands(ERRAND_TASK_ID);
    expect(store.getState().errands.resetEpochByTaskId[ERRAND_TASK_ID]).toBe(2);
    expect(store.getState().errands.detailBySessionId["errand-1"]).toBeUndefined();
    expect(store.getState().errands.detailOwnersBySessionId["errand-1"]).toBeUndefined();
    store.getState().openErrandHistory("panel", ERRAND_TASK_ID, ERRAND_PARENT_ID);
    store.getState().beginErrandHistoryRead("panel", oldObservation, null, "old-page");
    store
      .getState()
      .setErrandHistoryPage(
        "panel",
        makeHistoryPage([makeBrowserErrandRow()]),
        oldObservation,
        null,
        "old-page",
      );
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, oldObservation, "old-summary");
    store
      .getState()
      .setErrandBrowserSummary(
        ERRAND_TASK_ID,
        makeBrowserSummary([makeBrowserErrandRow()]),
        oldObservation,
        "old-summary",
      );
    expect(store.getState().errands.historyByOwnerId.panel.pages).toEqual([]);
    expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(store.getState().errands.browserSummaryReadByTaskId[ERRAND_TASK_ID]).toBeUndefined();

    const current = browserObservation(store);
    const received = makeBrowserErrandRow({ delivery_state: "received", outstanding: true });
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, current, "fresh-summary");
    store
      .getState()
      .setErrandBrowserSummary(
        ERRAND_TASK_ID,
        makeBrowserSummary([received]),
        current,
        "fresh-summary",
      );
    expect(store.getState().errands.browserSummaryByTaskId[ERRAND_TASK_ID].errands).toEqual([
      received,
    ]);
  });

  it("drops the task's errands and browser state, leaving other tasks", () => {
    const store = createAppStore();
    store.getState().upsertErrand(makeErrand());
    store.getState().upsertErrand(makeErrand({ task_id: "task-b" }));
    store.getState().removeTaskErrands(ERRAND_TASK_ID);

    const { errands } = store.getState();
    expect(errands.byTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.browserRevisionByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(errands.removedSessionIdsByTaskId[ERRAND_TASK_ID]).toBeUndefined();
    expect(Object.keys(taskErrands(store, "task-b"))).toEqual(["errand-1"]);
  });
});
