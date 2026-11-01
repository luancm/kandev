---
id: "06-executor-role-guard"
title: "Preserve task state on unreadable session role"
status: done
wave: 6
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.1
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 06: Preserve task state on unreadable session role

## Scope and authority

Repairs review finding(s) 11 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implemented with session-aware runtime reconciliation and required fallback role/activity evidence.

## Low-level implementation and regression plan

**Owned:** `internal/orchestrator/executor/executor_execute.go:handleLaunchFailure`, `executor_resume.go:{writeTaskInProgressForRuntime,writeTaskFailedForRuntime}`, `errand_session_context.go:sessionIsErrand`; `executor_errand_test.go`. No orchestrator callback contract changes.

**Algorithm:** After `transitionLaunchFailure` reports an actual changed session, call `writeTaskFailedForRuntime(taskID,sessionID)` instead of `!sessionIsErrand` + task-only `updateTaskState`. This existing seam dispatches to `onTaskRuntimeStateReconcile`, whose production implementation reads the session and checks ownership under `taskRuntimeStateMu`, with DB CAS guarding state. Its fallback already returns read errors and rejects an errand. Log a failed reconciliation and return the sanitized original provider error; a failed task-role read cannot authorize a write. For the IN_PROGRESS repository fallback use one required session read for role and working-state checks, not a bool read followed by a second independent read. Remove `sessionIsErrand` once no callers remain, or change it to `(bool,error)` with every caller handling errors fail-closed; do not retain a misleading error-as-ordinary helper.

**State:** Child STARTING -> FAILED still occurs. Shared task stays exactly unchanged for errand, missing, unreadable, mismatched or stale session evidence. An ordinary eligible session still follows the existing task FAILED path. No new schema, API, error reason or metric label.

**Tests:** `TestErrandLaunchFailureRoleReadErrorLeavesSharedTaskState` ports the two cases in the existing executor diagnostic: repository fallback and production-shaped `onSessionStateTransition` + `onTaskStateChange` callbacks. Fail the independent role read after session transition succeeds; enable `onTaskRuntimeStateReconcile` and observe the actual state, not just callback count. Add `TestRuntimeTaskStateFallbackRoleReadFailureIsClosed` for both FAILED and IN_PROGRESS, with a one-shot first read failure and a succeeding second read poised to reveal the old double-read gap. Keep ordinary-session positive controls and original error sanitization assertions.

```bash
(cd apps/backend && go test -race -trimpath ./internal/orchestrator/executor -run 'Test(ErrandLaunchFailureRoleReadError|HandleLaunchFailureLeavesTaskState|RuntimeTaskStateFallback|Bootstrap|EarlyLaunchFailure)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_execute.go apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/executor/errand_session_context.go
git diff --check
```

**ACs:** `005.1`, `012.3`; ordinary-session compatibility is a positive control. Complete IDs are `AC-TASKS-ERRANDS-005.1` and `AC-TASKS-ERRANDS-012.3`.

## Dependencies and ownership

None.

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Preserved and completed the previous executor implementation. Launch failure now routes task FAILED through `writeTaskFailedForRuntime` after a successful child session transition, keeping runtime reconciliation authority and the sanitized original provider error. Removed the error-as-ordinary `sessionIsErrand` helper. Repository fallbacks reject errand/mismatched/stale/missing evidence; IN_PROGRESS now performs one required session read even when its identifier is empty, removing the remaining bypass. Ordinary eligible sessions still change task state.

- RED: `GOMAXPROCS=2 go test -trimpath -p 1 -overlay /tmp/errands-recovery-task06-baseline/overlay.json ./internal/orchestrator/executor -run 'Test(ErrandLaunchFailureRoleReadError|RuntimeTaskStateFallback)' -count=1 -v` reproduced role-error writes on both callback/fallback paths, swallowed IN_PROGRESS read errors, mismatched task/session writes and the empty-ID bypass. `/tmp/errands-recovery-task06-red.log`. The overlay uses original production files from `6400a265bb8a0a35fb702eced2aff1eeb81023d9`; shared partial edits were never reverted.
- Final GREEN: `GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/orchestrator/executor -run 'Test(ErrandLaunchFailureRoleReadError|HandleLaunchFailureLeavesTaskState|RuntimeTaskStateFallback|Bootstrap|EarlyLaunchFailure|ExecuteWithProfile_PersistsEarlyLaunchFailure|WriteTaskInProgressForRuntime)' -count=1 -v` passed 13 top-level tests and 19 subcases with no skips; `/tmp/errands-recovery-task06-final-race.log`. Includes ordinary-session and error-sanitization controls, missing/empty/stale/mismatched evidence and bootstrap successor/early-launch fences.
- Final lint: `bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_execute.go apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/executor/errand_session_context.go apps/backend/internal/orchestrator/executor/executor_errand_test.go` passed, 0 issues; `/tmp/errands-recovery-task06-final-lint.log`.
- No schema, public API or new error/metric identity. No Task07, commit, push or live-instance changes.
