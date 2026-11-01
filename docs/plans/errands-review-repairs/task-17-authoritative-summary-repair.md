---
id: "17-authoritative-summary-repair"
title: "Repair missing and stale errand summaries"
status: done
wave: 17
depends_on:
  - "16-bounded-summary-aggregate"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-006.3
  - AC-TASKS-ERRANDS-010.5
  - AC-TASKS-ERRANDS-012.2
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 17: Repair missing and stale errand summaries

## Scope and authority

Repairs review finding(s) 12 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Partial implementation exists from earlier interrupted work. Task18 verification reproduced CAS-reload failures and a complexity lint issue; recover and complete the implementation without replacing unrelated accepted changes.

## Low-level implementation and regression plan

### Root cause and ownership

The event-driven statussummary.Projector has LoadErrands, refreshErrandsLocked and restoreErrands. The parallel hydration/read-repair path has no Errands in RebuildInput, no Errands comparison in summaryNeedsReconcile and no fresh aggregate on CAS retry. A missing row loses Errands entirely; an existing stale row can keep wrong counts indefinitely until a suitable source event happens. Replaying a cached DTO or subscribing every board row to children would violate the existing boundary.

Owned files:
- apps/backend/internal/task/service/service.go: one optional statusSummaryErrands field.
- New apps/backend/internal/task/service/service_status_summary_errands.go: TaskStatusSummaryErrandsReader function type, setter, task observation loading/validation and equality/clone helpers.
- apps/backend/internal/task/service/service_status_summary_rebuild.go: ReconcileTaskStatusSummaries, reconcileExistingSummaries, rebuildMissingSummaries, rebuildMissingSummary.
- apps/backend/internal/task/service/service_status_summary_rebuild_input.go: rebuildInput.
- apps/backend/internal/task/service/service_status_summary_reconcile.go: reconcileExistingSummaryWithGateObservation, summaryNeedsReconcile, nextReconciledSummary, reloadSummaryReconcileState/finishSummaryReconcile.
- apps/backend/internal/task/statussummary/rebuild.go: RebuildInput and BuildFromAuthoritative.
- apps/backend/internal/backendapp/status_summary_errands.go: narrow batch-reader interface and taskErrandsSummaryBatchReader factory.
- apps/backend/internal/backendapp/gateway.go: configure the service reader beside SetTaskStatusSummaryEventProjector, BEFORE projector.Start.
- New apps/backend/internal/task/service/service_status_summary_errands_test.go; extend existing backendapp/status_summary_errands_test.go and statussummary/rebuild_test.go. Extend service_status_summary_batch_test.go only as needed for bounded-read/no-op assertions.

### Resolved interface and flag choice

Use the existing backendapp flag closure pattern, not an unguarded type assertion on s.tasks: it would read retained errands while the flag is off. Define TaskStatusSummaryErrandsReader as func(context.Context, []string) (map[string]statussummary.ErrandsSummary,error), set through SetTaskStatusSummaryErrandsReader. Nil reader means unobserved compatibility state. In production always wire a closure. Enabled -> real batch read; disabled -> complete explicit zeros for requested IDs with NO repository call. This mirrors taskErrandsSummaryLoader and clears stale retained Errands projection while disabled. No runtime flag entry/default change.

### Algorithm

1. At ReconcileTaskStatusSummaries entry, gather valid task IDs once and batch-observe Errands once for this request. Represent each task as summary/observed/error; a successful empty aggregate is observed zero, nil reader is unobserved, and a missing key in a supposedly complete successful map is an observation error, not zero.
2. Thread observations through existing existing/missing-summary flows. In RebuildInput add Errands (bounded value) and ErrandsObserved. BuildFromAuthoritative uses existing normalizeErrands/clone semantics; observed Live:0/Waiting:false omits the field, waiting-only retains Live:0/Waiting:true.
3. For an existing row, summaryNeedsReconcile compares normalized Errands only when observed. Keep other fields, revision, queued prompts, errors, launch queue and completion gate semantics. On a mismatch clone current, set/clear Errands, validate, increment revision once, advance UpdatedAt with the existing advancedSummaryTime helper, persist by CAS and publish only accepted writes.
4. Preserve the unobserved path without erasing current.Errands. On an Errands read error, keep the last valid existing row in the response and durable store, return/log the repair error through the existing error plumbing and do not claim freshness. For a missing summary with an Errands error, skip creating a fabricated empty summary and log/return the error. This is the ADR's last-valid-on-projection-failure behavior; do not convert error to live:0.
5. On CAS rejection reload the current stored summary AND its authoritative Errands observation via the configured reader for []string{taskID}, together with existing session/pending/gate reloads. Do not reuse the initial aggregate after a competing write. The final exhaustion/no-op check must compare against the newest observation too. Preserve existing bounded three-attempt retry and cancellation semantics.
6. If initial missing-row creation loses CAS, reload the winner as today, then pass it through the same reconciliation path with a re-observed aggregate; do not blindly publish an old Errands snapshot.
7. Same facts -> no revision bump, no event, no task-state mutation. Zero existing summary and zero aggregate -> field remains absent. Success is authoritative DTO + persisted row + accepted event, not merely an in-memory test value.

Internal reader and RebuildInput additions only. Wire field remains existing optional status_summary.errands:{live,waiting}; no change to schema version, migrations, transport subscriptions, endpoints or public type shape.

### Regression suite and reliable RED

- TestTaskStatusSummaryErrandsReadRepair/missing_summary and /stale_summary: promote the real SQLite diagnostic, configure the real reader via setter, seed an idle parent and queued durable child without child session; before correction got.Errands is nil. Assert returned AND reloaded persisted summary have {live:1,waiting:true}. Observe the matching accepted summary event.
- /clears_stale_nonzero, /waiting_only, /mixed_live_and_failed, /parent_begins_turn: settle/delete pending/queued state or run the parent using actual repository writes without projector events; read repair restores exact aggregate.
- TestTaskStatusSummaryErrandsRepairCASReloadsObservation: existing rejectingStatusSummaryRepository wrapper writes a competing revision and changes real errand state before returning false once. Assert final aggregate is the NEW durable value and competitor fields survive. Use channels/barrier or synchronous controlled CAS fake, never sleep.
- TestTaskStatusSummaryErrandsRepairReadFailureKeepsLastValid: inject failure at reader boundary; current revision/persisted Errands/event count remain unchanged; missing summary is not invented; a subsequent successful call repairs it.
- TestTaskStatusSummaryErrandsRepairNoopAndBatching: 130 real tasks, one batch-reader invocation on cold/warm repair (chunking is inside repo), unchanged facts produce no new event/revision. No full errand list or per-task normal-path aggregate calls.
- TestTaskErrandsSummaryBatchReaderFlagOffOmitsRetainedRows: production factory disabled returns complete zero map and makes zero repository calls; positive enabled control proves underlying aggregate is nonzero.
- TestBuildFromAuthoritativeIncludesErrands: real statussummary builder, nonzero/waiting-only/zero inputs, not a stubbed builder.

~~~
(cd apps/backend && go test -trimpath -race ./internal/task/service ./internal/task/statussummary ./internal/backendapp -run 'TaskStatusSummaryErrands|TaskErrandsSummary|BuildFromAuthoritative|TaskStatusSummaryBatchRepair|ReconcileTaskStatusSummar|ReconcileExistingSummary|StatusSummaryActivityRebuild' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/service/service.go apps/backend/internal/task/service/service_status_summary_errands.go apps/backend/internal/task/service/service_status_summary_rebuild.go apps/backend/internal/task/service/service_status_summary_rebuild_input.go apps/backend/internal/task/service/service_status_summary_reconcile.go apps/backend/internal/task/service/service_status_summary_errands_test.go apps/backend/internal/task/statussummary/rebuild.go apps/backend/internal/task/statussummary/rebuild_test.go apps/backend/internal/backendapp/status_summary_errands.go apps/backend/internal/backendapp/status_summary_errands_test.go apps/backend/internal/backendapp/gateway.go
~~~

If touching service_status_summary_batch_test.go include it in changed-Go lint. Existing backendapp boot-summary tests are the integration boundary for boot publication; add one Errands case there only if setter/wiring cannot be exercised by current factory/service cases. No new browser badge implementation is needed; existing board/sidebar/Threads assertions consume this same bounded field.

## Dependencies and ownership

[Task 16](task-16-bounded-summary-aggregate.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

Parallel-safe with Task04C frontend/localization: this work order owns backend summary service/builder/wiring files only. User authorizes two disjoint workers. All heavy checks use `/tmp/kandev-errands-repair-build.lock` and `/tmp/errands-verification-resource-policy.md`; no overlapping builds, lint, typecheck or browser suites.

## Results

- RED, stale/existing and missing summary races: `cd apps/backend && go test -trimpath -race -p 1 -parallel 1 ./internal/task/service -run '^TestTaskStatusSummaryErrandsRepairCASReloadsObservation$' -count=1 -v` failed both subtests before the retry fix; existing returned the pre-competition waiting value and the missing row omitted Errands. Log: `/tmp/errands-task17-red-cas.log`.
- RED, retry read failure: `cd apps/backend && go test -trimpath -race -p 1 -parallel 1 ./internal/task/service -run '^TestTaskStatusSummaryErrandsRepairRetryReadFailureKeepsWinner$' -count=1 -v` failed both existing/missing subtests because no post-CAS aggregate read occurred and its failure was not surfaced. Log: `/tmp/errands-task17-red-retry-read-error.log`.
- Focused GREEN: `cd apps/backend && go test -trimpath -race -p 1 -parallel 1 ./internal/task/service -run '^TestTaskStatusSummaryErrandsRepair(CASReloadsObservation|RetryReadFailureKeepsWinner)$' -count=1 -v` passed 2 top-level tests and 4 subtests with no skips. Log: `/tmp/errands-task17-green-cas.log`.
- Final acceptance: `cd apps/backend && go test -trimpath -race -p 1 -parallel 1 ./internal/task/service ./internal/task/statussummary ./internal/backendapp -run 'TaskStatusSummaryErrands|TaskErrandsSummary|BuildFromAuthoritative|TaskStatusSummaryBatchRepair|ReconcileTaskStatusSummar|ReconcileExistingSummary|StatusSummaryActivityRebuild' -count=1 -v` passed 37 top-level tests and 13 subtests across all 3 packages with 0 skips. Package results: service 3.722s, statussummary 1.087s, backendapp 1.327s. Log: `/tmp/errands-task17-final-race.log`.
- Changed-Go lint passed with one worker: `GOMAXPROCS=1 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/task/service/service.go apps/backend/internal/task/service/service_status_summary_errands.go apps/backend/internal/task/service/service_status_summary_rebuild.go apps/backend/internal/task/service/service_status_summary_rebuild_input.go apps/backend/internal/task/service/service_status_summary_reconcile.go apps/backend/internal/task/service/service_status_summary_errands_test.go apps/backend/internal/task/statussummary/rebuild.go apps/backend/internal/task/statussummary/rebuild_test.go apps/backend/internal/backendapp/status_summary_errands.go apps/backend/internal/backendapp/status_summary_errands_test.go apps/backend/internal/backendapp/gateway.go`; `golangci-lint` reported 0 issues. Log: `/tmp/errands-task17-lint.log`.
- Backend composition build passed: `cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=8GiB go build -trimpath -p 1 -o /tmp/errands-task17-kandev ./cmd/kandev`; the Linux binary was produced outside the repository. Log: `/tmp/errands-task17-build.log`.
- `gofmt -l` on all owned Go files, targeted `git diff --check`, and no-skip review passed. Task16's accepted PostgreSQL aggregate boundary evidence was not repeated; Task17 uses the SQLite fixtures specified here. No broader backend suite, live instance, commit, push, or publication was run.
