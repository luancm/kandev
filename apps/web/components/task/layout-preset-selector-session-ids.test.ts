import { describe, expect, it, vi } from "vitest";
import { resolveLayoutApplySessionIds } from "./layout-preset-selector-session-ids";

const ACTIVE_SESSION_ID = "active-session";
const SIBLING_SESSION_ID = "sibling-session";

describe("resolveLayoutApplySessionIds", () => {
  it("loads task sessions before reading ids when the cache is not loaded", async () => {
    let sessions = [{ id: ACTIVE_SESSION_ID }];
    let sessionsLoaded = false;
    const loadSessions = vi.fn(async () => {
      sessions = [{ id: ACTIVE_SESSION_ID }, { id: SIBLING_SESSION_ID }];
      sessionsLoaded = true;
    });

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      sessionsLoaded: false,
      loadSessions,
      getSessionsForTask: () => sessions,
      getSessionsLoadedForTask: () => sessionsLoaded,
    });

    expect(loadSessions).toHaveBeenCalledWith(true);
    expect(result).toEqual({ sessionIds: [ACTIVE_SESSION_ID, SIBLING_SESSION_ID], loaded: true });
  });

  it("does not inject an active session that is absent from ordinary membership", async () => {
    const loadSessions = vi.fn();

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      sessionsLoaded: true,
      loadSessions,
      getSessionsForTask: () => [{ id: SIBLING_SESSION_ID }],
      getSessionsLoadedForTask: () => true,
    });

    expect(loadSessions).not.toHaveBeenCalled();
    expect(result).toEqual({ sessionIds: [SIBLING_SESSION_ID], loaded: true });
  });

  it("holds session identity when a forced list read remains unresolved", async () => {
    const loadSessions = vi.fn(async () => undefined);

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      sessionsLoaded: false,
      loadSessions,
      getSessionsForTask: () => [{ id: ACTIVE_SESSION_ID }],
      getSessionsLoadedForTask: () => false,
    });

    expect(result).toEqual({ sessionIds: [], loaded: false });
  });

  it("does not duplicate the active session when it is already loaded", async () => {
    const loadSessions = vi.fn();

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      sessionsLoaded: true,
      loadSessions,
      getSessionsForTask: () => [{ id: ACTIVE_SESSION_ID }, { id: SIBLING_SESSION_ID }],
      getSessionsLoadedForTask: () => true,
    });

    expect(loadSessions).not.toHaveBeenCalled();
    expect(result).toEqual({ sessionIds: [ACTIVE_SESSION_ID, SIBLING_SESSION_ID], loaded: true });
  });
});
