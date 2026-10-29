---
id: "07-task-summary-indicators"
title: "Show errand counts and waiting status on task rows"
status: in_progress
wave: 12
depends_on:
  - "03-frontend-data-layer"
  - "05-errand-results-and-delivery"
  - "12-lifecycle-convergence"
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

Finish the frontend half of the bounded `ErrandsSummary` projection already
implemented at `95031bfc6a`. Render live counts and waiting state on board,
sidebar, desktop Threads and phone Threads, and nest errand sessions under
their parent without adding row-level transcript/session fetches.

## In scope

- Add `errands` to `lib/types/task-status-summary.ts`, preserve authoritative
  summary revisions through existing merges, and pass it through kanban,
  sidebar and Threads view models. Keep the backend `live`/`waiting`
  projection; do not reimplement its counts in each row.
- Cover the actual sidebar metadata path (`task-session-sidebar-item.ts`,
  `task-item-stats-row.tsx`) and Threads projection (`active-threads.ts`),
  desktop `ThreadMeta`, phone header/picker, and session switcher. Do not
  change `hasActiveSubagents` or native-subagent filter meaning.
- Gate presentation with `useFeature("errands")`. No rows/controls for zero,
  absent summaries or flag off; waiting can be visible after the live count
  reaches zero while a result is still outstanding. Retain the existing
  task-load error path rather than inventing count data on error.
- Copy in `kanban.json`, `threads.json`, and `sidebar.json` for all seven
  shipped locales, using `count` plurals and `pnpm run i18n:zh-hant` for
  Traditional Chinese. Keep pseudo coverage and unrelated translations.

## Out of scope

- Errand transcript cards, header, chip, and panel (task 08).

## Acceptance

1. Existing durable counts survive summary/revision merges, update through
   `task.status_summary.updated`, and omit the errand decoration at zero/off;
   each surface consumes the bounded summary without a per-row fetch.
2. Board, sidebar, and desktop/phone Threads show count and waiting state
   from the same source, the switcher nests errands beneath parents, and
   native subagent counts/filters keep their meaning.
3. Copy is localized, controls are keyboard/touch reachable, and phone
   navigation keeps the existing focused layout without horizontal overflow.

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

### Phone composition and states

UI-05 shares the board/sidebar indicator content across viewports. Phone
Threads uses its existing column header and picker, with errand sessions
nested beneath the parent in the existing bottom picker. Tapping a child
opens the full session view; back returns to the parent. Keep one scrolling
list with fixed context/actions, safe-area clearance and 44px action targets.
Nearest exemplars are `mobile-thread-picker.tsx`,
`mobile-thread-column-header.tsx`, and `session-task-switcher-sheet.tsx`.
Desktop retains compact inline counts and its existing switcher.

Test present/zero/waiting-only/disabled/long-localized-label states. Loading
and error handling remain owned by the existing task load. This is a content
and hierarchy extension of the current navigation, not a new panel or filter.
Task 09 supplies the UI-05 rendered desktop/phone proof.

## Verification

Run from root after task 00 installs dependencies. New indicator and phone test files
below are planned additions; extend the existing switcher and projection
suites with TDD and verify each selector discovers the intended cases.

```bash
(cd apps/backend && go test -trimpath ./internal/task/statussummary -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'StatusSummary|Errand' -count=1)
(cd apps/web && pnpm exec vitest run components/kanban-card-status-icon.test.tsx components/kanban-card-actions.processing.test.tsx components/kanban-card-errands.test.tsx components/task/task-session-sidebar-item.test.ts components/task/task-item-stats-row.test.tsx components/task/task-item.test.tsx components/threads/thread-session-switcher.test.tsx components/threads/thread-errands-indicator.test.tsx components/threads/mobile-thread-column-header.test.tsx components/threads/mobile-thread-picker.test.tsx lib/threads/active-threads.test.ts lib/kanban/map-task.test.ts lib/types/task-status-summary.test.ts lib/task-status-summary.test.ts lib/ws/handlers/tasks-status-summary.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Run `pnpm exec eslint --max-warnings 0` from `apps/web` with the exact changed
TypeScript paths below. Review converter output and retain only intended new
Traditional Chinese keys. There is no planned backend logic change; any
projection failure belongs to task 12 rather than a second implementation.

## Files likely touched

- `apps/web/lib/types/task-status-summary.ts`, summary merge tests,
  `lib/kanban/map-task.ts`, `components/kanban-card-types.ts`,
  `components/kanban-card-status-icon.tsx`, `components/kanban-card-actions.tsx`,
  and new `kanban-card-errands.tsx`.
- `apps/web/components/task/{task-session-sidebar-item.ts,task-item.tsx,task-item-stats-row.tsx}`.
- `apps/web/lib/threads/active-threads.ts`,
  `components/threads/{thread-column.tsx,thread-session-switcher.tsx,mobile-thread-column-header.tsx,mobile-thread-picker.tsx}`,
  and a shared `thread-errands-indicator.tsx` if needed.
- Focused tests beside these surfaces and
  `apps/web/src/locales/*/{kanban,threads,sidebar}.json`.

## Dependencies

Historical tasks 03 and 05, then task 12 for corrected live waiting state.
Task 09 proves this completed slice in the browser.

## Risks

- The summary is a bounded, persisted projection with its own revision; new
  inputs must keep revision ordering and must not carry unbounded data.
- `activeSubagentCount` shares the card's indicator area; keep the two counts
  distinct in copy and accessible names.

## Parallelism

`sequential` in the continuation; the original parallel pairing with task 06
is complete.

## Inputs

- [Bounded task-status delivery](../../specs/platform/requirements/bounded-task-status-delivery.md)
  and [ADR-2026-08-01-separate-task-summary-session-stream-traffic](../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md).
- `/mobile-parity` for the Threads phone row.

## Results

The backend portion was implemented in `95031bfc6a`: `ErrandsSummary`, durable
projection/rebuild/event refresh, backendapp adapter, and focused tests. This
remains historical source evidence. Packet 07A now adds the optional bounded
summary type and tests revision-ordered update/partial-merge behavior through
the existing projection path. The Kanban card and task sidebar consume that
summary in their existing metadata slots, gated by the Errands feature flag;
zero or absent summaries stay hidden, waiting-only state remains visible, and
native subagent counts retain their separate meaning. No per-row fetch was
added.

Fresh verification passed: backend `statussummary` and `backendapp` Errand
tests; the focused web suite (10 files, 171 tests) plus the task-overview merge
suite (1 file, 7 tests); web typecheck; exact changed-file ESLint; Traditional
Chinese conversion; i18n check and ratchet; and focused tracked/untracked
whitespace checks. Command and exit receipts are in
`.kandev/diagnostics/errands-07a-20261028T1321Z/receipts.md`.

This completes only 07A. Threads/navigation indicators and nesting in 07B
remain outstanding, and task 07 stays `in_progress`. Task 09 still owns
rendered desktop/phone proof; task 13 remains excluded from this packet.
PostgreSQL skips remain an explicit readiness gate, and no broader support or
provider-pilot claim is made.
