---
id: "03-frontend-data-layer"
title: "Add the errands frontend data layer and tab filter"
status: done
wave: 2
depends_on:
  - "01-flag-and-persistence"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 03: Add the errands frontend data layer and tab filter

## Summary

Give the web app a typed errand model, an `errands` store slice with
selectors, WebSocket handling for `errand.updated`, session fields for role
and parent, hooks for listing and acting on errands, and the automatic tab
filter. No new visible surface ships here except that errand sessions stop
appearing as automatic tabs while the flag is on.

## In scope

- `TaskSession` in `lib/types/http.ts` gains `role`, `parent_session_id`, and
  `waiting_on_errands`; new `lib/types/errands.ts` mirrors `ErrandDTO`.
- New `lib/state/slices/errands/` with `byTaskId[taskId][errandId]`, upsert
  and remove actions, and selectors: task errands, errands by parent, live
  count, errand by session ID. Wire it into the root store.
- `lib/ws/handlers/errands.ts` for `errand.updated`; `upsertTaskSessionFromEvent`
  in `lib/ws/handlers/agent-session.ts` keeps the new session fields.
- `hooks/domains/session/use-task-errands.ts` (loads `errand.list` once per
  task while `useFeature("errands")` is on) and `use-errand-actions.ts`
  (`errand.cancel`, `errand.cancel_all`, `errand.send_result`), with
  deduplicated subscriptions.
- `runAutoSessionTabEffect` and `useAutoSessionTab` skip `role === "errand"`
  while the flag is on, and keep current behavior when it is off.

## Out of scope

- Rendered errand components and copy (task 08) and row indicators (task 07).

## Acceptance

1. Slice, selector, and WS handler tests cover upsert ordering, removal on
   task deletion, grouping by parent, and live counts.
2. With the flag on, an errand session never becomes an automatic tab; with
   the flag off, it behaves like any other session tab.

## ASCII UI preview

Excerpt of [UI-01](plan.md#ascii-ui-preview): the tab strip after errands
start (AC-TASKS-ERRANDS-010.2).

```text
| [* Main (Claude)] [+]          <- flag on: no errand tabs added
| [* Main (Claude)] [API audit]  <- flag off: errand shows as a normal tab
```

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/errands lib/ws/handlers/errands.test.ts lib/ws/handlers/agent-session.test.ts hooks/domains/session/use-task-errands.test.ts hooks/domains/session/use-errand-actions.test.ts components/task/dockview-session-tabs.test.ts components/task/dockview-session-tabs.hook.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/types/errands.ts lib/state/slices/errands lib/ws/handlers/errands.ts lib/ws/handlers/agent-session.ts hooks/domains/session/use-task-errands.ts hooks/domains/session/use-errand-actions.ts components/task/dockview-session-tabs.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/web/lib/types/http.ts`, new `apps/web/lib/types/errands.ts`.
- New `apps/web/lib/state/slices/errands/` and the root store composition.
- New `apps/web/lib/ws/handlers/errands.ts`; `agent-session.ts` and the
  handler registry.
- New `apps/web/hooks/domains/session/use-task-errands.ts` and
  `use-errand-actions.ts`.
- `apps/web/components/task/dockview-session-tabs.ts` and its tests.

## Dependencies

Task 01 for the DTO field names and the `errands` feature key. Backend
events arrive in task 04; tests use the documented contract.

## Risks

- State in `lib/state/` must not import from `components/`; keep the slice
  dependency-neutral.
- Dockview layout restore can reintroduce a saved errand tab; the filter must
  apply to automatic insertion only, not to tabs the user opened.

## Parallelism

`parallel-safe` with task 02.

## Inputs

- [System design frontend section](../../specs/tasks/system-design/errands.md#frontend).
- `apps/web/AGENTS.md` data flow, store, and WebSocket rules.

## Results

Completed on 2026-10-26 on branch `feat/errands-web`, test-first. No
delegation. No user-facing copy was added.

- Types: `lib/types/errands.ts` mirrors `ErrandDTO` as `Errand` (with
  `ErrandStatus`, `ErrandResultState`, `TaskSessionRole`,
  `ErrandListResponse`, `ErrandCancelAllResponse`, and
  `ErrandBackendMessageMap`, which joins `BackendMessageMap`) and adds
  `isLiveErrand`. `TaskSession` and `TaskSessionStateChangedPayload` gain
  optional `role`, `parent_session_id`, and `waiting_on_errands`; a missing
  role means an ordinary session.
- Slice: `lib/state/slices/errands` holds `errands.byTaskId`,
  `loadedByTaskId`, and `liveRevisionByTaskId`, wired into the root store,
  default state, and `AppState`. `upsertErrand` ignores a row whose
  `updated_at` is older than the stored one. `setTaskErrands` replaces the
  task's rows unless live updates landed while the request was in flight, in
  which case it merges, and never rolls back a newer row. `removeTaskErrands`
  runs from the `task.deleted` handler.
- Selectors (`selectors.ts`, reference-stable for `useAppStore`):
  `selectTaskErrands` (creation order), `selectErrandsByParent`,
  `selectParentErrands`, `selectLiveErrandCount`, `selectErrandBySessionId`,
  and `isErrandSession` (role, or a known errand row while the role is
  still missing from a partial session row).
- Transport: `lib/ws/handlers/errands.ts` applies `errand.updated` and is
  registered in `router.ts`. `role`, `parent_session_id`, and
  `waiting_on_errands` join `CARRIED_WHEN_DEFINED` in `agent-session.ts`, so
  `upsertTaskSessionFromEvent` keeps them and an event without them never
  clears them. `lib/api/domains/errand-api.ts` sends `errand.list`
  `{task_id}`, `errand.cancel` `{session_id}`, `errand.cancel_all`
  `{task_id}`, and `errand.send_result` `{session_id}` through the
  authenticated WebSocket client.
- Hooks: `useTaskErrands(taskId)` returns `{ errands, errandsByParent,
  liveCount, isLoaded }`; while the flag is on it loads `errand.list` once
  per task and connection, shared by every consumer, and refreshes after a
  reconnect. While the flag is off it returns nothing and sends nothing.
  `useErrandActions()` returns `cancelErrand(sessionId)`,
  `cancelAllErrands(taskId)` (resolves with the count),
  `sendErrandResult(sessionId)`, and `isPending(action, targetId)`;
  concurrent identical requests share one request.
- Tabs: with the flag on, `runAutoSessionTabEffect` leaves errand sessions
  out of automatic sibling insertion and out of the hidden-session fallback
  selection. Reconciliation still uses every session, so an errand tab the
  user opened, the active errand session, or a restored layout's errand tab
  stays. `useAutoSessionTab` re-runs when the flag changes. With the flag
  off, errands behave like any session tab.

Deviations: the WebSocket requests live in a new `errand-api.ts`, following
the `lib/api/domains` pattern. The errand tab-effect tests are in
`dockview-session-tabs.hook.test.tsx` with a shared store stand-in in
`dockview-session-tabs.test-utils.ts`, because `dockview-session-tabs.test.ts`
was already at 570 of its 600-line limit. The `task.deleted` per-session
cleanup moved into `evictDeletedTaskSessions` to stay under the complexity
limit.

Verification passed:

- The work order's Vitest command: 8 files, 160 tests. Also
  `lib/types/errands.test.ts`, `lib/api/domains/errand-api.test.ts`,
  `lib/ws/handlers/tasks.deleted.test.ts`, and the broader `lib/ws`,
  `hooks/domains/session`, `components/task/dockview*`, and `lib/types`
  suites (214 files, 2075 tests).
- The work order's ESLint command, and `eslint --max-warnings 0` plus
  `prettier --check` on every changed file.
- `pnpm run typecheck` (8 GiB heap), `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, and `git diff --check`.

`lib/state` also shows two failures (`comments/persistence.test.ts` and
`message-favorites/favorites-store.test.ts`) that reproduce identically on
the base commit.

Open contract points for tasks 04, 05, and 08:

- No event removes an errand row. When an errand session is deleted, live
  clients keep the row until the next `errand.list` load.
- `ErrandDTO` cannot express **Send result to parent** eligibility: the rule
  depends on `latest_settled_turn_id` and `result_turn_id`, which are not
  projected. A user follow-up after a delivered result is not detectable.
- `ErrandDTO` carries `agent_profile_id` only. Task 08 resolves the profile
  name and model from the agent profile store.
- `maybeAdoptSessionOnTransition` in `agent-session.ts` can still adopt a new
  errand as the active session (when the active session is missing, idle, or
  terminal) or pick it as a replacement, which then shows its tab.
- A partial session row without `role`, such as one from
  `session.agentctl_starting`, that arrives before both the role-bearing
  event and `errand.updated` can still become an automatic tab once.
  Publishing `errand.updated` before the session's first event avoids this.
- Responses of `errand.cancel` and `errand.send_result` are unspecified; the
  hooks ignore them, since state arrives through `errand.updated`.
