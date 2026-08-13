import { describe, it, expect, vi, beforeEach } from "vitest";
import { create, type StoreApi } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "@/lib/state/slices/session-runtime/session-runtime-slice";
import type { SessionRuntimeSlice } from "@/lib/state/slices/session-runtime/types";
import type { AppState } from "@/lib/state/store";
import type {
  GitCommitsResetEvent,
  GitBranchSwitchedEvent,
  GitStatusUpdateEvent,
} from "@/lib/types/git-events";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";
import { registerGitStatusHandlers } from "./git-status";

// invalidateCumulativeDiffCache lives in a hook module that pulls React in via
// its imports. Stub it out so this test can run as a pure unit test against
// the slice + handler without dragging in React.
vi.mock("@/hooks/domains/session/use-cumulative-diff", () => ({
  invalidateCumulativeDiffCache: vi.fn(),
}));

const SESSION = "sess-1";
const STATUS_TIME_1 = "2026-05-28T00:00:01Z";
const STATUS_TIME_2 = "2026-05-28T00:00:02Z";
const STATUS_TIME_3 = "2026-05-28T00:00:03Z";
const FRONTEND_GENERATION = "frontend-generation";
const BACKEND_GENERATION = "backend-generation";
const FRONTEND_REPOSITORY = "frontend";
const BACKEND_REPOSITORY = "backend";
const MISSING_HANDLER_MESSAGE = "session.git.event handler is missing";
const invalidateCumulativeDiffCacheMock = vi.mocked(invalidateCumulativeDiffCache);

function makeStore() {
  // The handler only touches session-runtime state and environmentIdBySessionId.
  // We don't need the full AppState — cast through unknown so the handler
  // signature is satisfied without standing up unrelated slices.
  return create<SessionRuntimeSlice>()(
    immer((set, get, store) => createSessionRuntimeSlice(set, get, store)),
  ) as unknown as StoreApi<AppState>;
}

function gitEvent(payload: GitCommitsResetEvent | GitBranchSwitchedEvent | GitStatusUpdateEvent) {
  return {
    id: "msg",
    type: "notification" as const,
    action: "session.git.event" as const,
    timestamp: payload.timestamp,
    payload,
  };
}

function gitStatusHandler(store: StoreApi<AppState>) {
  const handler = registerGitStatusHandlers(store)["session.git.event"];
  if (!handler) throw new Error(MISSING_HANDLER_MESSAGE);
  return handler;
}

function statusUpdateEvent(timestamp: string, diff = "-old\n+new"): GitStatusUpdateEvent {
  return {
    type: "status_update",
    session_id: SESSION,
    timestamp,
    status: {
      branch: "main",
      remote_branch: null,
      modified: ["a.ts"],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      remote_ahead: 0,
      remote_behind: 0,
      files: {
        "a.ts": {
          path: "a.ts",
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 1,
          diff,
        },
      },
    },
  };
}

function repoStatusUpdateEvent(
  timestamp: string,
  repositoryName: string,
  modifiedPath: string,
): GitStatusUpdateEvent {
  return {
    type: "status_update",
    session_id: SESSION,
    timestamp,
    status: {
      branch: "main",
      remote_branch: null,
      modified: [modifiedPath],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      remote_ahead: 0,
      remote_behind: 0,
      repository_name: repositoryName,
      files: {
        [modifiedPath]: {
          path: modifiedPath,
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 0,
        },
      },
    },
  };
}

function seedSessionCommits(store: StoreApi<AppState>) {
  store.getState().setSessionCommits(SESSION, [
    {
      id: "id",
      session_id: SESSION,
      commit_sha: "old",
      parent_sha: "parent",
      commit_message: "msg",
      author_name: "a",
      author_email: "a@a",
      files_changed: 0,
      insertions: 0,
      deletions: 0,
      committed_at: "2026-05-28T00:00:00Z",
      created_at: "2026-05-28T00:00:00Z",
    },
  ]);
}

// eslint-disable-next-line max-lines-per-function -- handler lifecycle cases share one store fixture.
describe("git-status WS handler — stale-while-revalidate", () => {
  let store: StoreApi<AppState>;

  beforeEach(() => {
    invalidateCumulativeDiffCacheMock.mockClear();
    store = makeStore();
    seedSessionCommits(store);
  });

  it("commits_reset bumps refetchTrigger and keeps existing commits visible", () => {
    const handler = gitStatusHandler(store);

    handler(
      gitEvent({
        type: "commits_reset",
        session_id: SESSION,
        timestamp: "2026-05-28T00:00:01Z",
        reset: { previous_head: "old-head", current_head: "new-head", deleted_count: 1 },
      }),
    );

    const state = store.getState();
    // Trigger bumped — useSessionCommits will refetch.
    expect(state.sessionCommits.refetchTrigger[SESSION]).toBe(1);
    // Existing commits remain — this is the whole point. Clearing would make
    // the Changes panel briefly render its empty state until the refetch
    // resolved.
    expect(state.sessionCommits.byEnvironmentId[SESSION]).toHaveLength(1);
    expect(state.sessionCommits.byEnvironmentId[SESSION][0].commit_sha).toBe("old");
  });

  it("branch_switched bumps refetchTrigger and keeps existing commits visible", () => {
    const handler = gitStatusHandler(store);

    handler(
      gitEvent({
        type: "branch_switched",
        session_id: SESSION,
        timestamp: "2026-05-28T00:00:02Z",
        branch_switch: {
          previous_branch: "old",
          current_branch: "new",
          current_head: "head",
          base_commit: "base",
        },
      }),
    );

    const state = store.getState();
    expect(state.sessionCommits.refetchTrigger[SESSION]).toBe(1);
    expect(state.sessionCommits.byEnvironmentId[SESSION]).toHaveLength(1);
  });

  it("does not invalidate cumulative diff for duplicate status snapshots", () => {
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1)));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2)));

    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(1);
  });

  it("stores the status-level submodule marker", () => {
    const handler = gitStatusHandler(store);
    const event = statusUpdateEvent(STATUS_TIME_1);
    event.status.is_submodule = true;

    handler(gitEvent(event));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION].is_submodule).toBe(true);
  });

  it("retains the commit and upstream evidence from a status event", () => {
    const handler = gitStatusHandler(store);
    const event = statusUpdateEvent(STATUS_TIME_1);
    event.status = {
      ...event.status,
      head_commit: "head-sha",
      base_commit: "base-sha",
      remote_head_commit: "remote-head-sha",
      remote_ahead: 2,
      remote_behind: 3,
    } as typeof event.status;

    handler(gitEvent(event));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toMatchObject({
      head_commit: "head-sha",
      base_commit: "base-sha",
      remote_head_commit: "remote-head-sha",
      remote_ahead: 2,
      remote_behind: 3,
    });
  });

  it("retains role observations per repository without leaking sibling state", () => {
    const handler = gitStatusHandler(store);
    const frontend = repoStatusUpdateEvent(STATUS_TIME_1, FRONTEND_REPOSITORY, "frontend.tsx");
    frontend.status.remote_roles_generation = FRONTEND_GENERATION;
    frontend.status.action_head = {
      observation_state: "present",
      remote_head_commit: "frontend-action",
    };
    frontend.status.tracking_upstream = { observation_state: "absent" };
    handler(gitEvent(frontend));

    const backend = repoStatusUpdateEvent(STATUS_TIME_2, BACKEND_REPOSITORY, "backend.go");
    backend.status.remote_roles_generation = BACKEND_GENERATION;
    backend.status.action_head = {
      observation_state: "present",
      remote_head_commit: "backend-action",
    };
    backend.status.tracking_upstream = { observation_state: "present" };
    handler(gitEvent(backend));

    const frontendPartial = repoStatusUpdateEvent(
      STATUS_TIME_3,
      FRONTEND_REPOSITORY,
      "frontend-next.tsx",
    );
    handler(gitEvent(frontendPartial));

    const repoMap = store.getState().gitStatus.byEnvironmentRepo[SESSION];
    expect(repoMap[FRONTEND_REPOSITORY]).toMatchObject({
      remote_roles_generation: FRONTEND_GENERATION,
      action_head: frontend.status.action_head,
      tracking_upstream: frontend.status.tracking_upstream,
    });
    expect(repoMap[BACKEND_REPOSITORY]).toMatchObject({
      remote_roles_generation: BACKEND_GENERATION,
      action_head: backend.status.action_head,
      tracking_upstream: backend.status.tracking_upstream,
    });
  });

  it("invalidates cumulative diff when status diff content changes", () => {
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, "-old\n+new")));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2, "-old\n+newer")));

    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);
  });

  it("does not invalidate or overwrite env status for duplicate sibling-repo snapshots", () => {
    const handler = gitStatusHandler(store);

    handler(gitEvent(repoStatusUpdateEvent(STATUS_TIME_1, "frontend", "frontend.tsx")));
    handler(gitEvent(repoStatusUpdateEvent(STATUS_TIME_2, "backend", "backend.go")));
    const envAfterBackend = store.getState().gitStatus.byEnvironmentId[SESSION];
    invalidateCumulativeDiffCacheMock.mockClear();

    handler(gitEvent(repoStatusUpdateEvent(STATUS_TIME_3, "backend", "backend.go")));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toBe(envAfterBackend);
    expect(invalidateCumulativeDiffCacheMock).not.toHaveBeenCalled();
  });
});

describe("git-status WS handler comparison evidence", () => {
  let store: StoreApi<AppState>;

  beforeEach(() => {
    invalidateCumulativeDiffCacheMock.mockClear();
    store = makeStore();
    seedSessionCommits(store);
  });

  it("retains comparison evidence and invalidates when its resolution changes", () => {
    const handler = gitStatusHandler(store);
    const first = statusUpdateEvent(STATUS_TIME_1);
    first.status.comparison = {
      context_generation: "generation-1",
      resolution_state: "unresolved",
      reason: "comparison ref is not available locally",
      base_commit: "stored-base",
    };
    handler(gitEvent(first));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION].comparison).toEqual(
      first.status.comparison,
    );

    const second = statusUpdateEvent(STATUS_TIME_2);
    second.status.comparison = {
      ...first.status.comparison,
      resolution_state: "resolved",
      resolved_ref: "canonical-remote/main",
    };
    handler(gitEvent(second));
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);

    const partial = statusUpdateEvent(STATUS_TIME_3);
    handler(gitEvent(partial));
    expect(store.getState().gitStatus.byEnvironmentId[SESSION].comparison).toEqual(
      second.status.comparison,
    );
  });

  it("retains omitted role observations but replaces an explicit atomic observation", () => {
    const handler = gitStatusHandler(store);
    const first = statusUpdateEvent(STATUS_TIME_1);
    first.status.remote_roles_generation = "generation-1";
    first.status.action_head = {
      observation_state: "present",
      remote_head_commit: "action-head",
      ahead: 2,
      behind: 1,
    };
    first.status.tracking_upstream = { observation_state: "unknown" };
    handler(gitEvent(first));

    const partial = statusUpdateEvent(STATUS_TIME_2);
    handler(gitEvent(partial));
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toMatchObject({
      remote_roles_generation: "generation-1",
      action_head: first.status.action_head,
      tracking_upstream: first.status.tracking_upstream,
    });

    const cleared = statusUpdateEvent(STATUS_TIME_3);
    cleared.status.action_head = { observation_state: "absent" };
    handler(gitEvent(cleared));
    expect(store.getState().gitStatus.byEnvironmentId[SESSION].action_head).toEqual(
      cleared.status.action_head,
    );
  });
});
