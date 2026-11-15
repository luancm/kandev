import { describe, expect, it } from "vitest";
import { defaultState, mergeInitialState } from "./default-state";
import { createAppStore, type HydrationState } from "./store";
import {
  ERRAND_PARENT_ID,
  ERRAND_TASK_ID,
  makeErrand,
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeHistoryPage,
  makeInspection,
} from "./slices/errands/errands.test-helpers";
import { stateReadScopeIdentity } from "./shared-resource-reads";

describe("turn hydration state", () => {
  it("defaults and deep-merges loaded session markers", () => {
    const state = mergeInitialState({
      turns: {
        bySession: { "session-1": [] },
        activeBySession: {},
        loadedBySession: {},
      },
    } as unknown as HydrationState);

    expect(defaultState.turns.loadedBySession).toEqual({});
    expect(state.turns.loadedBySession).toEqual({ "session-1": true });
  });

  it("keeps a bounded turn window separate from full-history completion", () => {
    const state = mergeInitialState({
      turns: {
        bySession: { "session-1": [{ id: "turn-window" }] },
        activeBySession: { "session-1": "turn-window" },
        loadedBySession: {},
        windowCoverageBySession: {
          "session-1": { messageIds: ["message-1"], activeTurnObserved: true },
        },
      },
    } as unknown as HydrationState);

    expect(state.turns.loadedBySession).toEqual({});
    expect(state.turns.windowCoverageBySession?.["session-1"]).toEqual({
      messageIds: ["message-1"],
      activeTurnObserved: true,
    });
  });
});

describe("errands hydration state", () => {
  it("clears runtime browser ownership from a previously retained Errands snapshot", () => {
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    const running = makeErrand();
    store.getState().upsertErrand(running);
    const observation = {
      connectionGeneration: store.getState().connection.generation ?? 0,
      readScopeIdentity: stateReadScopeIdentity(store.getState()),
      taskResetEpoch: 0,
      observationRevision: store.getState().errands.browserRevisionByTaskId[ERRAND_TASK_ID],
    };
    const outstanding = makeBrowserErrandRow({
      status: "running",
      result_state: "none",
      live: true,
      outstanding: true,
    });
    const finished = makeBrowserErrandRow({ errand_id: "finished-child" });
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary");
    store
      .getState()
      .setErrandBrowserSummary(
        ERRAND_TASK_ID,
        makeBrowserSummary([outstanding]),
        observation,
        "summary",
      );
    store.getState().openErrandHistory("panel", ERRAND_TASK_ID, ERRAND_PARENT_ID);
    store.getState().beginErrandHistoryRead("panel", observation, null, "history");
    store
      .getState()
      .setErrandHistoryPage("panel", makeHistoryPage([finished]), observation, null, "history");
    store.getState().retainErrandInspection("child", makeInspection(finished));
    const retained = store.getState().errands;
    expect(retained.browserSummaryByTaskId[ERRAND_TASK_ID].errands).toEqual([outstanding]);
    expect(retained.historyByOwnerId.panel.pages[0].errands).toEqual([finished]);
    expect(retained.detailOwnersBySessionId[finished.errand_id]).toEqual({ child: true });

    const state = mergeInitialState({ errands: retained });
    expect(state.errands.byTaskId[ERRAND_TASK_ID]).toEqual({ [running.errand_id]: running });
    expect(state.errands.browserReadScopeIdentity).toBeUndefined();
    expect(state.errands.browserSummaryByTaskId).toEqual({});
    expect(state.errands.browserSummaryReadByTaskId).toEqual({});
    expect(state.errands.browserRevisionByTaskId).toEqual({});
    expect(state.errands.historyByOwnerId).toEqual({});
    expect(state.errands.detailBySessionId).toEqual({});
    expect(state.errands.detailOwnersBySessionId).toEqual({});
    expect(state.errands.detailObservationRevisionBySessionId).toBeUndefined();
  });
});

describe("prompt hydration authority (AC-UI-PINNED-PROMPT-AVAILABILITY-001.6)", () => {
  it("hydrates only rows and metadata, not observed or deleted prompt identities", () => {
    const state = mergeInitialState({
      messagePrompts: {
        bySession: { session: [] },
        metaBySession: {},
        authoritativeBySession: { session: true },
        observedBySession: {
          session: {
            ids: { old: true },
            newestKey: { id: "old", created_at: "2026-08-22T00:00:00Z" },
          },
        },
        deletedIdsBySession: { session: { old: true } },
      },
    } as unknown as HydrationState);

    expect(state.messagePrompts.bySession).toEqual({ session: [] });
    expect(state.messagePrompts.authoritativeBySession).toEqual({});
    expect(state.messagePrompts.observedBySession).toEqual({});
    expect(state.messagePrompts.deletedIdsBySession).toEqual({});
  });
});

describe("quick chat hydration state", () => {
  it("marks an empty boot snapshot ready for the active workspace", () => {
    const state = mergeInitialState({
      workspaces: { items: [], activeId: "workspace-1" },
      quickChat: { sessions: [] },
    } as unknown as HydrationState);

    expect(state.quickChat.selectionReadyByWorkspace).toEqual({ "workspace-1": true });
  });
});

describe("failed Inbox hydration state", () => {
  it("keeps the request revision map when older persisted state omits it", () => {
    const state = mergeInitialState({
      failedInbox: {
        byWorkspaceId: {},
        generationByWorkspaceId: {},
      },
    } as unknown as HydrationState);

    expect(state.failedInbox.readRevisionByWorkspaceId).toEqual({});
  });
});

describe("settings agent hydration", () => {
  it("restores selector creation order from boot Settings rows while preserving option metadata", () => {
    const state = mergeInitialState({
      settingsAgents: {
        items: [
          {
            id: "a",
            profiles: [
              { id: "old", created_at: "2026-01-01T00:00:00Z" },
              { id: "new", created_at: "2026-02-01T00:00:00Z" },
            ],
          },
        ],
      },
      agentProfiles: {
        items: [
          { id: "old", agent_id: "a", label: "Old", model: "old-model" },
          { id: "new", agent_id: "a", label: "New", model: "new-model" },
        ],
      },
    } as unknown as HydrationState);
    expect(state.agentProfiles.items.map((profile) => profile.id)).toEqual(["new", "old"]);
    expect(state.agentProfiles.items[0].model).toBe("new-model");
  });

  it("normalizes fallback fields from boot-hydrated profiles", () => {
    const state = mergeInitialState({
      settingsAgents: {
        items: [
          {
            profiles: [
              {
                id: "explicit-profile",
                fallback_model: "provider/model",
                auto_fallback: false,
              },
              {
                id: "automatic-profile",
                fallback_model: "",
                auto_fallback: true,
              },
            ],
          },
        ],
      },
    } as unknown as HydrationState);

    expect(state.settingsAgents.items[0]?.profiles).toMatchObject([
      {
        id: "explicit-profile",
        fallbackModel: "provider/model",
        autoFallback: false,
      },
      {
        id: "automatic-profile",
        fallbackModel: "",
        autoFallback: true,
      },
    ]);
  });
});
