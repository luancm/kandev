---
id: "02-ordinary-isolation"
title: "Isolate ordinary boot and saved-layout materialization"
status: done
wave: 3
depends_on:
  - "01-browser-reads"
plan: plan.md
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
acceptance_criteria:
  - AC-TASKS-ERRAND-HISTORY-001.1
  - AC-TASKS-ERRAND-HISTORY-001.4
  - AC-TASKS-ERRAND-HISTORY-001.5
  - AC-TASKS-ERRAND-HISTORY-001.6
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Isolate ordinary boot and saved-layout materialization

## Scope, implementation acceptance and exclusions

Own browser-only ordinary summary repository/service option and HTTP/WS list caller wiring, `apps/backend/internal/backendapp/boot_state_task_detail.go`, `apps/web/lib/state/slices/session/session-slice.ts`, session WS reconciliation and Dockview restore/setup/store entry points. Inventory all calls to `ListTaskSessionSummaryObservations`, `listTaskSessions` and `setTaskSessionsForTask` before editing: internal complete consumers keep defaults. In particular, `use-task-removal.ts` must retain complete deletion inventory; only explicit navigation consumers switch to the new ordinary-browser read. History pages never enter authoritative ordinary membership reconciliation. Exclude child rows at query time; preserve full ordinary fields and all normal detail laziness and subscription lifetimes. Prevent child WS arrivals/detail reads from joining normal membership.

Use an authoritative ordinary identity allowlist before any saved-layout panel creation, including maximize/pre-maximize/global migration and delayed identity. Unknown identity holds only session panels; do not persist a temporary filtered layout. Tests delay hydration and spy on creation, not just eventual absence. Preserve normal order/geometry and restore surviving maximized group. Targeted child routing is part of this same slice, before switching any normal-list consumer: wire inspect into boot requested-session resolution, use-open-task-session and parent-owned route helpers. A child excluded from ordinary membership must still open directly and survive reload without ordinary-tab fallback. Keep the existing complete Errands panel read until task03 migrates it; bounded history UI is a later compatible change, not a prerequisite for working child navigation.

Acceptance: (1) zero/100/1000 finished children produce the same ordinary boot/list membership and per-ordinary reconcile work, (2) no child panel is ever created before or after identity hydration on every restore branch, (3) direct ordinary and child links, siblings, normal maximize/layout, complete task removal and reconnect keep baseline behavior. Exclude history UI and lifecycle mutations.

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
(cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 go test -race -p 1 ./internal/backendapp ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -run 'Boot|SessionSummary|SessionScope|Errand' -count=1)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm exec vitest run components/task/dockview-layout-restore.test.ts components/task/dockview-layout-restore-maximize-session.test.ts components/task/dockview-session-tabs.hook.test.tsx components/task/chat/errands/use-open-task-session-owner.test.tsx lib/state/dockview-env-switch-maximize-session-filter.test.ts lib/ws/handlers/agent-session.errand-adoption.test.ts hooks/use-task-sessions.foreground.test.ts hooks/use-task-sessions.stale-reads.test.ts lib/state/task-session-reads.test.ts lib/state/hydration/hydrator-session-freshness.test.ts)
(cd apps/web && flock /tmp/kandev-errands-repair-build.lock pnpm run typecheck)
```

## Ordinary-session compatibility evidence

Baseline review disposition: prove ordinary filtering before enrichment and boot side-map construction, with deterministic reconciliation row/call bounds. Broad CDP renderer deltas remain unattributed and are not isolated reconciliation timings. Hold identity unresolved in normal, maximized and pre-maximized restoration tests, then release it and assert preserved ordinary geometry and zero child mounts. The baseline's natural identity lag and three waiting sessions are payload controls, not proof of these races or full revision/detail/subscription compatibility.

Preserve complete ordinary membership with zero Errands and with fixed ordinary siblings plus growing finished history. Existing completed, failed and cancelled ordinary sessions retain their numbering, reopen menu and selection behavior. Extend the existing hook/restore tests above with delayed identity and held A-to-B-to-A reads; observe panel creation rather than eventual DOM absence. Protect existing `visible-session-demand` behavior: all ordinary tabs exist while only visible chats subscribe, including both sides of a split. Final browser controls are named in task04.

## Results

Parent accepted Task02 on 2026-11-13. Era Sol accepted the child-navigation, cross-task layout and persistence-reader corrections; parent closed the remaining same-owner preset race by inspecting shared attempt ownership for saved/customized layouts, built-ins and Reset Layout. Source SHA-256 `d340b1de459201dabb244f8f691b2c88e065bca1b5043225bcf7b84b66406e73` and final 14/14 selector GREEN receipt match the handoff. The earlier desktop/phone and persistence results remain prior evidence, not rerun for this narrow callback correction. Original implementation had no retained behavioral RED; remediation has explicit RED/GREEN evidence. Nonverbose persistence receipts establish package passes but DSN activation/no-skip details remain worker attestations. This accepts Task02's isolation/routing slice; Task03 UI and Task04 candidate performance/complete compatibility acceptance remain pending. Earlier in-progress statements below are historical.

Task02 implementation and focused controls completed on 2026-11-13 after Task01 acceptance; the work order remains `in_progress` pending parent review. Browser navigation and task-detail boot now use ordinary-only summaries, while complete operational reads and the complete Errands panel read retain their prior semantics. A directly requested child resolves through one `errand.inspect` plus one targeted detail read and remains outside ordinary membership. Unknown ordinary identity holds only session panels across saved-layout restoration, then replays the saved source without persisting the temporary projection.

Final checks passed: backend race targets (four packages), Task02-scoped Go lint (zero issues), web typecheck, 19 focused Vitest files / 302 tests, exact changed-file Prettier and Oxlint, i18n check and ratchet, desktop ordinary/child control (1/1), and phone ordinary/child reload and Back control (1/1). The full changed-package Go lint invocation timed out while loading the wider Task00/Task01 package set; the scoped Task02 invocation passed. Exact commands, results, limits, and hashes are in [ordinary-isolation-handoff.md](ordinary-isolation-handoff.md).

This does not claim the 0/100/1000 candidate performance matrix, full PostgreSQL certification, or Task04 acceptance. No schema or write path changed in Task02. No commit, push, PR, or external publication was made.

Supplemental reviewer remediation on 2026-11-13 fenced delayed uncached-child publication and saved-layout materialization by navigation owner, request attempt, and mount lifetime. The focused deferred regressions passed after demonstrating stale publication in the pre-fix code. The existing environment-gated PostgreSQL session-summary test now exercises the ordinary reader and compares its complete projection with the complete-reader control; SQLite runs the same assertion. Focused browser controls passed on desktop and phone. Detailed dispositions, receipts, and file hashes are recorded in [ordinary-isolation-handoff.md](ordinary-isolation-handoff.md); parent review remains pending.

The remaining P1 same-owner layout-selection finding is resolved: saved/customized layout attempts now share invalidation with uncustomized built-in selection and Reset Layout. Deferred selector regressions reproduced stale saved-layout application after both replacement actions, then passed with the fix; the focused selector suite passed 14/14 along with web typecheck and exact-file lint/format checks. Task02 remains `in_progress` pending parent acceptance; exact logs and final source hashes are recorded in the [handoff](ordinary-isolation-handoff.md).
