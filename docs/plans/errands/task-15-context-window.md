---
id: "15-context-window"
title: "Show passive context occupancy in errand panel rows"
status: done
wave: 16
depends_on:
  - "14-pill-owned-management"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-CONTEXT-001
acceptance_criteria:
  - AC-TASKS-ERRANDS-CONTEXT-001.1
  - AC-TASKS-ERRANDS-CONTEXT-001.2
  - AC-TASKS-ERRANDS-CONTEXT-001.3
  - AC-TASKS-ERRANDS-CONTEXT-001.4
system_design:
  - ../../specs/tasks/system-design/errand-context-window.md
---

# Task 15: Show passive context occupancy in errand panel rows

## Summary and scope

Add a compact context-occupancy line to each Errands panel/drawer row when the child's existing session telemetry permits it. The user delegated the trade-off decision; investigation found this worthwhile because unopened children already receive workspace context events and metadata restoration. Task 14 remains complete; this follow-up is partially implemented and awaiting a bounded summary-projection correction.

Own the passive row presentation, a shared pure interpretation helper, regression coverage and the public coordination guide. Reuse `useSessionContextWindow(childId, false)`, existing grouped rows and locale keys. Include the bounded existing session-summary context projection; exclude new DTO fields, full metadata hydration, extra loaders/subscriptions, interactive row rings, aggregate context counts, historical card changes, automatic resets and new provider certification.

## Acceptance

1. Desktop/phone rows show each child's measured percentage, including 100%, or explicit unmeasured text for known windows without a positive sample. Unavailable/impossible reports are omitted; the parent reading never supplies a child value. Persisted/live updates and clears remain consistent after reload (AC-001.1/001.2).
2. Opening the management panel causes no child detail subscription, transcript request or agent admission. The existing context widget remains behaviorally unchanged and context does not affect lifecycle/counts/actions (AC-001.3).
3. Phone rows remain one accessible 44px Open target with context exposed in their description; desktop Open, phone Back, narrow fine-pointer geometry, internal scrolling and the fixed bulk footer work without overflow or nested buttons (AC-001.4).

## ASCII UI preview

UI-12, excerpt from [the combined preview](plan.md#context-window-follow-up-ui-12). Labels and values are illustrative; hierarchy and interaction are required.

```text
Desktop pill -> popover              Phone pill -> existing bottom drawer
Parent                              Parent
  Child A / profile                   [ Child A / profile           > ]
  Running / elapsed      [Open]       [ Running / elapsed             ]
  Context 68%                         [ Context 68%                   ]
  Child B / profile                   [ Child B / profile           > ]
  Running / elapsed      [Open]       [ Running / elapsed             ]
  Context: not measured               [ Context: not measured         ]
[Stop all errands]                  [Stop all errands]  fixed footer
```

Use the existing Errands drawer as the mobile exemplar and its list as the sole scroll owner. Context is passive secondary text; unknown readings add no line. Desktop retains its separate Open action. Phone row tap opens full-screen child chat with Back. Full usage/source/compaction details remain in the opened child's current widget.

## Files and implementation boundaries

- New `apps/web/lib/context-window-usage.ts` and `.test.ts`: isolate the existing reliability, unmeasured and rounded-percent rules for the two consumers. Keep existing tooltip formatting/behavior intact.
- `apps/web/components/task/chat/token-usage-display.tsx` and existing test: consume the shared helper with unchanged output; preserve or adjust internal imports coherently.
- `apps/web/components/task/chat/errands/errands-panel-row.tsx`: select the child reading once and pass passive content/description IDs through both presentation branches. No nested context trigger.
- `apps/web/hooks/domains/session/use-session-context-window.test.ts`: protect passive metadata/live/clear behavior without child subscriptions. Existing store/event tests remain regression evidence.
- `apps/web/e2e/tests/session/{errands.spec.ts,mobile-errands.spec.ts}` and `apps/web/e2e/helpers/errands.ts`: share fixture setup. Use existing `seedTaskSession` to persist/broadcast samples for explicit child IDs; preserve lifecycle fields and do not patch clear handlers. Assert measured/unmeasured/unknown, two-child isolation, live change, reload, explicit clear and phone geometry. Use the existing context-source browser suites to protect the shared widget.
- `docs/public/coordination.md`: describe the new optional panel readout after implementation. Locale catalogs change only if existing keys cannot express the compact line.

## Correction packet: persisted reload

The first implementation stopped because compact task-session summaries omit context metadata. Preserve AC-001.2 and all partial frontend changes. Add the fixed-field projection specified in the owning design before finishing the original desktop/phone proof. This is a correction within task15, not a successor or a new owner. The existing child is the sole implementation owner only after the parent's explicit regrant.

Additional owned files: `apps/backend/internal/task/repository/sqlite/session_summary.go`, `session_summary_projection_test.go`, `apps/backend/internal/task/service/service_sessions.go`, `service_session_summary_projection_test.go`; a small task-model projection helper and adjacent tests if needed to keep SQL and fallback semantics aligned. Extend DTO/handler tests only to prove propagation through existing fields. No schema, route, producer, Errand DTO field, full metadata, child loader/subscription or authorization change is allowed.

TDD must prove measured, unmeasured, exactly-full, missing, explicit-null, invalid shapes and unrelated-field exclusion. Preserve batch/list parity, model/error fields and the 4KiB/1MiB payload-size invariant. Cover fallback parity and reload after clear; include both SQLite and actual PostgreSQL evidence. A skipped PostgreSQL test is an unresolved check, not a pass. Use the existing disposable database only if still available and isolated; never the user's main database. Record exact local DSN setup privately in scratch and redact credentials from reports.

Additional commands from repository root, before the original frontend/browser checks:

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestSessionSummaryProjection' -count=1)
(cd apps/backend && go test ./internal/task/service ./internal/task/dto ./internal/task/handlers -run 'Summary|Projection' -count=1)
```

Run the same repository selector with the repository's documented PostgreSQL test environment configured, recording real execution separately from skips. Run `bash scripts/lint-go-changed` with every changed Go path as an explicit argument, and `gofmt -l` for those files. Do not run the full backend suite. Then complete every original task15 command below, desktop and phone sequentially, and inspect phone measured/unmeasured renderings. Stop and release all processes/writes on handoff. Existing 121 passing unit tests and the failing desktop reload are preliminary evidence only.

## Verification

Implement with TDD. Planned new helper tests provide the first red; browser cases prove actual row composition and workspace updates. From repository root, run each command independently:

```bash
(cd apps/web && pnpm exec vitest run lib/context-window-usage.test.ts components/task/chat/token-usage-display.test.tsx hooks/domains/session/use-session-context-window.test.ts components/task/chat/errands/errands-chip.test.tsx lib/state/slices/session-runtime/context-window.test.ts lib/ws/handlers/agent-session.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/context-window-usage.ts components/task/chat/token-usage-display.tsx components/task/chat/errands/errands-panel-row.tsx e2e/helpers/errands.ts e2e/tests/session/errands.spec.ts e2e/tests/session/mobile-errands.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts e2e/tests/chat/context-window-source.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts e2e/tests/chat/mobile-context-window-source.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

If a new key is needed, run `(cd apps/web && pnpm run i18n:zh-hant)` before localization checks and review its output. Do not claim an actual provider/reset-command pilot from fixture samples. Record browser discovery/counts and inspect rendered phone measured/unmeasured rows.

## Dependencies, risks and parallelism

Depends on completed task 14. Execute sequentially; shared interpretation, both renderers and fixtures overlap. Preserve workspace-event semantics and explicit-null invalidation; missing child metadata remains absent, without fetching detail. The seed helper also updates lifecycle fields, so use settled fixture children and preserve those fields. Both owning Errands documents are near their size ceilings; this focused vertical requirement/design pair carries the new context contract without expanding unrelated lifecycle content.

## Results

The fixed-field context summary now carries `context_window` scalars and `context_compaction_count` through the existing repository projection and service fallback. Shared model projection tests preserve missing versus explicit-null semantics, reject malformed shapes without failing hydration, and exclude unrelated/nested metadata. SQLite, service/DTO/handler, model and dialect selectors pass. Red/green evidence: `/tmp/task15-backend-context-model-red.log`, `/tmp/task15-backend-context-model-green.log`, `/tmp/task15-backend-context-sql-red.log`, `/tmp/task15-backend-context-fallback-red.log`, `/tmp/task15-backend-sqlite-final.log`, `/tmp/task15-backend-service-final.log`, `/tmp/task15-backend-model-final.log`, and `/tmp/task15-backend-dialect-final.log`.

PostgreSQL evidence is now complete: `TestSessionSummaryProjectionPostgres` executed and passed, not skipped, alongside the SQLite and SQL-shape cases. PostgreSQL 16.15 ran from the disposable extraction `/tmp/errands-delivery-20261029/pg`, bound only to loopback port 55439. The test used a dedicated temporary database plus `OpenIsolatedPostgres`'s unique per-test schema; that database was dropped after the run. Exact command and result are `/tmp/errands-delivery-20261029/pg/task15-context-window-command.txt` and `/tmp/errands-delivery-20261029/pg/task15-context-window-test.log`. Start/identity/drop/stop evidence is in the adjacent `task15-postgres-*.log` files. The earlier unavailable skip at `/tmp/task15-backend-postgres-unavailable.log` is superseded by this executed result. The server was stopped; `/tmp/errands-delivery-20261029/pg/task15-postgres-stop-verification.log` confirms no postmaster, process or listener remains. The temporary DSN environment file is mode 0600 and is not included in handoffs. Changed-Go lint reports 0 issues at `/tmp/task15-backend-lint-final.log`; `gofmt -l` emitted no paths at `/tmp/task15-backend-gofmt-final.log`. Web unit tests pass 121/121 across 6 files at `/tmp/task15-unit-tests-final.log`; ESLint, typecheck, i18n check and ratchet pass at `/tmp/task15-eslint-final.log`, `/tmp/task15-typecheck-final.log`, `/tmp/task15-i18n-check-final.log`, and `/tmp/task15-i18n-ratchet-final.log`.

Desktop Chromium passed 11/11 tests, including measured, unmeasured, unknown, live update, persisted reload, and explicit clear: `/tmp/task15-e2e-desktop-final.log` and raw `/tmp/kandev-run.e2e.dvC7dyTm.log`. Phone Chromium passed 8/8 tests, including one 44px Open target, accessible context description, internal scrolling, fixed footer, reload, and clear: `/tmp/task15-e2e-phone-final-retry.log` and raw `/tmp/kandev-run.e2e.CMQtKPLl.log`. The first phone attempt failed because its fully settled fixture correctly hid Stop all; the fixture now keeps one child live, and the final run passes. Inspected screenshot with both measured and unmeasured rows plus the fixed footer: `apps/web/e2e/test-results/session-mobile-errands-pho-b149e--one-accessible-Open-target-mobile-chrome/errands-phone-context-window.png`.

Documentation checks pass: 421 decisions/1,852 specifications validated, full spec lint passed, public-doc tests passed 63/63, and 54 public pages validated. Logs: `/tmp/task15-list-docs-validate.log`, `/tmp/task15-spec-lint-final.log`, `/tmp/task15-public-docs-test-final.log`, `/tmp/task15-public-docs-validation-final.log`; `git diff --check` passes at `/tmp/task15-diff-check-final.log`. The public coordination guide now documents the optional row readout. Branch/head remain `feature/plan-errands-first-d-424` / `0a045f6d2c75d127aec9bf902dcc786ea6d3ba31`. No staging, commit, push, PR, default change, or workflow advance was performed. Parent accepted the complete evidence and explicit process/write release on 2026-10-30. Work order is `done`; delivery remains a separate phase.
