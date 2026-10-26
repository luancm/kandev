import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { pickReplacementSessionId, registerTaskSessionHandlers } from "./agent-session";

const STATE_CHANGED = "session.state_changed";
const TASK_ID = "t-1";
const ERRAND_ID = "s-errand";

type StoreOptions = {
  errandsEnabled: boolean;
  activeSessionId?: string | null;
  sessions?: Array<Record<string, unknown>>;
  errandRows?: Record<string, unknown>;
};

function makeStore({
  errandsEnabled,
  activeSessionId = null,
  sessions = [],
  errandRows = {},
}: StoreOptions) {
  const items = Object.fromEntries(sessions.map((session) => [session.id as string, session]));
  const state: Record<string, unknown> = {
    features: { errands: errandsEnabled },
    errands: { byTaskId: { [TASK_ID]: errandRows }, loadedByTaskId: {}, liveRevisionByTaskId: {} },
    tasks: {
      activeTaskId: TASK_ID,
      activeSessionId,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
      resumeSkippedSessionIds: {},
    },
    taskSessions: { items },
    taskSessionsByTask: { itemsByTaskId: { [TASK_ID]: sessions } },
    upsertTaskSessionFromEvent: vi.fn(),
    setActiveSession: vi.fn(),
    setActiveSessionAuto: vi.fn(),
    setSessionAgentctlStatus: vi.fn(),
    invalidateConfirmedConfigOptions: vi.fn(),
    setResumeSkipped: vi.fn(),
    clearLaunchWarning: vi.fn(),
    setSessionFailureNotification: vi.fn(),
    setContextWindow: vi.fn(),
    clearContextWindow: vi.fn(),
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function sendStateChanged(store: StoreApi<AppState>, payload: Record<string, unknown>) {
  registerTaskSessionHandlers(store)[STATE_CHANGED]!({
    id: "m",
    type: "notification",
    action: STATE_CHANGED,
    payload: { task_id: TASK_ID, ...payload } as never,
  });
}

const terminalActive = { id: "s-old", task_id: TASK_ID, state: "COMPLETED", started_at: "" };

beforeEach(() => vi.clearAllMocks());

describe("errand sessions are never auto-adopted while the errands flag is on", () => {
  it("does not adopt a new errand session announced with its role", () => {
    const store = makeStore({ errandsEnabled: true });

    sendStateChanged(store, {
      session_id: ERRAND_ID,
      new_state: "STARTING",
      role: "errand",
      parent_session_id: "s-parent",
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });

  it("does not adopt a new session already known as an errand row", () => {
    const store = makeStore({
      errandsEnabled: true,
      activeSessionId: "s-old",
      sessions: [terminalActive],
      errandRows: { [ERRAND_ID]: { errand_id: ERRAND_ID } },
    });

    sendStateChanged(store, { session_id: ERRAND_ID, new_state: "STARTING" });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does not hand off from a terminal session to a running errand", () => {
    const old = { id: "s-old", task_id: TASK_ID, state: "RUNNING", started_at: "" };
    const errand = { id: ERRAND_ID, task_id: TASK_ID, state: "RUNNING", role: "errand" };
    const store = makeStore({
      errandsEnabled: true,
      activeSessionId: old.id,
      sessions: [old, errand],
    });

    sendStateChanged(store, { session_id: old.id, new_state: "COMPLETED" });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("keeps an opened errand selected when it reaches a terminal state", () => {
    const errand = { id: ERRAND_ID, task_id: TASK_ID, state: "RUNNING", role: "errand" };
    const peer = { id: "s-peer", task_id: TASK_ID, state: "RUNNING", started_at: "" };
    const store = makeStore({
      errandsEnabled: true,
      activeSessionId: ERRAND_ID,
      sessions: [peer, errand],
    });

    sendStateChanged(store, { session_id: ERRAND_ID, new_state: "CANCELLED", role: "errand" });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("still adopts an ordinary new session", () => {
    const store = makeStore({ errandsEnabled: true });

    sendStateChanged(store, { session_id: "s-new", new_state: "STARTING" });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, "s-new");
  });
});

describe("errand sessions keep ordinary adoption while the errands flag is off", () => {
  it("adopts a new errand session like any session", () => {
    const store = makeStore({ errandsEnabled: false });

    sendStateChanged(store, { session_id: ERRAND_ID, new_state: "STARTING", role: "errand" });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, ERRAND_ID);
  });

  it("hands off to a running errand like any session", () => {
    const old = { id: "s-old", task_id: TASK_ID, state: "RUNNING", started_at: "" };
    const errand = { id: ERRAND_ID, task_id: TASK_ID, state: "RUNNING", role: "errand" };
    const store = makeStore({
      errandsEnabled: false,
      activeSessionId: old.id,
      sessions: [old, errand],
    });

    sendStateChanged(store, { session_id: old.id, new_state: "COMPLETED" });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, ERRAND_ID);
  });
});

describe("pickReplacementSessionId", () => {
  const ordinary = { id: "s-ordinary", task_id: TASK_ID, state: "RUNNING" };
  const errand = { id: ERRAND_ID, task_id: TASK_ID, state: "RUNNING", role: "errand" };

  it("skips errands while the flag is on", () => {
    const store = makeStore({ errandsEnabled: true, sessions: [ordinary, errand] });
    expect(pickReplacementSessionId(store.getState(), TASK_ID)).toBe("s-ordinary");
  });

  it("returns null when only errands remain", () => {
    const store = makeStore({ errandsEnabled: true, sessions: [errand] });
    expect(pickReplacementSessionId(store.getState(), TASK_ID)).toBeNull();
  });

  it("considers errands while the flag is off", () => {
    const store = makeStore({ errandsEnabled: false, sessions: [ordinary, errand] });
    expect(pickReplacementSessionId(store.getState(), TASK_ID)).toBe(ERRAND_ID);
  });
});
