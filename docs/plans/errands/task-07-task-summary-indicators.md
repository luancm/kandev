---
id: "07-task-summary-indicators"
title: "Show errand counts and waiting status on task rows"
status: pending
wave: 5
depends_on:
  - "03-frontend-data-layer"
  - "05-errand-results-and-delivery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-010
acceptance_criteria:
  - AC-TASKS-ERRANDS-006.3
  - AC-TASKS-ERRANDS-010.5
  - AC-TASKS-ERRANDS-010.6
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 07: Show errand counts and waiting status on task rows

## Summary

Add the bounded `ErrandsSummary` to `TaskStatusSummary` and render a live
errand count and a waiting-on-errands status on board cards, sidebar rows,
and Threads rows. Nest errands under their parent in the Threads session
switcher. Rows of tasks without errands stay byte-identical.

## In scope

- `statussummary.TaskStatusSummary.Errands` (`live`, `waiting`), omitted when
  both are zero; projector rebuild and event inputs from errand changes and
  parent waiting changes; `task.status_summary.updated` publication.
- Frontend summary types and merge, kanban card status and indicator
  components, sidebar task row metadata, Threads row, and
  `thread-session-switcher.tsx` nesting, all gated by `useFeature("errands")`.
- Copy in `kanban.json`, `threads.json`, and `sidebar.json` for all seven
  shipped locales, using `count` plurals; `pnpm run i18n:zh-hant` for the
  Traditional Chinese pair.

## Out of scope

- Errand transcript cards, header, chip, and panel (task 08).

## Acceptance

1. The summary reports live errands and waiting state from durable rows, is
   omitted when zero, and updates through `task.status_summary.updated`.
2. Board, sidebar, and Threads rows show the count and waiting status only
   when present, and the switcher nests errands under their parent.

## ASCII UI preview

Excerpt of [UI-05](plan.md#ascii-ui-preview) (AC-TASKS-ERRANDS-010.5, 006.3).

```text
+----------------------------------+
| Add auth to API                  |
| [2 errands]   Waiting on errands |  <- absent when the task has none
+----------------------------------+

Threads session switcher:
  Main (Claude)        *
    API audit      Running
    Schema review  Done
```

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./internal/task/statussummary -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'StatusSummary|Errand' -count=1)
(cd apps/web && pnpm exec vitest run components/kanban-card-status-icon.test.tsx components/kanban-card-errands.test.tsx components/threads/thread-session-switcher.test.tsx components/threads/thread-errands-indicator.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/kanban-card-status-icon.tsx components/kanban-card-errands.tsx components/threads/thread-session-switcher.tsx components/threads/thread-column.tsx components/threads/thread-errands-indicator.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/statussummary/model.go`, `projector.go`,
  `projector_events.go`, `projector_derive.go`, `rebuild.go`, and tests.
- `apps/backend/internal/backendapp/status_summary_adapter.go`.
- `apps/web/components/kanban-card-status-icon.tsx`, a new
  `kanban-card-errands.tsx`, `apps/web/components/kanban-card-types.ts`,
  `apps/web/lib/kanban/map-task.ts`.
- `apps/web/components/threads/thread-column.tsx`,
  `thread-session-switcher.tsx`, a new `thread-errands-indicator.tsx`.
- `apps/web/src/locales/*/kanban.json`, `threads.json`, `sidebar.json`.

## Dependencies

Tasks 03 and 05.

## Risks

- The summary is a bounded, persisted projection with its own revision; new
  inputs must keep revision ordering and must not carry unbounded data.
- `activeSubagentCount` shares the card's indicator area; keep the two counts
  distinct in copy and accessible names.

## Parallelism

`parallel-safe` with task 06.

## Inputs

- [Bounded task-status delivery](../../specs/platform/requirements/bounded-task-status-delivery.md)
  and [ADR-2026-08-01-separate-task-summary-session-stream-traffic](../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md).
- `/mobile-parity` for the Threads phone row.

## Results

Pending.
