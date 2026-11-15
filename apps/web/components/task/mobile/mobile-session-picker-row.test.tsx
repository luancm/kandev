import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState, type ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { sessionId, taskId } from "@/lib/types/http";
import {
  makeBrowserErrandRow,
  makeBrowserSummary,
} from "@/lib/state/slices/errands/errands.test-helpers";
import { MobileSessionPickerRow } from "./mobile-session-picker-row";

const TASK_ID = "task-1";
const PARENT_ID = "session-parent";
const ERRAND_ID = "session-errand";
const PILL = "mobile-sessions-pill";

type Fixture = { enabled: boolean; errandRole: boolean; row: boolean };
let fixture: Fixture;

vi.mock("./mobile-sessions-section", () => ({
  MobileSessionsPicker: ({ sessionId }: { sessionId: string | null }) => (
    <button type="button" data-testid={PILL} data-session-id={sessionId} />
  ),
}));

function setState({ enabled = true, errandRole = true, row = true }: Partial<Fixture> = {}) {
  fixture = { enabled, errandRole, row };
}

function SeedStore({ children }: { children: ReactNode }) {
  const store = useAppStoreApi();
  useState(() => {
    store.getState().retainErrandDetail("picker-fixture", ERRAND_ID);
    const common = {
      task_id: taskId(TASK_ID),
      state: "RUNNING" as const,
      started_at: "2026-10-26T10:00:00Z",
      updated_at: "2026-10-26T10:00:00Z",
    };
    store.getState().setTaskSession({ ...common, id: sessionId(PARENT_ID) });
    store.getState().setTaskSession({
      ...common,
      id: sessionId(ERRAND_ID),
      role: fixture.errandRole ? "errand" : undefined,
      parent_session_id: sessionId(PARENT_ID),
    });
    if (fixture.row) {
      store.getState().setConnectionStatus("connected");
      const state = store.getState();
      const observation = {
        connectionGeneration: state.connection.generation ?? 0,
        readScopeIdentity: stateReadScopeIdentity(state),
        taskResetEpoch: state.errands.resetEpochByTaskId[TASK_ID] ?? 0,
        observationRevision: state.errands.browserRevisionByTaskId[TASK_ID] ?? 0,
      };
      const summary = makeBrowserSummary([
        makeBrowserErrandRow({
          errand_id: ERRAND_ID,
          status: "running",
          live: true,
          outstanding: true,
          settled_at: null,
        }),
      ]);
      state.beginErrandBrowserSummaryRead(TASK_ID, observation, "picker-fixture");
      state.setErrandBrowserSummary(TASK_ID, summary, observation, "picker-fixture");
      store.getState().setConnectionStatus("disconnected");
    }
  });
  return children;
}

function renderRow(selectedSessionId: string | null) {
  return render(
    <StateProvider
      initialState={{ features: { ...defaultState.features, errands: fixture.enabled } }}
    >
      <SeedStore>
        <MobileSessionPickerRow taskId={TASK_ID} sessionId={selectedSessionId} />
      </SeedStore>
    </StateProvider>,
  );
}

afterEach(cleanup);
beforeEach(() => setState());

describe("MobileSessionPickerRow", () => {
  it.each([true, false])(
    "shows the sessions pill for an ordinary session with errands=%s",
    (enabled) => {
      setState({ enabled });
      renderRow(PARENT_ID);
      expect(screen.getByTestId("mobile-session-picker-row")).toBeTruthy();
      expect(screen.getByTestId(PILL).getAttribute("data-session-id")).toBe(PARENT_ID);
    },
  );

  it("shows the pill when there is no session yet", () => {
    renderRow(null);
    expect(screen.getByTestId(PILL)).toBeTruthy();
  });

  it.each([true, false])("hides the row for a persisted errand with errands=%s", (enabled) => {
    setState({ enabled });
    renderRow(ERRAND_ID);
    expect(screen.queryByTestId("mobile-session-picker-row")).toBeNull();
    expect(screen.queryByTestId(PILL)).toBeNull();
  });

  it("hides the row for an errand known only by its errand row", () => {
    setState({ errandRole: false });
    renderRow(ERRAND_ID);
    expect(screen.queryByTestId(PILL)).toBeNull();
  });

  it("hides the row for a retained errand without a cached row while the flag is off", () => {
    setState({ enabled: false, row: false });
    renderRow(ERRAND_ID);
    expect(screen.queryByTestId("mobile-session-picker-row")).toBeNull();
    expect(screen.queryByTestId(PILL)).toBeNull();
  });
});
