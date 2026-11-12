---
created: 2026-11-12
status: done
requirements:
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-status-and-continuation.md
legacy_specs: []
---

# Errand panel status and individual controls

## Outcome and evidence

Show truthful parent activity counts and offer direct individual Stop in the Errands panel. Tasks owns this outcome because it owns errand lifecycle and cancellation; UI is a consumer of that contract. `ErrandsChipBody` counts `isLiveErrand`, which includes queued and attention, then always uses `errandsChipRunning`. `PopoverRow` offers only Open; `DrawerRow` is one navigation button. Individual cancellation already exists in the child header and `useErrandActions`. The screenshot and this static source trace establish the presentation defect; no live instance or database was mutated and no rendered reproduction was run.

## Scope and assumptions

Confirmed: distinguish recovery attention from running and expose individual stop in the panel. Autonomous conservative interpretation: delete/stop means existing cancellation with transcript retention. Permanent deletion, result removal, backend recovery repair, capacity changes and other summary surfaces are excluded. Existing ADR settlement authority remains unchanged; stopping does not prove external effects. No material unknown blocks this reversible design. Companion orphan-settlement and conversation-navigation packages retain their backend/navigation ownership; this package adds only chip presentation and row controls.

## Delivery and ownership

The user approved the design and implementation. Frontend and backend workers released source ownership; the parent completed review remediation and final integration. Unit/hook coverage passed 34 tests, backend race checks passed 77, and fresh managed browser acceptance passed 19 desktop and 16 phone cases against the combined source. Public coordination guidance is updated. Heavy checks ran serially under the shared lock. See [final integration evidence](final-integration-handoff.md) for receipts, exact source hashes, rendered inspection and limits; parent owns commit and push.

## Technical approach

Use the selected-parent inventory for activity counts, preserving task-wide Stop-all. Reuse `cancelErrand`, pending state, localized error handling and authoritative events. Refactor the phone row into sibling navigation and cancellation controls; use the shipped Errands drawer and mobile picker sheet as the composition exemplar. Preserve internal scrolling, safe-area clearance, Back navigation and focus behavior. No new transport or persistence boundary.

## ASCII UI preview

UI-01: Parent status bar and desktop popover, recovery plus running plus queued work.

```text
Before: Errands: 3 running
After:  Errands: 1 needs attention, 1 running, 1 queued
+--------------------------------------------+
| Errands                           106 total|
| Review browser acceptance                  |
| Recovery needs verification                |
| Earlier work and effects remain unknown.   |
|                              [Open] [Stop] |
| Completed history (103) >                  |
|             [Stop all errands]             |
+--------------------------------------------+
```

UI-02: Phone bottom drawer opened from the same parent chip.

```text
+----------------------------------+
| Errands                     Close|
| [Review acceptance           >]  |
| Recovery needs verification      |
| Earlier effects remain unknown.  |
| [Open conversation]    [Stop]    |
| Completed history (103) >        |
|----------------------------------|
|        Stop all errands          |
+----------------------------------+
```

The summary and Open may share one navigation button; Stop is always a sibling control. UI-02 illustrates hierarchy, not duplicate required Open buttons. List content scrolls inside the existing drawer; the Stop-all footer is fixed. Narrow layouts can wrap count fragments without dropping information. Every phone action is at least 44px; desktop actions retain 28px density. Pending Stop shows progress and is disabled only for that target; failure leaves the row and warning visible with a localized toast. Settled rows retain Open and omit Stop. These structures satisfy AC-TASKS-ERRANDS-010.9, 010.12 and 011.8; spacing and wording are illustrative and must use locale keys.

## Tests and E2E

`errands-chip.test.tsx` covers AC-010.9 with status-only, mixed and parent-scoped inventories, including recovery and reasonless permission attention. New `errands-panel-row.test.tsx` covers AC-010.12 and 007.1 with selected-target cancellation, queued/recovery eligibility, duplicate click, pending/error handling and settled-row exclusion. Extend `errands.spec.ts` and `mobile-errands.spec.ts` for actual row cancellation, sibling isolation, authoritative count refresh, retained transcript and unchanged Open/Back. Phone coverage measures action hitboxes, viewport containment, scroll ownership and no horizontal overflow, including a narrow fine-pointer viewport. Locale checks cover every shipped language; use the Traditional Chinese converter.

## Work orders

- [x] [Task 01: Truthful counts and individual Stop](task-01-panel-controls.md) (complete; combined backend/desktop/phone acceptance passed)

## Verification commands

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/errands/errands-chip.test.tsx components/task/chat/errands/errands-panel-row.test.tsx hooks/domains/session/use-errand-actions.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
make build-web
make build-backend
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/errands.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-errands.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

All implementation and review findings are closed for this package. Final backend race checks passed 77 tests; Go lint passed with zero issues. The focused frontend suite passed 34/34, with typecheck, lint, locale checks and production build receipts in the work order. Final fresh managed acceptance passed 19/19 desktop and 16/16 phone cases, including mixed-count wrapping, target geometry, isolated cancellation, retained transcript, authoritative count refresh, ordering and Open/Back. The parent inspected desktop and 320px phone screenshots. Catalog/spec lint and formatting passed. The original phone failures were test synchronization/locator defects and now pass; their receipts remain historical failure evidence. See [final integration handoff](final-integration-handoff.md).

## Risks

Long localized mixed-count labels must wrap within the chip container. Phone button refactoring can accidentally nest actions or navigate on Stop. Existing backend cancellation can fail; preserve the row and show failure instead of falsely settling it. Existing unrelated untracked loading-performance notes are preserved.
