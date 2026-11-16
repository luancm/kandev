import { useState, type ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import {
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeInspection,
  makeHistoryPage,
} from "@/lib/state/slices/errands/errands.test-helpers";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import { sessionId, taskId } from "@/lib/types/http";
import { MobileSessionsPicker } from "../../mobile/mobile-sessions-section";
import { MobileSessionPickerRow } from "../../mobile/mobile-session-picker-row";
import { ErrandHeader } from "./errand-header";

const api = vi.hoisted(() => ({ inspectErrand: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => api);

type Store = ReturnType<typeof useAppStoreApi>;
const HEADER = "errand-header";
const PILL = "mobile-sessions-pill";
const TASK_ID = "task-1";
const SESSION_ID = "ordinary-1";
const ordinarySession = {
  id: sessionId(SESSION_ID),
  task_id: taskId(TASK_ID),
  name: "Ordinary conversation",
  state: "COMPLETED" as const,
  started_at: "2026-10-26T10:00:00Z",
  updated_at: "2026-10-26T10:00:00Z",
};

function subscriptionTracker() {
  const listeners = new Set<Parameters<Store["subscribe"]>[0]>();
  let callbacks = 0;
  let store: Store;
  return {
    attach(current: Store) {
      store = current;
      const subscribe = store.subscribe;
      store.subscribe = (listener) => {
        const observed: typeof listener = (...args) => {
          callbacks++;
          listener(...args);
        };
        listeners.add(observed);
        const unsubscribe = subscribe(observed);
        return () => {
          listeners.delete(observed);
          unsubscribe();
        };
      };
      store.getState().setTaskSession(ordinarySession);
      store
        .getState()
        .setTaskSessionsForTask(
          TASK_ID,
          [ordinarySession],
          captureTaskSessionHydrationEpochs(store.getState(), TASK_ID),
        );
    },
    get active() {
      return listeners.size;
    },
    get store() {
      return store;
    },
    notifyUnrelated() {
      const before = callbacks;
      act(() => {
        const userSettings = store.getState().userSettings;
        store.setState({ userSettings: { ...userSettings, loaded: !userSettings.loaded } });
      });
      return callbacks - before;
    },
  };
}

type Tracker = ReturnType<typeof subscriptionTracker>;
function TrackedStore({ tracker, children }: { tracker: Tracker; children: ReactNode }) {
  const store = useAppStoreApi();
  useState(() => tracker.attach(store));
  return children;
}

afterEach(() => {
  cleanup();
  api.inspectErrand.mockReset();
});

describe.each(["header", "phone-picker"] as const)("%s ordinary subscription budget", (surface) => {
  it.each([true, false])("adds at most two listeners with errands=%s", (enabled) => {
    const tracker = subscriptionTracker();
    const baseline =
      surface === "phone-picker" ? (
        <MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_ID} fullWidth />
      ) : null;
    const consumer =
      surface === "header" ? (
        <ErrandHeader taskId={TASK_ID} sessionId={SESSION_ID} />
      ) : (
        <MobileSessionPickerRow taskId={TASK_ID} sessionId={SESSION_ID} />
      );
    const tree = (children: ReactNode) => (
      <StateProvider initialState={{ features: { ...defaultState.features, errands: enabled } }}>
        <TrackedStore tracker={tracker}>{children}</TrackedStore>
      </StateProvider>
    );
    const view = render(tree(baseline));
    const baselineListeners = tracker.active;
    const baselineCallbacks = tracker.notifyUnrelated();
    view.rerender(tree(consumer));
    const addedListeners = tracker.active - baselineListeners;
    const addedCallbacks = tracker.notifyUnrelated() - baselineCallbacks;
    // @covers AC-TASKS-ERRAND-HISTORY-001.1: ordinary navigation has bounded Errand work.
    expect.soft(addedListeners).toBeLessThanOrEqual(2);
    expect.soft(addedCallbacks).toBeLessThanOrEqual(2);
    act(() => tracker.store.getState().setTaskSession({ ...ordinarySession, name: "Renamed" }));
    expect(screen.queryByTestId(HEADER)).toBeNull();
    if (surface === "phone-picker") {
      expect(screen.getByTestId(PILL).textContent).toContain("Renamed");
    }
    expect(tracker.store.getState().errands.detailOwnersBySessionId).toEqual({});
    expect(tracker.store.getState().errands.detailBySessionId).toEqual({});
    expect(api.inspectErrand).not.toHaveBeenCalled();
    view.rerender(tree(baseline));
    expect(tracker.active).toBe(baselineListeners);
    view.unmount();
    expect(tracker.active).toBe(0);
    expect(tracker.notifyUnrelated()).toBe(0);
    console.info("ordinary Errand subscription budget", {
      surface,
      enabled,
      baselineListeners,
      baselineCallbacks,
      addedListeners,
      addedCallbacks,
      afterUnmount: tracker.active,
    });
  });
});

it.each([true, false])(
  "preserves independent header and phone-picker owners through navigation with errands=%s",
  (enabled) => {
    const tracker = subscriptionTracker();
    const tree = (header: boolean, picker: boolean, selected: string) => (
      <StateProvider initialState={{ features: { ...defaultState.features, errands: enabled } }}>
        <TrackedStore tracker={tracker}>
          <ToastProvider>
            {header && <ErrandHeader taskId={TASK_ID} sessionId={selected} />}
            {picker && <MobileSessionPickerRow taskId={TASK_ID} sessionId={selected} />}
          </ToastProvider>
        </TrackedStore>
      </StateProvider>
    );
    const view = render(tree(true, true, SESSION_ID));
    expect(screen.getByTestId(PILL).textContent).toContain("Ordinary conversation");
    act(() => {
      tracker.store.getState().setTaskSession({
        ...ordinarySession,
        id: sessionId("child-1"),
        role: "errand",
        name: "Child",
        parent_session_id: sessionId(SESSION_ID),
      });
    });
    view.rerender(tree(true, true, "child-1"));
    expect(screen.getByTestId(HEADER)).toBeTruthy();
    expect(screen.queryByTestId(PILL)).toBeNull();
    expect(
      Object.keys(tracker.store.getState().errands.detailOwnersBySessionId["child-1"]),
    ).toHaveLength(2);
    view.rerender(tree(false, true, "child-1"));
    expect(
      Object.keys(tracker.store.getState().errands.detailOwnersBySessionId["child-1"]),
    ).toHaveLength(1);
    expect(tracker.store.getState().taskSessions.items["child-1"]).toBeDefined();
    view.rerender(tree(false, true, SESSION_ID));
    expect(screen.getByTestId(PILL).textContent).toContain("Ordinary conversation");
    expect(tracker.store.getState().errands.detailOwnersBySessionId["child-1"]).toBeUndefined();
    expect(tracker.store.getState().taskSessions.items["child-1"]).toBeUndefined();
    expect(api.inspectErrand).not.toHaveBeenCalled();
  },
);

it.each([true, false])(
  "updates both surfaces when an ordinary session gains an errand role with errands=%s",
  (enabled) => {
    const tracker = subscriptionTracker();
    const view = render(
      <StateProvider initialState={{ features: { ...defaultState.features, errands: enabled } }}>
        <TrackedStore tracker={tracker}>
          <ToastProvider>
            <ErrandHeader taskId={TASK_ID} sessionId={SESSION_ID} />
            <MobileSessionPickerRow taskId={TASK_ID} sessionId={SESSION_ID} />
          </ToastProvider>
        </TrackedStore>
      </StateProvider>,
    );
    expect(screen.queryByTestId(HEADER)).toBeNull();
    expect(screen.getByTestId(PILL)).toBeTruthy();
    act(() => tracker.store.getState().setTaskSession({ ...ordinarySession, role: "errand" }));
    expect(screen.getByTestId(HEADER)).toBeTruthy();
    expect(screen.queryByTestId(PILL)).toBeNull();
    expect(
      Object.keys(tracker.store.getState().errands.detailOwnersBySessionId[SESSION_ID]),
    ).toHaveLength(2);
    view.unmount();
    expect(tracker.store.getState().errands.detailOwnersBySessionId[SESSION_ID]).toBeUndefined();
  },
);

it.each(["summary", "inspection"] as const)(
  "recognizes %s evidence arriving after the ordinary surfaces mount",
  (evidence) => {
    const tracker = subscriptionTracker();
    render(
      <StateProvider initialState={{ features: { ...defaultState.features, errands: true } }}>
        <TrackedStore tracker={tracker}>
          <ToastProvider>
            <ErrandHeader taskId={TASK_ID} sessionId={SESSION_ID} />
            <MobileSessionPickerRow taskId={TASK_ID} sessionId={SESSION_ID} />
          </ToastProvider>
        </TrackedStore>
      </StateProvider>,
    );
    expect(screen.getByTestId(PILL)).toBeTruthy();
    const row = makeBrowserErrandRow({ errand_id: SESSION_ID });
    act(() => {
      if (evidence === "summary") tracker.store.getState().setConnectionStatus("connected");
      const state = tracker.store.getState();
      if (evidence === "inspection") {
        state.retainErrandInspection("opener", makeInspection(row));
      } else {
        const observation = {
          connectionGeneration: state.connection.generation ?? 0,
          readScopeIdentity: stateReadScopeIdentity(state),
          taskResetEpoch: state.errands.resetEpochByTaskId[TASK_ID] ?? 0,
          observationRevision: state.errands.browserRevisionByTaskId[TASK_ID] ?? 0,
        };
        state.beginErrandBrowserSummaryRead(TASK_ID, observation, "test");
        state.setErrandBrowserSummary(TASK_ID, makeBrowserSummary([row]), observation, "test");
        tracker.store.getState().setConnectionStatus("disconnected");
      }
    });
    expect(screen.getByTestId(HEADER)).toBeTruthy();
    expect(screen.queryByTestId(PILL)).toBeNull();
  },
);

it("keeps the phone picker open and focused when partial inspection recognition has no task identity", () => {
  const tracker = subscriptionTracker();
  render(
    <StateProvider initialState={{ features: { ...defaultState.features, errands: false } }}>
      <TrackedStore tracker={tracker}>
        <ToastProvider>
          <MobileSessionPickerRow taskId={TASK_ID} sessionId="unknown-child" />
        </ToastProvider>
      </TrackedStore>
    </StateProvider>,
  );
  act(() =>
    tracker.store
      .getState()
      .setTaskSession({ ...ordinarySession, id: sessionId("unknown-child"), task_id: taskId("") }),
  );
  const pill = screen.getByTestId(PILL);
  fireEvent.click(pill);
  expect(pill.getAttribute("aria-expanded")).toBe("true");
  const focused = document.activeElement;
  act(() =>
    tracker.store
      .getState()
      .retainErrandInspection(
        "opener",
        makeInspection(makeBrowserErrandRow({ errand_id: "unknown-child" })),
      ),
  );
  expect(screen.getByTestId(PILL)).toBe(pill);
  expect(pill.getAttribute("aria-expanded")).toBe("true");
  expect(document.activeElement).toBe(focused);
  act(() => tracker.store.getState().releaseErrandInspection("opener"));
  expect(screen.getByTestId(PILL)).toBe(pill);
  expect(pill.getAttribute("aria-expanded")).toBe("true");
  expect(document.activeElement).toBe(focused);
});

it.each([true, false])("recognizes late history-only evidence only when enabled=%s", (enabled) => {
  const tracker = subscriptionTracker();
  render(
    <StateProvider initialState={{ features: { ...defaultState.features, errands: enabled } }}>
      <TrackedStore tracker={tracker}>
        <ToastProvider>
          <ErrandHeader taskId={TASK_ID} sessionId={SESSION_ID} />
          <MobileSessionPickerRow taskId={TASK_ID} sessionId={SESSION_ID} />
        </ToastProvider>
      </TrackedStore>
    </StateProvider>,
  );
  expect(screen.getByTestId(PILL)).toBeTruthy();
  act(() => {
    tracker.store.getState().setConnectionStatus("connected");
    const state = tracker.store.getState();
    state.openErrandHistory("history-test", TASK_ID, "session-parent");
    const current = tracker.store.getState();
    const observation = {
      connectionGeneration: current.connection.generation ?? 0,
      readScopeIdentity: stateReadScopeIdentity(current),
      taskResetEpoch: current.errands.resetEpochByTaskId[TASK_ID] ?? 0,
      observationRevision: current.errands.browserRevisionByTaskId[TASK_ID] ?? 0,
    };
    current.beginErrandHistoryRead("history-test", observation, null, "attempt");
    current.setErrandHistoryPage(
      "history-test",
      makeHistoryPage([makeBrowserErrandRow({ errand_id: SESSION_ID })]),
      observation,
      null,
      "attempt",
    );
    tracker.store.getState().setConnectionStatus("disconnected");
  });
  expect(Boolean(screen.queryByTestId(HEADER))).toBe(enabled);
  expect(Boolean(screen.queryByTestId(PILL))).toBe(!enabled);
});

it("retains partial role-only child independently for header and picker until their final release", () => {
  const tracker = subscriptionTracker();
  const tree = (header: boolean, picker: boolean) => (
    <StateProvider initialState={{ features: { ...defaultState.features, errands: false } }}>
      <TrackedStore tracker={tracker}>
        <ToastProvider>
          {header && <ErrandHeader taskId={null} sessionId="partial-child" />}
          {picker && <MobileSessionPickerRow taskId={TASK_ID} sessionId="partial-child" />}
        </ToastProvider>
      </TrackedStore>
    </StateProvider>
  );
  const view = render(tree(true, true));
  act(() =>
    tracker.store.getState().setTaskSession({
      ...ordinarySession,
      id: sessionId("partial-child"),
      task_id: taskId(""),
      role: "errand",
    }),
  );
  expect(screen.queryByTestId(HEADER)).toBeNull();
  expect(screen.getByTestId(PILL)).toBeTruthy();
  expect(
    Object.keys(tracker.store.getState().errands.detailOwnersBySessionId["partial-child"]),
  ).toHaveLength(2);
  view.rerender(tree(false, true));
  expect(
    Object.keys(tracker.store.getState().errands.detailOwnersBySessionId["partial-child"]),
  ).toHaveLength(1);
  expect(tracker.store.getState().taskSessions.items["partial-child"]).toBeDefined();
  view.rerender(tree(false, false));
  expect(tracker.store.getState().errands.detailOwnersBySessionId["partial-child"]).toBeUndefined();
  expect(tracker.store.getState().taskSessions.items["partial-child"]).toBeUndefined();
});
