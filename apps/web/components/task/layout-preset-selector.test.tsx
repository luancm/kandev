import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getBuiltInLayoutProfile } from "@/lib/layout/layout-profiles";

type LayoutAppState = {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  taskSessionsByTask: {
    itemsByTaskId: Record<string, Array<{ id: string }>>;
    loadedByTaskId: Record<string, boolean>;
  };
};
type LayoutAppListener = (state: LayoutAppState, previousState: LayoutAppState) => void;
type LayoutDockState = { currentLayoutEnvId: string | null; api: { id: string } | null };
type LayoutDockListener = (state: LayoutDockState, previousState: LayoutDockState) => void;

const mocks = vi.hoisted(() => ({
  finePointer: true,
  isMobile: false,
  savedLayouts: [] as Array<Record<string, unknown>>,
  sessionsLoaded: true,
  loadSessions: vi.fn(),
  appListeners: new Set<LayoutAppListener>(),
  dockListeners: new Set<LayoutDockListener>(),
  taskAId: "task-1",
  taskBId: "task-2",
  sessionAId: "session-1",
  sessionBId: "session-b",
  savedSessionId: "session-a",
  environmentAId: "environment-a",
  environmentBId: "environment-b",
  appState: {
    tasks: { activeTaskId: null, activeSessionId: null },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {} },
  } as LayoutAppState,
  dockState: { currentLayoutEnvId: null, api: null } as LayoutDockState,
  appliedLayoutsByEnvironment: {} as Record<string, unknown>,
  updateUserSettings: vi.fn(),
  setUserSettings: vi.fn(),
  toastError: vi.fn(),
  applyCustomLayout: vi.fn(),
  applyBuiltInPreset: vi.fn(),
  resetLayout: vi.fn(),
  tooltipOpen: false,
  requestTooltipOpen: null as ((open: boolean) => void) | null,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { name?: string }) => {
      if (key === "task:delete2") return `Delete ${values?.name ?? "saved layout"}`;
      if (key === "task:deleteLayoutConfirm") return `Delete ${values?.name ?? "saved layout"}?`;
      return key;
    },
  }),
}));

vi.mock("@/lib/i18n", () => ({
  i18n: { language: "en", on: vi.fn() },
  t: (key: string) => key,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.finePointer, isMobile: mocks.isMobile }),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => mocks.toastError(...args) },
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: (...args: unknown[]) => mocks.updateUserSettings(...args),
}));

vi.mock("@/lib/ssr/user-settings", () => ({
  mapUserSettingsResponse: (response: { settings: { saved_layouts: unknown[] } }) => ({
    savedLayouts: response.settings.saved_layouts,
  }),
}));

vi.mock("@/components/state-provider", () => {
  const appStore = {
    getState: () => ({
      ...mocks.appState,
      userSettings: { savedLayouts: mocks.savedLayouts },
      setUserSettings: mocks.setUserSettings,
    }),
    subscribe: (listener: LayoutAppListener) => {
      mocks.appListeners.add(listener);
      return () => mocks.appListeners.delete(listener);
    },
  };
  return {
    useAppStore: (selector: (state: unknown) => unknown) =>
      selector({
        userSettings: { savedLayouts: mocks.savedLayouts },
        ...mocks.appState,
        setUserSettings: mocks.setUserSettings,
      }),
    useAppStoreApi: () => appStore,
  };
});

vi.mock("@/lib/state/dockview-store", () => {
  const getState = () => ({
    ...mocks.dockState,
    applyCustomLayout: mocks.applyCustomLayout,
    applyBuiltInPreset: mocks.applyBuiltInPreset,
    resetLayout: mocks.resetLayout,
    captureCurrentLayout: vi.fn(() => getBuiltInLayoutProfile("default").layout),
  });
  const useDockviewStore = Object.assign(
    (selector: (state: unknown) => unknown) =>
      selector({
        ...getState(),
      }),
    {
      getState,
      subscribe: (listener: LayoutDockListener) => {
        mocks.dockListeners.add(listener);
        return () => mocks.dockListeners.delete(listener);
      },
    },
  );
  return { useDockviewStore };
});

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ isLoaded: mocks.sessionsLoaded, loadSessions: mocks.loadSessions }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({
    children,
    open,
    onOpenChange,
  }: {
    children: React.ReactNode;
    open?: boolean;
    onOpenChange?: (open: boolean) => void;
  }) => {
    mocks.tooltipOpen = open ?? false;
    mocks.requestTooltipOpen = onOpenChange ?? null;
    return <>{children}</>;
  },
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { LayoutPresetSelector } from "./layout-preset-selector";

const SAVED_DELETE_TEST_ID = "layout-saved-delete";
const SAVED_DELETE_POPOVER_TEST_ID = "layout-saved-delete-confirm-popover";
const SAVED_DELETE_CONFIRM_TEST_ID = "layout-saved-delete-confirm";
const SECOND_LAYOUT_ID = "second-layout";

function savedLayout(overrides: Record<string, unknown> = {}) {
  return {
    id: "saved-layout",
    name: "Saved layout",
    is_default: false,
    layout: getBuiltInLayoutProfile("default").layout,
    created_at: "2026-08-20T00:00:00.000Z",
    ...overrides,
  };
}

function openSavedLayouts() {
  const trigger = screen.getByTestId("layout-preset-trigger");
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(trigger);
  return waitFor(() =>
    expect(screen.getAllByTestId(SAVED_DELETE_TEST_ID).length).toBeGreaterThan(0),
  );
}

function deleteAction(layoutId: string) {
  const action = screen
    .getAllByTestId(SAVED_DELETE_TEST_ID)
    .find((candidate) => candidate.getAttribute("data-layout-id") === layoutId);
  expect(action).toBeTruthy();
  return action!;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function setAppTaskSelection(taskId: string, sessionId: string) {
  const previous = mocks.appState;
  mocks.appState = {
    ...mocks.appState,
    tasks: { activeTaskId: taskId, activeSessionId: sessionId },
  };
  mocks.appListeners.forEach((listener) => listener(mocks.appState, previous));
}

function setLayoutIdentity(environmentId: string, api: { id: string }) {
  const previous = mocks.dockState;
  mocks.dockState = { currentLayoutEnvId: environmentId, api };
  mocks.dockListeners.forEach((listener) => listener(mocks.dockState, previous));
}

async function selectSavedLayout() {
  await openSavedLayouts();
  fireEvent.click(screen.getByText("Saved layout"));
}

beforeEach(() => {
  mocks.finePointer = true;
  mocks.isMobile = false;
  mocks.savedLayouts = [savedLayout()];
  mocks.sessionsLoaded = true;
  mocks.loadSessions.mockReset();
  mocks.appListeners.clear();
  mocks.dockListeners.clear();
  mocks.appState = {
    tasks: { activeTaskId: mocks.taskAId, activeSessionId: mocks.sessionAId },
    taskSessionsByTask: {
      itemsByTaskId: { [mocks.taskAId]: [{ id: mocks.savedSessionId }] },
      loadedByTaskId: { [mocks.taskAId]: true },
    },
  };
  mocks.dockState = { currentLayoutEnvId: mocks.environmentAId, api: { id: "api-a" } };
  mocks.appliedLayoutsByEnvironment = {};
  mocks.updateUserSettings.mockReset();
  mocks.setUserSettings.mockReset();
  mocks.toastError.mockReset();
  mocks.applyCustomLayout.mockReset();
  mocks.applyCustomLayout.mockImplementation((layout: unknown) => {
    const environmentId = mocks.dockState.currentLayoutEnvId;
    if (environmentId) mocks.appliedLayoutsByEnvironment[environmentId] = layout;
  });
  mocks.applyBuiltInPreset.mockReset();
  mocks.resetLayout.mockReset();
  mocks.tooltipOpen = false;
  mocks.requestTooltipOpen = null;
  mocks.setUserSettings.mockImplementation(
    (settings: { savedLayouts?: Array<Record<string, unknown>> }) => {
      if (settings.savedLayouts) mocks.savedLayouts = settings.savedLayouts;
    },
  );
  mocks.updateUserSettings.mockResolvedValue({ settings: { saved_layouts: [] } });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("LayoutPresetSelector saved-layout deletion", () => {
  it("hands the phone menu off to a sheet without applying the layout", async () => {
    mocks.isMobile = true;
    mocks.finePointer = false;
    render(<LayoutPresetSelector />);
    const trigger = screen.getByTestId("layout-preset-trigger");
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));
    const sheet = await screen.findByRole("dialog", { name: "Delete Saved layout?" });
    expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
    expect(document.querySelector('[data-slot="dropdown-menu-content"]')).toBeNull();
    fireEvent.click(within(sheet).getByRole("button", { name: "common:cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.updateUserSettings).not.toHaveBeenCalled();
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));
    fireEvent.click(await screen.findByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1));
    expect(mocks.savedLayouts).toEqual([]);
  });
  it("anchors desktop confirmation to the menu action and keeps cancel local", async () => {
    render(<LayoutPresetSelector />);
    await openSavedLayouts();

    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));

    const popover = screen.getByTestId(SAVED_DELETE_POPOVER_TEST_ID);
    expect(popover).toBeTruthy();
    expect(popover.hasAttribute("data-confirmation-boundary")).toBe(true);
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(
      within(screen.getByTestId(SAVED_DELETE_POPOVER_TEST_ID)).getByRole("button", {
        name: "common:cancel",
      }),
    );
    expect(mocks.updateUserSettings).not.toHaveBeenCalled();
  });
  it("morphs the menu row into touch-sized actions without stacking an overlay", async () => {
    mocks.finePointer = false;
    render(<LayoutPresetSelector />);
    await openSavedLayouts();

    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));

    const inline = screen.getByTestId("layout-saved-delete-inline-confirmation");
    expect(screen.queryByTestId(SAVED_DELETE_POPOVER_TEST_ID)).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(within(inline).getByTestId("layout-saved-delete-confirm").className).toContain(
      "min-w-11",
    );

    fireEvent.click(within(inline).getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() =>
      expect(mocks.updateUserSettings).toHaveBeenCalledWith({ saved_layouts: [] }),
    );
  });
});

describe("LayoutPresetSelector delayed layout navigation ownership", () => {
  it("does not materialize a delayed task A layout in task B environment", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    setAppTaskSelection(mocks.taskBId, mocks.sessionBId);
    setLayoutIdentity(mocks.environmentBId, { id: "api-b" });
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.appliedLayoutsByEnvironment).toEqual({});
  });

  it("rejects a delayed task A layout after navigation returns from B to A", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    setAppTaskSelection(mocks.taskBId, mocks.sessionBId);
    setAppTaskSelection(mocks.taskAId, mocks.sessionAId);
    setLayoutIdentity(mocks.environmentBId, { id: "api-b" });
    setLayoutIdentity(mocks.environmentAId, { id: "api-a" });
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.appliedLayoutsByEnvironment).toEqual({});
  });

  it("does not apply through a replaced Dockview API even when the environment ID is unchanged", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    setLayoutIdentity(mocks.environmentAId, { id: "replacement-api-a" });
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.appliedLayoutsByEnvironment).toEqual({});
  });
});

describe("LayoutPresetSelector delayed layout completion ownership", () => {
  it("applies the layout when the captured task, environment, and API still own the request", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = true;
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ id: "saved-layout" }),
      expect.objectContaining({ envId: mocks.environmentAId, sessionIds: [mocks.savedSessionId] }),
    );
    expect(mocks.appliedLayoutsByEnvironment).toHaveProperty(mocks.environmentAId);
  });

  it("does not apply a pending layout after the selector unmounts", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    const { unmount } = render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    unmount();
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.appliedLayoutsByEnvironment).toEqual({});
  });

  it("lets only the latest saved-layout application finish", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const first = savedLayout({ id: "first-layout", name: "First layout" });
    const second = savedLayout({ id: SECOND_LAYOUT_ID, name: "Second layout" });
    mocks.savedLayouts = [first, second];
    const firstRead = deferred<void>();
    const secondRead = deferred<void>();
    mocks.loadSessions
      .mockReturnValueOnce(firstRead.promise)
      .mockReturnValueOnce(secondRead.promise);
    render(<LayoutPresetSelector />);

    await openSavedLayouts();
    fireEvent.click(screen.getByText("First layout"));
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledTimes(1));
    await openSavedLayouts();
    fireEvent.click(screen.getByText("Second layout"));
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledTimes(2));
    await act(async () => {
      firstRead.resolve();
      await firstRead.promise;
    });
    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = true;
    await act(async () => {
      secondRead.resolve();
      await secondRead.promise;
    });

    expect(mocks.applyCustomLayout).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ id: SECOND_LAYOUT_ID }),
      expect.objectContaining({ envId: mocks.environmentAId, sessionIds: [mocks.savedSessionId] }),
    );
  });
});

describe("LayoutPresetSelector layout selection invalidation", () => {
  it("lets a built-in preset selection supersede a pending saved layout", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    await openSavedLayouts();
    const builtInPlan = screen
      .getAllByTestId("layout-preset-item")
      .find((item) => item.getAttribute("data-preset-id") === "plan");
    expect(builtInPlan).toBeTruthy();
    fireEvent.click(builtInPlan!);

    expect(mocks.applyBuiltInPreset).toHaveBeenCalledExactlyOnceWith("plan", true);
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.appliedLayoutsByEnvironment).toEqual({});
  });

  it("lets reset supersede a pending saved layout", async () => {
    mocks.sessionsLoaded = false;
    mocks.appState.taskSessionsByTask.loadedByTaskId[mocks.taskAId] = false;
    const sessionRead = deferred<void>();
    mocks.loadSessions.mockReturnValue(sessionRead.promise);
    render(<LayoutPresetSelector />);

    await selectSavedLayout();
    await waitFor(() => expect(mocks.loadSessions).toHaveBeenCalledWith(true));
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId("layout-reset-item"));

    expect(mocks.resetLayout).toHaveBeenCalledOnce();
    await act(async () => {
      sessionRead.resolve();
      await sessionRead.promise;
    });

    expect(mocks.applyCustomLayout).not.toHaveBeenCalled();
    expect(mocks.appliedLayoutsByEnvironment).toEqual({});
  });
});

describe("LayoutPresetSelector saved-layout persistence", () => {
  it("serializes deletions and derives each payload from the latest saved layouts", async () => {
    const first = savedLayout({ id: "first-layout", name: "First layout" });
    const second = savedLayout({ id: SECOND_LAYOUT_ID, name: "Second layout" });
    const retained = savedLayout({ id: "retained-layout", name: "Retained layout" });
    mocks.savedLayouts = [first, second, retained];
    const firstResponse = deferred<{
      settings: { saved_layouts: Array<Record<string, unknown>> };
    }>();
    const secondResponse = deferred<{
      settings: { saved_layouts: Array<Record<string, unknown>> };
    }>();
    mocks.updateUserSettings
      .mockReturnValueOnce(firstResponse.promise)
      .mockReturnValueOnce(secondResponse.promise);

    render(<LayoutPresetSelector />);
    await openSavedLayouts();
    fireEvent.click(deleteAction("first-layout"));
    fireEvent.click(screen.getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1));

    await openSavedLayouts();
    fireEvent.click(deleteAction(SECOND_LAYOUT_ID));
    fireEvent.click(screen.getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1);

    firstResponse.resolve({ settings: { saved_layouts: [second, retained] } });
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(2));
    expect(mocks.updateUserSettings).toHaveBeenLastCalledWith({ saved_layouts: [retained] });
    secondResponse.resolve({ settings: { saved_layouts: [retained] } });
  });

  it("suppresses tooltip reopening after confirmation and clears the close timer on unmount", async () => {
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    const { unmount } = render(<LayoutPresetSelector />);
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));
    fireEvent.click(screen.getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1));

    const timerCallIndex = setTimeoutSpy.mock.calls.findIndex(([, delay]) => delay === 200);
    expect(timerCallIndex).toBeGreaterThanOrEqual(0);
    const timerId = setTimeoutSpy.mock.results[timerCallIndex]?.value;
    act(() => mocks.requestTooltipOpen?.(true));
    expect(mocks.tooltipOpen).toBe(false);

    unmount();
    expect(clearTimeoutSpy).toHaveBeenCalledWith(timerId);
  });

  it("keeps the existing localized error toast when deletion fails", async () => {
    mocks.updateUserSettings.mockRejectedValueOnce(new Error("delete failed"));
    render(<LayoutPresetSelector />);
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId(SAVED_DELETE_POPOVER_TEST_ID)).getByTestId(
        SAVED_DELETE_CONFIRM_TEST_ID,
      ),
    );

    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledTimes(1));
    expect(mocks.toastError).toHaveBeenCalledWith("task:failedToDeleteLayout", {
      description: "delete failed",
    });
  });
});
