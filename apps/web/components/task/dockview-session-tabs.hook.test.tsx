import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { DockviewApi } from "dockview-react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { LayoutState } from "@/lib/state/layout-manager";
import type { TaskSession, TaskId } from "@/lib/types/http";
import {
  makeErrandTabAppStore,
  makeReorderingAutoSessionApi,
  type ErrandTabSession,
} from "./dockview-session-tabs.test-utils";
import { runAutoSessionTabEffect, useAutoSessionTab } from "./dockview-session-tabs";
import { hideSessionPanel } from "./dockview-hidden-session-panels";

const TASK_ID = "task-1" as TaskId;
const ACTIVE_SESSION_ID = "session-a";
const SIBLING_SESSION_ID = "session-b";

function Harness() {
  useAutoSessionTab(ACTIVE_SESSION_ID);
  return null;
}

let capturedStore: StoreApi<AppState> | null = null;

function FocusHarness() {
  capturedStore = useAppStoreApi();
  useAutoSessionTab(ACTIVE_SESSION_ID);
  return null;
}

function renderHookWithHydratedSessions() {
  const sessions = [ACTIVE_SESSION_ID, SIBLING_SESSION_ID].map((id) => ({ id }) as TaskSession);

  return render(
    <StateProvider
      initialState={{
        ...defaultState,
        tasks: {
          ...defaultState.tasks,
          activeTaskId: TASK_ID,
          activeSessionId: ACTIVE_SESSION_ID,
        },
        taskSessionsByTask: {
          ...defaultState.taskSessionsByTask,
          itemsByTaskId: { [TASK_ID]: sessions },
        },
      }}
    >
      <Harness />
    </StateProvider>,
  );
}

afterEach(() => {
  cleanup();
  useDockviewStore.setState({
    api: null,
    currentLayoutEnvId: null,
    preMaximizeLayout: null,
    maximizedGroupId: null,
  });
});

describe("useAutoSessionTab", () => {
  it("reconciles every hydrated session when Dockview becomes ready later", () => {
    // @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.1
    useDockviewStore.setState({ api: null });
    renderHookWithHydratedSessions();

    const { api } = makeReorderingAutoSessionApi();
    act(() => {
      useDockviewStore.setState({ api: api as DockviewApi });
    });

    expect(api.panels.map((panel) => panel.id)).toEqual(
      expect.arrayContaining([`session:${ACTIVE_SESSION_ID}`, `session:${SIBLING_SESSION_ID}`]),
    );
  });
});

describe("useAutoSessionTab errand sessions", () => {
  const ERRAND_SESSION_ID = "session-errand";

  function renderWithErrandSession(errandsEnabled: boolean) {
    useDockviewStore.setState({ api: null });
    render(
      <StateProvider
        initialState={{
          ...defaultState,
          features: { ...defaultState.features, errands: errandsEnabled },
          tasks: {
            ...defaultState.tasks,
            activeTaskId: TASK_ID,
            activeSessionId: ACTIVE_SESSION_ID,
          },
          taskSessionsByTask: {
            ...defaultState.taskSessionsByTask,
            itemsByTaskId: {
              [TASK_ID]: [
                { id: ACTIVE_SESSION_ID } as TaskSession,
                { id: SIBLING_SESSION_ID } as TaskSession,
                { id: ERRAND_SESSION_ID, role: "errand" } as TaskSession,
              ],
            },
          },
        }}
      >
        <Harness />
      </StateProvider>,
    );
    const { api } = makeReorderingAutoSessionApi();
    act(() => {
      useDockviewStore.setState({ api: api as DockviewApi });
    });
    return api;
  }

  it("leaves the errand out of the automatic tabs while the flag is on", () => {
    const api = renderWithErrandSession(true);

    expect(api.getPanel(`session:${SIBLING_SESSION_ID}`)).not.toBeNull();
    expect(api.getPanel(`session:${ERRAND_SESSION_ID}`)).toBeNull();
  });

  it("adds the errand as an ordinary tab while the flag is off", () => {
    const api = renderWithErrandSession(false);

    expect(api.getPanel(`session:${ERRAND_SESSION_ID}`)).not.toBeNull();
  });
});

describe("runAutoSessionTabEffect errand sessions", () => {
  const PARENT = "session-parent";
  const ERRAND = "session-errand";
  const OTHER = "session-other";
  const SESSIONS: ErrandTabSession[] = [
    { id: PARENT },
    { id: ERRAND, role: "errand" },
    { id: OTHER },
  ];

  function runEffect(
    errandsEnabled: boolean,
    run: {
      sessions?: ErrandTabSession[];
      errandIds?: string[];
      effectiveSessionId?: string;
      hidden?: string[];
      openPanels?: string[];
    } = {},
  ) {
    const reordering = makeReorderingAutoSessionApi();
    const { sessions = SESSIONS, errandIds = [], hidden = [], openPanels = [] } = run;
    const store = makeErrandTabAppStore({ taskId: TASK_ID, errandsEnabled, sessions, errandIds });
    for (const id of openPanels) {
      reordering.api.addPanel({ id: `session:${id}`, component: "chat", inactive: true });
    }
    useDockviewStore.setState({ api: reordering.api, currentLayoutEnvId: null });
    for (const id of hidden) hideSessionPanel(reordering.api, id, TASK_ID);
    runAutoSessionTabEffect(run.effectiveSessionId ?? PARENT, store.appStore as never, {
      sessionTabCreatedRef: { current: new Set<string>() },
      prevTaskIdRef: { current: "task-old" },
      prevSessionIdRef: { current: "session-old" },
    });
    return { ...reordering, ...store };
  }

  it.each([
    { name: "adds no errand tab while the flag is on", enabled: true, run: {}, errandTab: false },
    { name: "adds an errand tab while the flag is off", enabled: false, run: {}, errandTab: true },
    {
      name: "skips an errand known from its row before its role arrives",
      enabled: true,
      run: { sessions: [{ id: PARENT }, { id: ERRAND }, { id: OTHER }], errandIds: [ERRAND] },
      errandTab: false,
    },
    {
      name: "keeps an errand tab that the user opened or a layout restored",
      enabled: true,
      run: { openPanels: [ERRAND] },
      errandTab: true,
    },
  ])("$name", ({ enabled, run, errandTab }) => {
    const { api } = runEffect(enabled, run);

    expect(api.getPanel(`session:${PARENT}`)).not.toBeNull();
    expect(api.getPanel(`session:${OTHER}`)).not.toBeNull();
    expect(api.getPanel(`session:${ERRAND}`) !== null).toBe(errandTab);
  });

  it("shows an errand's tab when it is the active session", () => {
    const { centerActivePanelId } = runEffect(true, { effectiveSessionId: ERRAND });
    expect(centerActivePanelId()).toBe(`session:${ERRAND}`);
  });

  it("never auto-selects an errand in place of a hidden active session", () => {
    const { setActiveSessionAuto } = runEffect(true, { hidden: [PARENT] });
    expect(setActiveSessionAuto).toHaveBeenCalledExactlyOnceWith(TASK_ID, OTHER);
  });
});

describe("useAutoSessionTab maximize hydration", () => {
  it("reconciles saved maximize panels when session loading completes without changing ids", () => {
    const { api } = makeReorderingAutoSessionApi();
    const activePanelId = `session:${ACTIVE_SESSION_ID}`;
    const siblingPanelId = `session:${SIBLING_SESSION_ID}`;
    const stalePanelId = "session:saved-stale";
    const preMaximizeLayout = {
      columns: [
        {
          id: "center",
          groups: [
            {
              id: "center",
              panels: [
                {
                  id: activePanelId,
                  component: "chat",
                  title: "Agent",
                  params: { sessionId: ACTIVE_SESSION_ID },
                },
                {
                  id: siblingPanelId,
                  component: "chat",
                  title: "Sibling",
                  params: { sessionId: SIBLING_SESSION_ID },
                },
                {
                  id: stalePanelId,
                  component: "chat",
                  title: "Stale",
                  params: { sessionId: "saved-stale" },
                },
              ],
              activePanel: activePanelId,
            },
          ],
        },
      ],
    } satisfies LayoutState;
    useDockviewStore.setState({
      api: api as DockviewApi,
      currentLayoutEnvId: "env-a",
      preMaximizeLayout,
    });
    capturedStore = null;

    render(
      <StateProvider
        initialState={{
          ...defaultState,
          tasks: {
            ...defaultState.tasks,
            activeTaskId: TASK_ID,
            activeSessionId: ACTIVE_SESSION_ID,
          },
          taskSessionsByTask: {
            ...defaultState.taskSessionsByTask,
            itemsByTaskId: {
              [TASK_ID]: [
                { id: ACTIVE_SESSION_ID } as TaskSession,
                { id: SIBLING_SESSION_ID } as TaskSession,
              ],
            },
            loadedByTaskId: { [TASK_ID]: false },
          },
        }}
      >
        <FocusHarness />
      </StateProvider>,
    );

    expect(capturedStore).not.toBeNull();
    const store = capturedStore as unknown as StoreApi<AppState>;
    const initialSessionIds = store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID] ?? [];
    act(() => {
      store.setState({
        taskSessionsByTask: {
          ...store.getState().taskSessionsByTask,
          itemsByTaskId: { [TASK_ID]: initialSessionIds },
          loadedByTaskId: { [TASK_ID]: true },
        },
      });
    });

    const restoredPanelIds =
      useDockviewStore
        .getState()
        .preMaximizeLayout?.columns[0]?.groups[0]?.panels.map((panel) => panel.id) ?? [];
    expect(restoredPanelIds).toEqual(expect.arrayContaining([activePanelId, siblingPanelId]));
    expect(restoredPanelIds).not.toContain(stalePanelId);
  });
});

describe("useAutoSessionTab focus requests", () => {
  it("subscribes to a same-session focus request and acknowledges after activation", () => {
    const { api, activationSequence, centerActivePanelId } = makeReorderingAutoSessionApi("files");
    const route = {
      operation_id: "workflow-session:task-1:step-b:entry:00000000000000000042:profile::reuse",
      destination_step_id: "step-b",
      entry_identity: "entry:00000000000000000042",
      destination_session_id: ACTIVE_SESSION_ID,
      phase: "committed",
    };
    capturedStore = null;

    render(
      <StateProvider
        initialState={{
          ...defaultState,
          tasks: {
            ...defaultState.tasks,
            activeTaskId: TASK_ID,
            activeSessionId: ACTIVE_SESSION_ID,
          },
          kanban: {
            ...defaultState.kanban,
            tasks: [
              {
                id: TASK_ID,
                workflowId: "workflow-1",
                workflowStepId: "step-b",
                title: "Task",
                position: 0,
                updatedAt: "2026-09-19T10:00:01Z",
                metadata: { workflow_session_route: route },
              },
            ],
          },
          taskSessionsByTask: {
            ...defaultState.taskSessionsByTask,
            itemsByTaskId: {
              [TASK_ID]: [
                { id: ACTIVE_SESSION_ID } as TaskSession,
                { id: SIBLING_SESSION_ID } as TaskSession,
              ],
            },
          },
        }}
      >
        <FocusHarness />
      </StateProvider>,
    );

    expect(capturedStore).not.toBeNull();
    const store = capturedStore as unknown as StoreApi<AppState>;
    act(() => {
      useDockviewStore.setState({ api: api as DockviewApi });
    });
    activationSequence.length = 0;

    const acknowledge = store.getState().acknowledgeWorkflowSessionFocus;
    const acknowledgeSpy = vi.fn((requestId: number) => acknowledge(requestId));
    store.setState({ acknowledgeWorkflowSessionFocus: acknowledgeSpy });

    act(() => {
      const requestId = store.getState().beginWorkflowSessionFocus({
        taskId: TASK_ID,
        workflowId: "workflow-1",
        destinationStepId: "step-b",
        presentationToken: 1,
        navigationRevision: 0,
      });
      expect(requestId).toBe(1);
      store.getState().bindWorkflowSessionFocus({
        requestId: requestId!,
        presentationToken: 1,
        entryIdentity: route.entry_identity,
      });
      store.getState().reconcileWorkflowSessionFocus(TASK_ID);
    });

    expect(centerActivePanelId()).toBe(`session:${ACTIVE_SESSION_ID}`);
    expect(acknowledgeSpy).toHaveBeenCalledTimes(1);
    expect(acknowledgeSpy).toHaveBeenCalledWith(1);
    expect(activationSequence.filter((id) => id === `session:${ACTIVE_SESSION_ID}`)).toHaveLength(
      1,
    );
    expect(store.getState().workflowSessionFocus.request).toBeNull();
  });
});
