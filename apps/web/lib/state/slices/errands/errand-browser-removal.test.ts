import { describe, expect, it } from "vitest";
import { createAppStore } from "../../store";
import { stateReadScopeIdentity } from "../../shared-resource-reads";
import { registerErrandsHandlers } from "@/lib/ws/handlers/errands";
import { selectErrandBrowserHistoryRows, selectErrandBrowserRows } from "./selectors";
import {
  ERRAND_PARENT_ID,
  makeErrand,
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeHistoryPage,
} from "./errands.test-helpers";

const TASK_ID = "task-1";
const ERRAND_ID = "errand-1";
const ERRAND_SIBLING_ID = "errand-sibling";

describe("removed Errand browser fencing", () => {
  // @covers AC-TASKS-ERRAND-HISTORY-001.6
  it.each(["summary", "history"] as const)(
    "does not let a pending bounded %s response restore a removed Errand",
    (kind) => {
      const store = createAppStore();
      store.getState().setConnectionStatus("connected");
      const rows = [ERRAND_ID, ERRAND_SIBLING_ID].map((id) =>
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
      store.getState().removeErrandBrowserSession(TASK_ID, ERRAND_ID);
      expect(retainedRows()).toEqual([rows[1]]);
      completePendingRead();
      expect(retainedRows()).toEqual([rows[1]]);
      // A fresh attempt still filters a deleted identity returned by the server.
      read("fresh")();
      expect(retainedRows()).toEqual([rows[1]]);
    },
  );

  it("ignores a delayed WebSocket errand update after the Errand was removed", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    const update = (errandId: string) =>
      handle({
        type: "notification",
        action: "errand.updated",
        payload: makeErrand({ errand_id: errandId, task_id: TASK_ID }),
      } as never);
    update(ERRAND_ID);
    update(ERRAND_SIBLING_ID);
    expect(store.getState().errands.byTaskId[TASK_ID][ERRAND_ID]).toBeDefined();
    store.getState().removeErrandBrowserSession(TASK_ID, ERRAND_ID);
    update(ERRAND_ID);

    expect(Object.keys(store.getState().errands.byTaskId[TASK_ID])).toEqual([ERRAND_SIBLING_ID]);
  });
});
