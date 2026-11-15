import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import { createAppStore } from "@/lib/state/store";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { errandBrowserReadCoordinator } from "./errand-browser-read-coordinator";
import {
  selectErrandBrowserHistoryRows,
  selectErrandBrowserRowBySessionId,
  selectErrandBrowserSummary,
  selectErrandInspection,
} from "./selectors";
import {
  ERRAND_TASK_ID as taskId,
  ERRAND_PARENT_ID as parentId,
  makeErrand,
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeHistoryPage,
  makeInspection,
} from "./errands.test-helpers";

function loadedBrowser() {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const observation = {
    connectionGeneration: store.getState().connection.generation ?? 0,
    readScopeIdentity: stateReadScopeIdentity(store.getState()),
    taskResetEpoch: 0,
    observationRevision: 0,
  };
  const row = makeBrowserErrandRow();
  const state = store.getState();
  state.beginErrandBrowserSummaryRead(taskId, observation, "summary");
  state.setErrandBrowserSummary(taskId, makeBrowserSummary([row]), observation, "summary");
  state.openErrandHistory("panel", taskId, parentId);
  state.beginErrandHistoryRead("panel", observation, null, "page");
  state.setErrandHistoryPage("panel", makeHistoryPage([row]), observation, null, "page");
  state.retainErrandInspection("child", makeInspection(row));
  return { store, observation, row };
}

it("invalidates loaded history for a child state event without inserting a normal session", () => {
  const { store, row } = loadedBrowser();
  store.getState().releaseErrandInspection("child");
  store.getState().upsertTaskSessionFromEvent(taskId, {
    id: toSessionId(row.errand_id),
    task_id: toTaskId(taskId),
    state: "COMPLETED",
    started_at: row.created_at,
    updated_at: "2026-11-14T00:00:00Z",
    metadata: { context_window: { size: 100000, used: 68000 } },
  });
  expect(store.getState().errands.historyByOwnerId.panel.stale).toBe(true);
  expect(store.getState().taskSessionsByTask.itemsByTaskId[taskId] ?? []).toEqual([]);
  expect(store.getState().taskSessions.items[row.errand_id]).toBeUndefined();
});

it("ends loading when a completed history response loses its observation ownership", () => {
  const { store, observation, row } = loadedBrowser();
  store.getState().restartErrandHistory("panel");
  store.getState().beginErrandHistoryRead("panel", observation, null, "refresh");
  store.getState().advanceErrandBrowserRevision(taskId);
  store
    .getState()
    .setErrandHistoryPage("panel", makeHistoryPage([row]), observation, null, "refresh");
  expect(store.getState().errands.historyByOwnerId.panel).toMatchObject({
    status: "idle",
    stale: true,
    pages: [],
  });
});

// @covers AC-TASKS-ERRAND-HISTORY-001.3
it("does not accumulate full rows after repeated completion", () => {
  const store = createAppStore();
  for (let index = 0; index < 250; index++) {
    const errand = makeErrand({ errand_id: `child-${index}` });
    store.getState().upsertErrand(errand);
    store.getState().upsertErrand({ ...errand, status: "done", result_state: "delivered" });
    store.getState().upsertErrand(makeErrand({ errand_id: `unseen-${index}`, status: "done" }));
  }
  expect(Object.keys(store.getState().errands.byTaskId[taskId] ?? {})).toHaveLength(0);
});

// @covers AC-TASKS-ERRAND-HISTORY-001.6
describe.each(["workspace", "authentication"] as const)("%s scope transition", (kind) => {
  it("immediately evicts loaded data and aborts requests, even across A-B-A", async () => {
    const { store, observation, row } = loadedBrowser();
    let resolve!: (value: string) => void;
    let signal!: AbortSignal;
    const lease = errandBrowserReadCoordinator(store).acquire(
      { kind: "history", taskId, ...observation },
      0,
      (nextSignal) => {
        signal = nextSignal;
        return new Promise<string>((done) => {
          resolve = done;
        });
      },
    );
    // Observe rejection after driving the transition, so a RED assertion cannot leak it.
    const result = lease.promise.catch((error: unknown) => error);
    await Promise.resolve();
    const before = store.getState();
    const changed =
      kind === "workspace"
        ? { ...before, workspaceContextGeneration: before.workspaceContextGeneration + 1 }
        : { ...before, auth: { ...before.auth, authenticated: !before.auth.authenticated } };
    expect(selectErrandBrowserHistoryRows(changed, "panel")).toEqual([]);
    expect(selectErrandBrowserSummary(changed, taskId)).toBeUndefined();
    expect(selectErrandInspection(changed, row.errand_id)).toBeUndefined();
    store.setState(
      kind === "workspace"
        ? { workspaceContextGeneration: changed.workspaceContextGeneration }
        : { auth: changed.auth },
    );
    expect(store.getState().errands.historyByOwnerId).toEqual({});
    expect(store.getState().errands.detailBySessionId).toEqual({});
    expect(store.getState().errands.browserSummaryByTaskId).toEqual({});
    expect(signal.aborted).toBe(true);
    store.setState(
      kind === "workspace"
        ? { workspaceContextGeneration: before.workspaceContextGeneration }
        : { auth: before.auth },
    );
    resolve("obsolete");
    expect(await result).toMatchObject({ name: "AbortError" });
    expect(selectErrandBrowserHistoryRows(store.getState(), "panel")).toEqual([]);
  });
});

// @covers AC-TASKS-ERRAND-HISTORY-001.6
it("updates retained inspection after completion and refuses older status", () => {
  const store = createAppStore();
  const inspection = makeInspection(
    makeBrowserErrandRow({
      status: "needs_attention",
      attention_reason: "recovery_unresolved",
      live: true,
      outstanding: true,
      can_send_result: false,
    }),
  );
  store.getState().retainErrandInspection("child", inspection);
  store.getState().upsertErrand(
    makeErrand({
      status: "done",
      result_state: "delivered",
      can_send_result: true,
      updated_at: "2026-10-26T11:00:00Z",
    }),
  );
  expect(selectErrandBrowserRowBySessionId(store.getState(), taskId, "errand-1")).toMatchObject({
    status: "done",
    live: false,
    can_send_result: true,
  });
  expect(
    selectErrandInspection(store.getState(), "errand-1")?.errand.attention_reason,
  ).toBeUndefined();
  store.getState().upsertErrand(makeErrand({ status: "running" }));
  expect(selectErrandInspection(store.getState(), "errand-1")?.errand.status).toBe("done");
  expect(selectErrandInspection(store.getState(), "errand-1")?.parent).toEqual(inspection.parent);
  store.getState().releaseErrandInspection("child");
  expect(selectErrandInspection(store.getState(), "errand-1")).toBeUndefined();
});

it("preserves a retained finished history row when an older running event arrives", () => {
  const { store, observation } = loadedBrowser();
  store.getState().releaseErrandInspection("child");
  store.getState().setErrandBrowserSummary(taskId, makeBrowserSummary([]), observation, "summary");
  store
    .getState()
    .upsertErrand(makeErrand({ status: "running", updated_at: "2026-10-25T10:00:00Z" }));
  expect(selectErrandBrowserHistoryRows(store.getState(), "panel")).toHaveLength(1);
  expect(selectErrandBrowserHistoryRows(store.getState(), "panel")[0].status).toBe("done");
  expect(store.getState().errands.byTaskId[taskId]?.["errand-1"]).toBeUndefined();
});

it("evicts operational rows after an authoritative summary repairs a missed terminal event", () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  store.getState().upsertErrand(makeErrand({ status: "running" }));
  const observation = {
    connectionGeneration: store.getState().connection.generation ?? 0,
    readScopeIdentity: stateReadScopeIdentity(store.getState()),
    taskResetEpoch: 0,
    observationRevision: store.getState().errands.browserRevisionByTaskId[taskId],
  };
  store.getState().beginErrandBrowserSummaryRead(taskId, observation, "repair");
  store.getState().setErrandBrowserSummary(taskId, makeBrowserSummary([]), observation, "repair");
  expect(Object.keys(store.getState().errands.byTaskId[taskId] ?? {})).toHaveLength(0);
});
