---
id: "03-frontend-data-layer"
title: "Add the errands frontend data layer and tab filter"
status: pending
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

Pending.
