---
id: "11-notification-dispatch"
title: "Dispatch notifications with safe retry acceptance"
status: done
wave: 10
depends_on:
  - "10-delivery-snapshots"
requirements:
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
plan: "plan.md"
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.7
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
  - AC-TASKS-ERRANDS-004.11
  - AC-TASKS-ERRANDS-004.12
  - AC-TASKS-ERRANDS-006.1
  - AC-TASKS-ERRANDS-006.2
  - AC-TASKS-ERRANDS-007.5
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 11: Dispatch notifications with safe retry acceptance

## Summary

Make a result wake an idle Auto-run parent and make manual Send now use the
same notification semantics. Preserve composed context and the task 10
claim through provider retry and fresh-runtime fallback without duplicate
results or premature workflow completion.

## In scope

- After successful notification queue admission, request the existing
  readiness/drain path outside errand locks. Publishing queue status alone
  does not start a turn. Auto-run off retains one queued notification.
- Preserve notification identity through Send now source claims. Handle a
  notification-only claim, a claim combined with ordinary user text, an
  already-consumed notification, queue refusal, and a competing user turn.
  Use the current FIFO claim/receipt API; do not restore obsolete queue flags.
- Carry the composed prompt through 529 retry and `ErrExecutionNotFound`
  fresh launch. Reuse `startCreatedSessionOptions` admission callbacks and
  the pattern in `queue_send_now_initial_admission.go` to propagate accepted,
  refused, and uncertain outcomes to claim finalization.
- Prove that only the first parent turn completing with no outstanding work
  resumes workflow. An empty notification never creates a provider turn.

## Out of scope

- New scheduler loops, batching timers, delivery routes, or queue storage.
- Child cancellation settlement and disabled deletion (task 12).

## Acceptance

1. A result arriving after the parent is idle starts exactly one parent turn
   with Auto-run on, queues exactly one notification with Auto-run off, and
   stays pending on admission refusal without blocking ordinary user prompts.
2. Send now preserves the current user text and notification envelope;
   consumed/withdrawn notifications produce no empty turn or repeated result.
3. Accepted, refused, and uncertain fresh-launch/retry cases preserve the
   composed block and finalize/release claims correctly; workflow runs only
   after the eventual parent completion with no outstanding results.

## Verification

Run from the repository root. Add TDD cases in the existing suites rather than
asserting queue insertion as a substitute for a real parent prompt.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Errand|SendNow|HandlePromptDispatchFailure|PromptTask|DrainQueued|CancelledTurn' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator/messagequeue -run 'SendNow|Admission|DeleteByID|Errand' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/errands/... -count=1)
git diff --check
```

Run changed-Go lint on the owned paths. Task 09 adds browser evidence for
Auto-run, manual queue dispatch, and result-card reload.

## Files likely touched

- `apps/backend/internal/orchestrator/errand_parent_deliverer.go`,
  `errand_parent_deliverer_test.go`, queue readiness/dispatch helpers.
- `queue_send_now.go`, `queue_send_now_initial_admission.go`,
  `event_handlers_workflow.go`, `task_operations.go`, and related tests.
- `prompt_dispatch_failure_handoff_test.go` and errand parent-turn tests.
- `internal/orchestrator/messagequeue/send_now.go` only if its existing claim
  metadata cannot retain source identity; preserve ordinary Send now behavior.

## Dependencies

Task 10. Its persisted envelope and acceptance readback are the shared input.

## Risks

- Calling the errand service back under its mutex can deadlock; preserve the
  `ParentDeliverer` non-reentrant contract and existing runtime lifetime.
- Fresh launch success does not establish provider prompt acceptance.
- Uncertainty must not release accepted claims for blind replay.
- Send now may combine a notification and ordinary content in one claim;
  checking only the oldest entry's metadata is insufficient.

## Parallelism

`sequential`

## Inputs

- System-design parent-turn flow and existing Send now initial-admission tests.
- Historical task 05's recorded fallback warning.

## Results

Primary accepted implementation and the recorded post-merge targeted checks on 2026-10-28. Later cancellation and lifecycle packets preserve the dispatch boundary. Browser and selected-provider signoff remain assigned to 09; no publication or readiness claim follows from this status.

Completed. Admitted notifications request the existing queue-readiness path, while Auto-run-off notifications remain queued. Send Now preserves notification identity across folded sources and user text, consumes empty notifications without an empty turn, and carries the composed prompt and admission outcome through transient retry and missing-execution fresh fallback. The retained-runtime 529 retry now reuses the composed prompt without recomposing it.

Verification passed:

- `(cd apps/backend && TMPDIR=/dev/shm GOFLAGS=-p=2 GOGC=50 go test -trimpath ./internal/orchestrator -run 'Errand|SendNow|HandlePromptDispatchFailure|PromptTask|DrainQueued|CancelledTurn' -count=1)`
- `(cd apps/backend && TMPDIR=/dev/shm GOFLAGS=-p=2 GOGC=50 go test -race -trimpath ./internal/orchestrator/messagequeue -run 'SendNow|Admission|DeleteByID|Errand' -count=1)`
- `(cd apps/backend && TMPDIR=/dev/shm GOFLAGS=-p=2 GOGC=50 go test -race -trimpath ./internal/errands/... -count=1)`
- `git diff --check`
- Changed-Go lint passed in serial package batches with the owned paths.
