---
id: "02-cancellation-settlement"
title: "Preserve cancellation against late settlement"
status: done
wave: 2
depends_on:
  - "01-token-fenced-release"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-007
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.4
  - AC-TASKS-ERRANDS-003.5
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.2
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 02: Preserve cancellation against late settlement

## Scope and authority

Repairs review finding(s) 3 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation is limited to the production and regression files listed below; completion evidence is recorded under Results.

## Low-level implementation and regression plan

Own `internal/errands/results.go`, `results_test.go` and new `cancellation_settlement_test.go`, and coordinate `reconcile.go` with recovery owner. Root cause: `applyCancel` sets lifecycle cancelled and may install cancellation notice/clear or discard output; delayed `applySettlement` accepts any row, overwrites notice with late answer, including after CAS retry. Add guard inside the `s.update` mutation on the fresh row: if `Lifecycle == ErrandLifecycleCancelled`, return false before mutating any field, including `LatestSettledTurnID`, `AwaitingResult`, result, timestamps or outcome. Guard must be inside CAS retry, not just at the stale pre-read; cancellation itself is an independent writer and does not take the parent lock. Preserve existing completed settlement that wins before cancellation and existing user/parent/cascade semantics. Apply equivalent guards inside `writeReconciled` closures `failLaunchedRow` and interrupted settlement: stale observations must not overwrite a now-cancelled or already settled/superseded successor; validate expected lifecycle, awaiting flag, exact turn identity and relevant version, or reread and repeat recovery decision rather than applying stale read-derived text. This packet owns the cancellation guard in these closures; Task 13 later extends their launch-evidence classification. Note that a newly admitted parent turn supersedes pending old results (003.4), so no late settlement/release may resurrect a superseded result. Tasks 03 and 07 own the later admission and exact-turn fence: `LatestTurn` is read outside CAS; a successor appearing after that read can invalidate it unless admission/settlement share an authoritative turn fence. Do not assert turn-ID equality alone if parent admissions have no durable active-turn ID.

RED `TestLateSettlementPreservesUserCancellationNotice`: block settlement after reading turn, cancel user and then release; assert cancellation lifecycle, pending notice text/kind, result ID, route/token and timestamp retained; repeat parent/cascade sources assert no new result, and add positive control completing before cancel. Race test `TestReconcileStaleSettlementDoesNotOverwriteCancellation` exercises actual callback/CAS. Match 003.4–003.5 and 007.1–007.3. Commands:

```
(cd apps/backend && go test -race ./internal/errands -run 'Test(LateSettlementPreservesUserCancellationNotice|ReconcileStaleSettlementDoesNotOverwriteCancellation)')
bash scripts/lint-go-changed apps/backend/internal/errands/results.go apps/backend/internal/errands/cancellation_settlement_test.go apps/backend/internal/errands/reconcile.go apps/backend/internal/errands/reconcile_test.go
```

## Dependencies and ownership

[Task 01](task-01-token-fenced-release.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

RED reproduced the defect before production edits. The focused race test failed because late success/failure replaced cancelled rows, and stale interrupted/launch-failure reconciliation replaced cancellation after the forced CAS retry. The completed-turn positive control passed.

- RED: `cd apps/backend && GOMAXPROCS=2 go test -trimpath -p 1 -race ./internal/errands -run 'Test(LateSettlementPreservesUserCancellationNotice|CompletedSettlementBeforeTurnCancellationKeepsTheResult|ReconcileStaleSettlementDoesNotOverwriteCancellation|ReconcileStaleLaunchFailureDoesNotOverwriteCancellation)$' -count=1` exited 1 as expected; log: `/tmp/errands-task02-red.log`.
- Focused GREEN: the same command and test selection after the cancellation and stale-snapshot guards passed (`ok github.com/kandev/kandev/internal/errands 1.281s`); log: `/tmp/errands-task02-green-focused.log`.
- Full package race suite: `cd apps/backend && GOMAXPROCS=2 go test -trimpath -p 1 -race -v ./internal/errands -count=1` passed with 88 reported test/subtest passes (`ok github.com/kandev/kandev/internal/errands 1.196s`); log: `/tmp/errands-task02-full-race-verbose.log`.
- Changed-file Go lint: `bash scripts/lint-go-changed apps/backend/internal/errands/results.go apps/backend/internal/errands/results_test.go apps/backend/internal/errands/reconcile.go apps/backend/internal/errands/reconcile_test.go` passed with `0 issues`; log: `/tmp/errands-task02-go-lint.log`.
- `git diff --check` passed for the four assigned Go files.

The user-cancel regression observes the committed cancellation row before releasing a blocked settlement read; the public cancel call may wait for the settlement-held parent lock to finish its delivery check. No test or check was skipped.
