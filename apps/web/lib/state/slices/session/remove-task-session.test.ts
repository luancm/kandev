import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import { createErrandsSlice } from "../errands/errands-slice";
import {
  ERRAND_PARENT_ID,
  makeErrand,
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeHistoryPage,
} from "../errands/errands.test-helpers";
import { createAppStore } from "../../store";
import { stateReadScopeIdentity } from "../../shared-resource-reads";
import { selectErrandBrowserHistoryRows, selectErrandBrowserRows } from "../errands/selectors";
import { registerErrandsHandlers } from "@/lib/ws/handlers/errands";
import type { SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import type { ErrandsSlice } from "../errands/types";

type CombinedSlice = SessionSlice & SessionRuntimeSlice & ErrandsSlice;

/** Creates a zustand store combining the session, session-runtime, and errands slices. */
function makeStore() {
  return create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createErrandsSlice as any)(set),
    })),
  );
}

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const ERRAND_SIBLING_ID = "errand-sibling";
const INCARNATION_ID = `incarnation-1`;

describe("removeTaskSession cleanup cascade", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("purges messages, turns, and runtime buffers for the removed session", () => {
    const s = store.getState();
    s.registerSessionEnvironment(SESSION_ID, "env-1");
    s.appendShellOutput(SESSION_ID, "shell output");
    s.setContextWindow(SESSION_ID, {
      size: 1,
      used: 1,
      remaining: 0,
      efficiency: 1,
      compactionCount: 0,
    });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    s.addMessage({ id: "m1", session_id: SESSION_ID, role: "user", content: "hi" } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    s.addTurn({ id: "t1", session_id: SESSION_ID } as any);
    s.markTurnsLoaded(SESSION_ID);
    // Seed the other two per-session reconciliation maps so removal must
    // clear all three: a leaked settled boundary would block every future
    // turn of a recreated session with the same id.
    s.reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:00:00Z");

    expect(store.getState().messages.bySession[SESSION_ID]).toHaveLength(1);
    expect(store.getState().turns.bySession[SESSION_ID]).toHaveLength(1);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBe(true);
    expect(store.getState().turns.settledBoundaryBySession[SESSION_ID]).toBe(
      "2026-07-23T10:00:00Z",
    );
    expect(store.getState().turns.reconcileEpochBySession[SESSION_ID]).toBe(1);
    s.installAuthoritativePromptMessages(SESSION_ID, [], {
      hasMore: false,
      oldestCursor: null,
    });
    s.removeMessage(SESSION_ID, "removed-before-response");
    expect(store.getState().messagePrompts.authoritativeBySession[SESSION_ID]).toBe(true);

    store.getState().removeTaskSession(TASK_ID, SESSION_ID);

    const after = store.getState();
    expect(after.messages.bySession[SESSION_ID]).toBeUndefined();
    expect(after.messagePrompts.bySession[SESSION_ID]).toBeUndefined();
    expect(after.messagePrompts.generationBySession[SESSION_ID]).toBe(1);
    expect(after.messagePrompts.authoritativeBySession[SESSION_ID]).toBeUndefined();
    expect(after.messagePrompts.observedBySession[SESSION_ID]).toBeUndefined();
    expect(after.messagePrompts.deletedIdsBySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.bySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.loadedBySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.settledBoundaryBySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.reconcileEpochBySession[SESSION_ID]).toBeUndefined();
    expect(after.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(after.shell.outputs["env-1"]).toBeUndefined();
    expect(after.environmentIdBySessionId[SESSION_ID]).toBeUndefined();
  });

  it("removes the errand row whose ID is the removed session", () => {
    store.getState().upsertErrand(makeErrand({ errand_id: SESSION_ID, task_id: TASK_ID }));
    store.getState().upsertErrand(makeErrand({ errand_id: ERRAND_SIBLING_ID, task_id: TASK_ID }));

    store.getState().removeTaskSession(TASK_ID, SESSION_ID);

    const errands = store.getState().errands.byTaskId[TASK_ID];
    expect(errands?.[SESSION_ID]).toBeUndefined();
    expect(errands?.[ERRAND_SIBLING_ID]).toBeDefined();
  });

  it("removes an ordinary session without touching the task's errands", () => {
    store.getState().upsertErrand(makeErrand({ errand_id: ERRAND_SIBLING_ID, task_id: TASK_ID }));

    store.getState().removeTaskSession(TASK_ID, "session-without-errand");

    expect(Object.keys(store.getState().errands.byTaskId[TASK_ID] ?? {})).toEqual([
      ERRAND_SIBLING_ID,
    ]);
  });

  it("clears queue metadata and protects a replacement operation token", () => {
    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = {
        id: SESSION_ID,
        task_id: TASK_ID,
        queue_incarnation_id: INCARNATION_ID,
      } as never;
    });
    expect(store.getState().beginQueueOperation(SESSION_ID, "stale-incarnation")).toBeNull();
    const first = store.getState().beginQueueOperation(SESSION_ID, INCARNATION_ID);
    expect(first).not.toBeNull();
    expect(store.getState().beginQueueOperation(SESSION_ID, INCARNATION_ID)).toBeNull();
    store.getState().setQueueEntries(SESSION_ID, [], {
      count: 0,
      max: 10,
      autoRun: true,
      mergeEnabled: true,
      sessionIncarnationId: INCARNATION_ID,
    });

    store.getState().removeTaskSession(TASK_ID, SESSION_ID);
    expect(store.getState().queue.metaBySessionId[SESSION_ID]).toBeUndefined();
    expect(store.getState().queue.activeOperationBySessionId[SESSION_ID]).toBeUndefined();

    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = {
        id: SESSION_ID,
        task_id: TASK_ID,
        queue_incarnation_id: "incarnation-2",
      } as never;
    });
    const replacement = store.getState().beginQueueOperation(SESSION_ID, "incarnation-2");
    expect(replacement).not.toBeNull();
    store.getState().finishQueueOperation(SESSION_ID, first!);
    expect(store.getState().queue.activeOperationBySessionId[SESSION_ID]).toEqual(replacement);
  });
});

describe("removed session errand fencing", () => {
  // @covers AC-TASKS-ERRAND-HISTORY-001.6
  it.each(["summary", "history"] as const)(
    "does not let a pending bounded %s response restore a removed session",
    (kind) => {
      const store = createAppStore();
      store.getState().setConnectionStatus("connected");
      const rows = [SESSION_ID, ERRAND_SIBLING_ID].map((id) =>
        makeBrowserErrandRow({
          errand_id: id,
          task_id: TASK_ID,
          ...(kind === "summary"
            ? {
                status: "running" as const,
                result_state: "none" as const,
                live: true,
                outstanding: true,
              }
            : {}),
        }),
      );
      const observation = () => ({
        connectionGeneration: store.getState().connection.generation ?? 0,
        readScopeIdentity: stateReadScopeIdentity(store.getState()),
        taskResetEpoch: store.getState().errands.resetEpochByTaskId[TASK_ID] ?? 0,
        observationRevision: store.getState().errands.browserRevisionByTaskId[TASK_ID] ?? 0,
      });
      store.getState().openErrandHistory("panel", TASK_ID, ERRAND_PARENT_ID);
      const read = (attemptId: string, responseRows = rows) => {
        const captured = observation();
        if (kind === "summary") {
          store.getState().beginErrandBrowserSummaryRead(TASK_ID, captured, attemptId);
          return () =>
            store
              .getState()
              .setErrandBrowserSummary(
                TASK_ID,
                makeBrowserSummary(responseRows, { live: 2, outstanding: 2 }),
                captured,
                attemptId,
              );
        }
        store.getState().beginErrandHistoryRead("panel", captured, null, attemptId);
        return () =>
          store
            .getState()
            .setErrandHistoryPage(
              "panel",
              makeHistoryPage(responseRows),
              captured,
              null,
              attemptId,
            );
      };
      const retainedRows = () =>
        kind === "summary"
          ? selectErrandBrowserRows(store.getState(), TASK_ID)
          : selectErrandBrowserHistoryRows(store.getState(), "panel");

      read("initial")();
      expect(retainedRows()).toEqual(rows);
      const completePendingRead = read("pending", [
        rows[0],
        { ...rows[1], title: "Stale sibling title" },
      ]);
      store.getState().removeTaskSession(TASK_ID, SESSION_ID);
      expect(retainedRows()).toEqual([rows[1]]);
      completePendingRead();
      expect(retainedRows()).toEqual([rows[1]]);
      // A fresh attempt still filters a deleted identity returned by the server.
      read("fresh")();
      expect(retainedRows()).toEqual([rows[1]]);
    },
  );

  it("ignores a delayed WebSocket errand update after the session was removed", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    const update = (errandId: string) =>
      handle({
        type: "notification",
        action: "errand.updated",
        payload: makeErrand({ errand_id: errandId, task_id: TASK_ID }),
      } as never);
    update(SESSION_ID);
    update(ERRAND_SIBLING_ID);
    expect(store.getState().errands.byTaskId[TASK_ID][SESSION_ID]).toBeDefined();
    store.getState().removeTaskSession(TASK_ID, SESSION_ID);
    update(SESSION_ID);

    expect(Object.keys(store.getState().errands.byTaskId[TASK_ID])).toEqual([ERRAND_SIBLING_ID]);
  });
});
