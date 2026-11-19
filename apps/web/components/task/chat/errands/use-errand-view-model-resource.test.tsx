import { useState, type ReactNode } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import { sessionId, taskId } from "@/lib/types/http";
import { MobileSessionsPicker } from "../../mobile/mobile-sessions-section";
import { MobileSessionPickerRow } from "../../mobile/mobile-session-picker-row";

const api = vi.hoisted(() => ({ inspectErrand: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => api);

type Store = ReturnType<typeof useAppStoreApi>;
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

describe("ordinary phone picker subscription budget", () => {
  it.each([true, false])("adds no Errand listeners with errands=%s", (enabled) => {
    const tracker = subscriptionTracker();
    const baseline = <MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_ID} fullWidth />;
    const consumer = <MobileSessionPickerRow taskId={TASK_ID} sessionId={SESSION_ID} />;
    const tree = (children: ReactNode) => (
      <StateProvider initialState={{ features: { ...defaultState.features, errands: enabled } }}>
        <TrackedStore tracker={tracker}>{children}</TrackedStore>
      </StateProvider>
    );
    const view = render(tree(baseline));
    const baselineListeners = tracker.active;
    const baselineCallbacks = tracker.notifyUnrelated();
    view.rerender(tree(consumer));
    // @covers AC-TASKS-ERRAND-HISTORY-001.1
    expect(tracker.active).toBe(baselineListeners);
    expect(tracker.notifyUnrelated()).toBe(baselineCallbacks);
    act(() => tracker.store.getState().setTaskSession({ ...ordinarySession, name: "Renamed" }));
    expect(screen.getByTestId(PILL).textContent).toContain("Renamed");
    expect(tracker.store.getState().errands.detailOwnersBySessionId).toEqual({});
    expect(api.inspectErrand).not.toHaveBeenCalled();
    view.unmount();
    expect(tracker.active).toBe(0);
    expect(tracker.notifyUnrelated()).toBe(0);
  });
});
