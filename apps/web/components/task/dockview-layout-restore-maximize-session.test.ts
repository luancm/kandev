import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import { restoreEnvLayout } from "./dockview-layout-restore";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { releaseDockviewSessionRestore } from "@/lib/state/dockview-session-restore-gate";
import * as localStorage from "@/lib/local-storage";

const VALID_COMPONENTS = new Set<string>(["chat", "files", "shell", "git", "terminal", "plan"]);

function makeSavedLayout() {
  return {
    grid: {
      root: {
        type: "branch" as const,
        size: 600,
        data: [
          {
            type: "leaf" as const,
            size: 350,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          {
            type: "leaf" as const,
            size: 800,
            data: { id: "g-center", views: ["chat"], activeView: "chat" },
          },
          {
            type: "leaf" as const,
            size: 450,
            data: { id: "g-right", views: ["git"], activeView: "git" },
          },
        ],
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL" as const,
    },
    panels: {
      files: { id: "files", contentComponent: "files" },
      chat: { id: "chat", contentComponent: "chat" },
      git: { id: "git", contentComponent: "git" },
    },
    activeGroup: "g-center",
  };
}

function makeMaximizeOverlay(panelId: string) {
  return {
    grid: {
      root: {
        type: "branch" as const,
        size: 600,
        data: [
          {
            type: "leaf" as const,
            size: 300,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          {
            type: "leaf" as const,
            size: 1300,
            data: { id: "g-max", views: [panelId], activeView: panelId },
          },
        ],
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL" as const,
    },
    panels: { [panelId]: { id: panelId, contentComponent: "chat" } },
    activeGroup: "g-max",
  };
}

function makeRestoreApi() {
  return {
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({})),
    layout: vi.fn(),
    hasMaximizedGroup: vi.fn(() => false),
    width: 1600,
    height: 600,
    groups: [],
    panels: [],
    activeGroup: { id: "g-center" },
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
    onDidActiveGroupChange: vi.fn(() => ({ dispose: vi.fn() })),
    onDidLayoutChange: vi.fn(() => ({ dispose: vi.fn() })),
  } as unknown as DockviewReadyEvent["api"];
}

function seedIncompleteSessionListLayout(
  activeSessionId: string,
  siblingSessionId: string,
  errandSessionId: string,
): string {
  const siblingPanelId = `session:${siblingSessionId}`;
  const errandPanelId = `session:${errandSessionId}`;
  vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(makeSavedLayout());
  const maximizedLayout = makeMaximizeOverlay(`session:${activeSessionId}`);
  maximizedLayout.grid.root.data[1].data.views.push("plan", errandPanelId);
  maximizedLayout.grid.root.data[1].data.activeView = `session:${activeSessionId}`;
  maximizedLayout.panels.plan = { id: "plan", contentComponent: "plan" };
  maximizedLayout.panels[errandPanelId] = { id: errandPanelId, contentComponent: "chat" };
  vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue({
    maximizedDockviewJson: maximizedLayout,
    preMaximizeLayout: {
      columns: [
        {
          id: "center",
          groups: [],
          tree: {
            type: "leaf",
            size: 600,
            group: {
              id: "g-center",
              panels: [
                {
                  id: siblingPanelId,
                  component: "chat",
                  title: "Sibling session",
                  params: { sessionId: siblingSessionId },
                },
                {
                  id: errandPanelId,
                  component: "chat",
                  title: "Errand child",
                  params: { sessionId: errandSessionId },
                },
              ],
              activePanel: siblingPanelId,
            },
          },
        },
      ],
    },
  });
  return errandPanelId;
}

function makeIncompleteRestoreAppStore(
  activeSessionId: string,
  siblingSessionId: string,
  errandSessionId: string,
) {
  let loaded = false;
  return {
    appStore: {
      getState: () => ({
        environmentIdBySessionId: {
          [activeSessionId]: "env-live",
          [errandSessionId]: "env-live",
        },
        tasks: { activeTaskId: "task-live", activeSessionId },
        taskSessionsByTask: {
          itemsByTaskId: {
            "task-live": loaded ? [{ id: activeSessionId }, { id: siblingSessionId }] : [],
          },
          loadedByTaskId: { "task-live": loaded },
        },
      }),
    },
    markLoaded: () => {
      loaded = true;
    },
  };
}

function flushRaf(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  requestAnimationFrame(() => resolve());
  return promise;
}

describe("on-ready maximize restore", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });

  it("preserves the active session and panel params from a stale-only pre-maximize tree through exit", async () => {
    const activeSessionId = "alive";
    const activePanelId = `session:${activeSessionId}`;
    const staleSessionId = "phantom";
    vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(makeSavedLayout());
    vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue({
      maximizedDockviewJson: makeMaximizeOverlay(activePanelId),
      preMaximizeLayout: {
        columns: [
          {
            id: "center",
            groups: [],
            tree: {
              type: "leaf",
              size: 600,
              group: {
                id: "g-center",
                panels: [
                  {
                    id: `session:${staleSessionId}`,
                    component: "chat",
                    title: "Stale session",
                    params: { keep: "value" },
                  },
                ],
                activePanel: `session:${staleSessionId}`,
              },
            },
          },
        ],
      },
    });
    let appliedLayout = {} as SerializedDockview;
    const api = makeRestoreApi();
    vi.mocked(api.fromJSON).mockImplementation((layout) => {
      appliedLayout = layout;
    });
    vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
    const persistLayout = vi.spyOn(localStorage, "setEnvLayout").mockReturnValue(true);
    const appStore = {
      getState: () => ({
        environmentIdBySessionId: {
          [activeSessionId]: "env-live",
          [staleSessionId]: "env-other",
        },
        tasks: { activeTaskId: "task-live", activeSessionId },
        taskSessionsByTask: {
          itemsByTaskId: { "task-live": [{ id: activeSessionId }] },
          loadedByTaskId: { "task-live": true },
        },
      }),
    };
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-live" });

    expect(
      restoreEnvLayout(
        api,
        "env-live",
        appStore as unknown as Parameters<typeof restoreEnvLayout>[2],
        VALID_COMPONENTS,
      ),
    ).toBe(true);
    expect(useDockviewStore.getState().preMaximizeLayout?.columns[0]?.tree).toMatchObject({
      type: "leaf",
      group: {
        id: "g-center",
        panels: [{ id: activePanelId, params: { keep: "value", sessionId: activeSessionId } }],
        activePanel: activePanelId,
      },
    });

    useDockviewStore.getState().exitMaximizedLayout();
    await flushRaf();

    const persistedLayout = vi.mocked(persistLayout).mock.calls.at(-1)?.[1] as {
      panels?: Record<string, { params?: Record<string, string> }>;
    };
    expect(persistedLayout.panels).toHaveProperty(activePanelId);
    expect(persistedLayout.panels).not.toHaveProperty(`session:${staleSessionId}`);
    expect(persistedLayout.panels?.[activePanelId]?.params).toMatchObject({
      keep: "value",
      sessionId: activeSessionId,
    });
  });
});

describe("on-ready maximize restore with an incomplete session list", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });
  it("holds session panels until ordinary identity loads, then restores only ordinary tabs", async () => {
    const activeSessionId = "alive";
    const siblingSessionId = "ordinary-sibling";
    const errandSessionId = "errand-child";
    const siblingPanelId = `session:${siblingSessionId}`;
    const errandPanelId = seedIncompleteSessionListLayout(
      activeSessionId,
      siblingSessionId,
      errandSessionId,
    );
    let appliedLayout = {} as SerializedDockview;
    const api = makeRestoreApi();
    vi.mocked(api.fromJSON).mockImplementation((layout) => {
      appliedLayout = layout;
    });
    vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
    const persistLayout = vi.spyOn(localStorage, "setEnvLayout").mockReturnValue(true);
    const { appStore, markLoaded } = makeIncompleteRestoreAppStore(
      activeSessionId,
      siblingSessionId,
      errandSessionId,
    );
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-live" });

    expect(
      restoreEnvLayout(
        api,
        "env-live",
        appStore as unknown as Parameters<typeof restoreEnvLayout>[2],
        VALID_COMPONENTS,
      ),
    ).toBe(true);

    const initialRestores = vi.mocked(api.fromJSON).mock.calls.map(([layout]) => layout);
    expect(initialRestores.length).toBeGreaterThan(0);
    expect(
      initialRestores.every(
        (layout) => !Object.keys(layout.panels).some((id) => id.startsWith("session:")),
      ),
    ).toBe(true);
    expect(api.addPanel).not.toHaveBeenCalled();
    expect(useDockviewStore.getState().isRestoringLayout).toBe(true);
    expect(persistLayout).not.toHaveBeenCalled();

    markLoaded();
    expect(releaseDockviewSessionRestore(api, [activeSessionId, siblingSessionId])).toBe(true);
    await flushRaf();

    const restoredLayouts = vi.mocked(api.fromJSON).mock.calls.map(([layout]) => layout);
    expect(
      restoredLayouts.some((layout) =>
        Object.keys(layout.panels).includes(`session:${activeSessionId}`),
      ),
    ).toBe(true);
    expect(
      restoredLayouts.every((layout) => !Object.keys(layout.panels).includes(errandPanelId)),
    ).toBe(true);
    expect(api.addPanel).not.toHaveBeenCalled();
    const restoredPreMaximizeLayout = useDockviewStore.getState().preMaximizeLayout;
    const preMaxTree = restoredPreMaximizeLayout?.columns[0]?.tree;
    const preMaxPanels = preMaxTree?.type === "leaf" ? preMaxTree.group.panels : [];
    expect(preMaxPanels).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: siblingPanelId }),
        expect.objectContaining({ id: `session:${activeSessionId}` }),
      ]),
    );
    expect(preMaxPanels).not.toContainEqual(expect.objectContaining({ id: errandPanelId }));
    expect(persistLayout).not.toHaveBeenCalled();
    expect(useDockviewStore.getState().isRestoringLayout).toBe(false);
  });
});
