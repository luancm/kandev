---
id: "05-parent-settlement-guard"
title: "Fail closed on parent settlement errors"
status: done
wave: 5
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-006.1
  - AC-TASKS-ERRANDS-006.2
  - AC-TASKS-ERRANDS-006.5
  - AC-TASKS-ERRANDS-007.5
  - AC-TASKS-ERRANDS-012.2
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 05: Fail closed on parent settlement errors

## Scope and authority

Repairs review finding(s) 4 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implemented with read-error propagation and fail-closed task workflow settlement.

## Low-level implementation and regression plan

**Owned production files/symbols:** `internal/orchestrator/errand_settlement.go` (`settleParentTurn`, `evaluateCompletedAgentWorkflow`), `event_handlers_agent.go` (ready and recoverable-failure callers), `task_operations.go` (`finishCancelledAgentTurn`), `cancel_turn.go` (`runTurnCancellationOwned`). Own the matching call-site tests in `errand_settlement_test.go` and add cases to `errand_isolation_test.go`; update assertions in `errand_acceptance_test.go` for the private helper's new return signature only.

**Interface/schema:** Change private `settleParentTurn` to return `(outstanding bool, err error)`. No service/public API change: `errands.OnParentTurnSettled` already has that shape. No DB or DTO changes. Nil/unconfigured controller and empty ID stay `false,nil`; the service's disabled branch stays `false,nil`.

**Algorithm:**

1. Preserve the original read error. Log once with session identity; do not translate it into an empty set.
2. At each completion caller compute `deferWorkflow := outstanding || settleErr != nil`. A read error forbids workflow evaluation and task review writes, but does not forbid the user's next prompt.
3. On a known-outstanding result retain `recordErrandsOutstandingNoOp`. On an unreadable result do not falsely record a successful `errands_outstanding`/`not_required` workflow receipt; leave the original durable effect unresolved for its existing settlement retry. Keep original turn/entry attribution; do not evaluate the latest arbitrary entry on a retry.
4. Ready/error/completed paths settle only the session to `WAITING_FOR_INPUT` with `setSessionWaitingForInputWithoutTaskState`. Existing completed-path `outstanding` boolean can be renamed `deferTaskWorkflow` internally so it includes read failure without pretending that the inventory is known. Preserve runtime cleanup and cancellation finishing; do not skip physical cancellation because workflow readback failed.
5. `CancelAgent`/`CancelTurn`: end awaits and pause Auto-run exactly as today. A read failure skips `reconcileCancelledAgentWorkflow`, produces no new notification or child cascade, and does not turn successful physical cancellation into a retry of the old provider prompt.
6. A subsequent correctly attributed parent completion with readable, empty outstanding state evaluates normally. Do not permanently park a parent or force `waiting_on_errands=true` when state is unknown.

**Tests:** Replace `TestSettleParentTurnFailureIsNotOutstanding` with `TestParentSettlementReadFailureDefersWorkflow`. Add `TestErrandParentSettlementReadFailureNeverMovesOrReviewsTask` table with real moving workflow fixture, actual pending errand row, and one-shot Store `ListErrandsByParent` fault at the production boundary. Exercise `agent.ready`, `agent.completed`, `CancelAgent`, `CancelTurn`; cancellation has `cancel_triggers_turn_complete` enabled. Assert actual task step and task-state write history unchanged, parent idle/promptable, no signal recorded, no child stop. Positive controls use identical moving configuration with no outstanding errands and prove a transition is reachable; a second readable completion restores normal behavior. Include configured disabled gate control. A fake returning `outstanding=true,error` only at the helper is supplementary, not full regression evidence.

**Commands from repo root after implementation:**

```bash
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'Test(ParentSettlement|ErrandParentSettlement|ErrandParentCancellation|ParentTurnCompleting|ParentWorkflowWaits|SettlementHooks|CancelledTurn)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/errand_settlement.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/cancel_turn.go
git diff --check
```

**Overlap:** Task 07 owns the drain ordering and choice of state setter; Task 05 must not independently modify result capture or claim release.

## Dependencies and ownership

None.

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Implemented `(outstanding, error)` propagation, with the original read error logged once by `settleParentTurn`. Ready/completed paths without a queued successor, recoverable-failure and CancelAgent callers defer task workflow and REVIEW on unreadable inventory without recording a successful workflow receipt. Session completion, physical cancellation, queue pause and runtime cleanup remain independent. Silent CancelTurn keeps its existing no-workflow behavior. Tests exercise actual Store faults, live child rows/runtime identity, moving workflows, readable/disabled controls and a later correctly attributed completion without resolving the original unknown receipt.

- RED: real-store regressions reproduced REVIEW writes and workflow movement before the fix; `/tmp/errands-recovery-task05-red-valid-fixture.log` and `/tmp/errands-recovery-task05-red-final-fixture.log`. Earlier fixture setup and helper-type failures are recorded separately in `/tmp/errands-backend-recovery-progress.md`, not counted as behavioral RED. The final fixture uses the real task turn service, initial-submission-capable runtime fake, persisted workflow-policy rows and joined on-enter cleanup.
- Final GREEN: `GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/orchestrator -run 'Test(ParentSettlement|ErrandParentSettlement|ErrandParentCancellation|ParentTurnCompleting|ParentWorkflowWaits|ParentWithOutstanding|SettlementHooks|CancelledTurn|SettleParentTurn|SettleErrandTurn|CancelAgentSettlesOnlyTheObservedErrandTurn|CancelTurnSettlesOnlyTheObservedErrandTurn|ErrandAgentReady|ErrandAgentCompleted)' -count=1 -v` passed 14 top-level tests and 22 subcases, no skips; `/tmp/errands-recovery-task05-final-race-after-lint.log`.
- Final integration after Task06 production completion: the same extended orchestrator selector passed all 14 tests and 22 subcases under `-race`, no skips; `/tmp/errands-recovery-task05-integration-race.log`.
- Final lint: `bash scripts/lint-go-changed apps/backend/internal/orchestrator/errand_settlement.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/cancel_turn.go apps/backend/internal/orchestrator/errand_settlement_test.go apps/backend/internal/orchestrator/errand_isolation_test.go apps/backend/internal/orchestrator/errand_acceptance_test.go` passed with 0 issues; `/tmp/errands-recovery-task05-lint-after-fix.log`. The initial parallel-run lock collision was infrastructure; a later test-only slice-copy finding was corrected and the final race/lint checks rerun.
- `git diff --check` passed; `/tmp/errands-recovery-diff-check.log`.
- Task07 remains untouched: the existing completion queue drain still precedes the new error branch, and its REVIEW setter and not_required receipt handling remain the queued-successor integration gate. These checks do not prove isolation with a queued successor. No result-capture or claim-release changes in this packet. No commit, push or live-instance mutation.
