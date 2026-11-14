---
id: "03-demand-ui"
title: "Switch browser Errands to demand history and targeted inspection"
status: done
wave: 4
depends_on:
  - "02-ordinary-isolation"
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.2
  - AC-TASKS-ERRAND-HISTORY-001.3
  - AC-TASKS-ERRAND-HISTORY-001.4
  - AC-TASKS-ERRAND-HISTORY-001.6
  - AC-TASKS-ERRAND-HISTORY-001.7
  - AC-TASKS-ERRAND-HISTORY-001.8
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Switch browser Errands to demand history and targeted inspection

## Scope, implementation acceptance and exclusions

Own `apps/web/hooks/domains/session/use-task-errands.ts`, `lib/state/slices/errands/`, `lib/ws/handlers/errands.ts`, parent/preview/Threads consumer migrations, `components/task/chat/errands/errands-panel.tsx`, integration with task02 targeted routing, bounded child-detail cache ownership and locale catalogs. Use new typed APIs; never reinterpret `Errand[]` as complete history. Summary totals drive capacity/count/Stop-all display; cancel-all remains server-owned. History gets no mount/reconnect prefetch. Enforce one task/parent history cache per visible surface, four pages/100 summaries, explicit range/Start again, close/scope eviction, changed-chain Refresh, and one selected child detail owner plus existing retained parent.

Test shared read cancellation, duplicate coalescing, row/session-only revisions, tombstones, stale task/parent/cursor/auth/connection responses, continuations and terminal-pending transitions. Preserve task02 direct-URL inspection of one child/minimal parent before routing, without normal-tab fallback; adapt new history rows to that existing targeted path. Missing parent, queued no-session child and flag-off retained inspection are explicit states. Saved cards never fetch details until Open. Preserve parent reader anchors, ordinary sibling subscriptions and phone drawer scroll/Back behavior.

Acceptance: (1) demand-only bounded history and targeted inspection satisfy all race/cache states, (2) all browser selectors stop assuming a full Errands inventory without narrowing legacy clients, (3) localized desktop/phone states and normal subscription ownership remain correct. Exclude general UI redesign and provider/runtime cleanup changes.

## ASCII UI preview

UI-01: Desktop parent Errands popover, finished history explicitly opened.

```text
Errands                    1 running
Parent conversation
  Running audit          [Open] [Stop]
  Done: result pending          [Open]
v Finished (1000)
  Loading... / No finished errands
  Could not load                [Retry]
  Review checklist              [Open]
  Showing 1-25 of 1000     [Load more]
                         [Stop all]
```

UI-02: Phone Errands pill -> inset drawer -> full-screen child.

```text
+-----------------------------+
| Errands                 [X] | fixed header
| 1 running, 1 result pending |
| Audit         [Open] [Stop] |
| v Finished (1000)           | one internal scroller
| Review checklist     [Open] |
|                     [More]  |
| [Stop all]                  | fixed safe-area footer
+-----------------------------+
          Open ->
+-----------------------------+
| [Back] Review checklist     |
| Child transcript            | full-screen reader
| ...                         |
+-----------------------------+
```

Loading, empty and error are alternative history-body states; retry retains live actions. At the 100-summary cache bound show the retained range and Start again; at end show an end marker. A changed chain offers Refresh rather than silently appending stale pages. Labels are illustrative localized copy, spacing is not a pixel requirement. Required structure maps to AC-TASKS-ERRAND-HISTORY-001.3/.4/.7; task04 proves desktop geometry, phone scroller/44px controls, focus, safe areas and Back anchor restoration.

## Verification

Run from repository root after implementation authorization; commands are future checks, not design evidence. Heavy checks require the shared lock, GOMAXPROCS=2 and no overlapping browser runs. Task00 must record the exact chosen isolated seeding/measurement invocation before its gate can pass. New task04 E2E files are proposed, not present yet. Also run changed-Go lint with explicit changed Go paths and exact-file web lint for files edited by this work order, following scoped AGENTS.md.

```bash
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm exec vitest run hooks/domains/session/use-task-errands.test.ts lib/state/slices/errands lib/ws/handlers/errands.test.ts components/task/chat/errands)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm run typecheck)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm run i18n:check)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm run i18n:ratchet)
```

## Results

Implemented the Task03 demand-only browser history and targeted inspection flow after Task02 acceptance. Each visible panel now owns one history chain shared by its parent disclosures; opening another parent replaces the chain, releases the previous parent’s page lease, and keeps late responses scoped to the active task, parent, cursor, attempt, and observation. Hooks expose history rows only to the matching parent. The four-page/100-row bound, explicit Load more/Start again, refresh-on-stale behavior, summary counts, server-backed Stop all, targeted child inspection, and legacy complete-list API are retained.

The browser consumer audit found `listErrands()` only at its legacy API declaration; no browser consumer calls it. The Errands chip reads the compact browser summary, and finished history uses the paged browser API. Targeted child Open continues through the Task02 inspection/route path without sibling enumeration or an ordinary-session fallback.

TDD RED evidence: `errands-chip.test.tsx` failed with two distinct parent owners where one was expected (1 failed, 25 skipped); `use-task-errands.test.ts` failed because parent A exposed parent B’s active chain (1 failed, 7 skipped). After the shared-owner and parent-scope fix, the exact focused suite passed **13 files / 176 tests**. It covers coalesced waiters, releasing the initiating consumer, parent switching and abort, late A-to-B-to-A responses, session-only observation revisions, tombstones, terminal pending results, bounded eviction, targeted detail owners, and legacy WebSocket projections.

Checks passed: web typecheck; exact-file Oxlint with warnings denied; Prettier; `i18n:check` (10,565 referenced keys, all seven shipped locales complete and pseudo synchronized); `i18n:ratchet` (268 added and 487 modified files clean); and scoped `git diff --check`.

Managed isolated backend E2E passed in required order: desktop Chromium **1/1**, then Pixel 5 mobile Chromium **1/1**. A raw desktop rerun on the already verified managed build passed **1/1** to retain both desktop screenshots after the managed runner cleared its prior test-results directory. The dark screenshots use disposable mock seed data and prove the rendered UI, not provider certification. SHA-256 evidence is under `/tmp/kandev-errand-history-task03-evidence/`: `task03-errand-history-desktop-summary.png` (`1f14580ad7ef6425b6764b23b22c205cab5850523c751a4af581420c7bdf509b`), `task03-errand-history-desktop.png` (`ce79a681cd135a690a7ed3dcc2721b4615325ca11198700cabe332b9b05f1aa0`), and `task03-errand-history-phone.png` (`dd8836c583c5eba5d02bd9ce036b6b03ab41049bcba2cbc6b5ae4faffaa0cc67`).

Direct owner-switch checkpoint hashes: `apps/web/components/task/chat/errands/errands-panel.tsx` `8755fb338d56eb2beca2673ec1deeb729d0dc6b086b4e2ca013e52d47cfd1660`; `apps/web/components/task/chat/errands/errands-history-disclosure.tsx` `bc2e3c19c6602f4b983619a4230f12fbcd132984f4a45f6865ef0fb4aa84e83a`; `apps/web/components/task/chat/errands/errands-chip.test.tsx` `500dd0d5ac504febe940ad2ffeeab5a869e93122aa9c8837e724db68032f6e59`; `apps/web/hooks/domains/session/use-task-errands.ts` `1824c17dfe19ccb5b00a6d6d43a53ce1b76723b58a4f84c4f1e953d38b907dab`; and `apps/web/hooks/domains/session/use-task-errands.test.ts` `1df14a5b6168d933523e7e204c9a6071697f481711ffc08473528920dee2b79c`.

Remaining limits: Task04’s frozen 0/100/1000 timing and performance matrix was not rerun; mixed-parent global ordering browser coverage remains in Task04, while this work order’s shared-owner parent-switch behavior is covered at component and hook level. Task03 stays `in_progress` for parent/Era Sol review. No commit or push was made. E2E/test/build processes are terminal, `/tmp/kandev-errands-repair-build.lock` is released, and the main Kandev instance was left untouched.


### Parent acceptance after recovered review

The preceding worker checkpoint is historical. Parent addressed all three recovered findings and the adjacent session-only inspection case; [final remediation evidence](task-03-parent-remediation.md) records the exact source, RED/GREEN results, 274-test combined control suite, two inspection tests, typecheck/lint and fresh desktop/phone 1/1 passes. Task03 is accepted as the demand-history implementation boundary. Task04 owns remaining combined compatibility, rendered inspection and frozen performance acceptance. No commit or push is claimed.
