---
id: "02-workflow-isolation"
title: "Detach errand sessions from workflow, primary, and routing"
status: done
wave: 2
depends_on:
  - "01-flag-and-persistence"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-005.1
  - AC-TASKS-ERRANDS-005.2
  - AC-TASKS-ERRANDS-005.3
  - AC-TASKS-ERRANDS-005.4
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 02: Detach errand sessions from workflow, primary, and routing

## Summary

Introduce `models.IsWorkflowDetachedSession` and route every completion
follow-up check and every session selector through it, so an errand-role
session can never run workflow hooks, write task state, become primary, own
the title, or be picked as a default target. This lands before any errand
can start, and it does not read the flag.

## In scope

- `IsWorkflowDetachedSession(session)`: errand role or the existing
  completion follow-up marker. Migrate the orchestrator call sites listed in
  the [design predicate table](../../specs/tasks/system-design/errands.md#workflow-detached-predicate),
  keeping `prepareOnTurnStartSession` on the follow-up marker only.
- Early returns in `processOnTurnStartViaEngineResult` and
  `processOnTurnCompleteViaEngineWithCause` for detached sessions.
- A waiting settle variant of `setSessionWaitingForInput` that never calls
  `writeTaskReviewState`, used for errand sessions in the agent-completed
  detached branch; `runtimeSessionOwnsTaskState` returns false for errands.
- Primary exclusions: `bestPrimarySessionCandidate`,
  `promoteSessionIfTaskHasNoPrimary`, `promoteNextPrimaryAfterRemoval`, and a
  refusal in `Repository.SetSessionPrimary`; errands are inserted with
  `IsPrimary=false`.
- Routing exclusions: `resolvePrimaryOrActiveSession`,
  `feederCandidateSession`, the `resolveMessageTargetSession` fallback,
  `selectReusableWorkflowSession`, `ciAutomationSessionCanReceivePrompt`, and
  generated-title ownership.
- `validateMoveSessions` keeps blocking on a running errand.

## Out of scope

- Parent workflow deferral and turn composition (task 05). Errand creation
  (task 04).
  Menu changes for **Set as Primary** and **Handoff** (task 08).

## Acceptance

1. With an errand-role session seeded on a task, each listed path skips or
   refuses it while completion follow-up behavior stays unchanged; table tests
   cover every call site.
2. An errand turn completion records the `not_required` workflow outcome,
   settles the errand to `WAITING_FOR_INPUT`, and leaves task state and step
   unchanged.
3. The detachment applies with `features.errands` off and on.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/task/models -run 'WorkflowDetached' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Errand|WorkflowDetached|CompletionFollowUp|PrimarySession' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/service -run 'Errand|Feeder|ResolvePrimaryOrActive|ValidateMove' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/handlers -run 'MessageTarget|Errand' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'SetSessionPrimary|Errand' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/models/models.go` and a new
  `workflow_detached_test.go`.
- `apps/backend/internal/orchestrator/event_handlers_agent.go`,
  `event_handlers_workflow.go`, `event_handlers_streaming.go`,
  `workflow_move_preview.go`, `ci_automation_dispatch.go`,
  `primary_session_candidate.go`, `task_operations.go`, and a new
  `errand_isolation_test.go`.
- `apps/backend/internal/task/service/service_workflow.go` and a new test.
- `apps/backend/internal/mcp/handlers/message_target_session.go`.
- `apps/backend/internal/task/repository/sqlite/session.go`.

## Dependencies

Task 01 for the role column and model fields.

## Risks

- The agent-completed path orders clarification, pending-move, queue drain,
  and follow-up branches; changing the follow-up branch must not reorder them.
- PR [#4605](https://github.com/kdlbs/kandev/pull/4605) changes the terminal
  primary queue hand-off near the message routing fallback; rebase and
  re-check the errand exclusion if it lands first.

## Parallelism

`parallel-safe` with task 03.

## Inputs

- [System design](../../specs/tasks/system-design/errands.md#workflow-detached-predicate)
  and [primary session ownership](../../specs/tasks/requirements/primary-session-ownership.md).
- Existing completion follow-up tests in `internal/orchestrator`.

## Results

Completed on 2026-10-26 in the primary session, test-first. No delegation.
Nothing reads `features.errands`.

- Predicates: `models.IsErrandSession(session)` (role only) and
  `models.IsWorkflowDetachedSession(session)` (errand or completion
  follow-up marker), plus `models.ErrErrandSessionPrimary`.
- Detached call sites: `handleAgentReady` (via the new
  `settleDetachedAgentReadyTurn`), `finishAgentCompleted`,
  `finishAgentCompletedTurn`, `handleRecoverableFailureLockedState`,
  `processOnTurnCompleteViaEngineWithCause`,
  `processOnTurnStartViaEngineResult`, `reconcileTaskStateForRuntimeLocked`,
  `selectReusableWorkflowSession`, and `ciAutomationSessionCanReceivePrompt`.
  `prepareOnTurnStartSession` and `persistWorkflowSwitchTag` stay on the
  marker, because they only set or clear it.
- Task state: `setSessionWaitingForInputWithoutTaskState` settles an errand in
  both completion paths. `runtimeSessionOwnsTaskState` is false for errands,
  and `settledSessionKeepsTaskState` makes `writeTaskReviewState` and
  `writeTaskReviewStateOnCancel` skip an errand. `otherWorkingSessionID`
  ignores errands, so a running errand cannot keep the task out of REVIEW.
  The two startup-reconciliation REVIEW writes skip errands.
- Primary and title: `createTaskSession` forces `IsPrimary=false` for errands,
  `setSessionPrimary` refuses them for all three promotion entry points, and
  `ClaimTaskTitleSession` refuses them in SQL on both dialects.
  `bestPrimarySessionCandidate`, which also serves `maybePromotePrimary` and
  `promoteNextPrimaryAfterRemoval`, and `promoteSessionIfTaskHasNoPrimary`
  skip errands.
- Routing: `GetActiveTaskSessionByTaskID` excludes errands. That covers
  `resolvePrimaryOrActiveSession` and about 15 workflow, PR, queue-promotion,
  child-completion, and recovery fallbacks.
  `ListActiveTaskSessionsByTaskID` still returns errands, so task-wide stops
  reach them. Also changed: `feederCandidateSession`, the
  `resolveMessageTargetSession` fallbacks, `previewCurrentSession`, and the
  `findExistingSession` newest fallback. `feederCandidateBlocked` and
  `validateMoveSessions` still block on a running errand.
- Unchanged by design: `parked_projection.go`, task activity aggregates, and
  the read-only `resolveConversationSession` fallback.

Tests: `task/models/workflow_detached_test.go`;
`orchestrator/errand_isolation_test.go` and
`errand_isolation_routing_test.go`, with ordinary and follow-up regression
variants and a flag on/off case; `task/service/errand_isolation_test.go`;
`mcp/handlers/message_target_session_errand_test.go`; and
`task/repository/sqlite/errand_isolation_test.go` with PostgreSQL variants.
Six mutation checks were each caught.

Verification passed: every command above with `GOTOOLCHAIN=go1.26.3`, the
lint script with 0 issues, `go run ./cmd/sqlguard ./internal`, Go 1.26.3
`gofmt -l`, `git diff --check`, `list-docs.py validate`, and
`lint-spec-files.py --all`. PostgreSQL variants skipped without
`KANDEV_TEST_POSTGRES_DSN`. `./internal/task/...`, `./internal/mcp/...`,
`./internal/orchestrator/...`, `./internal/persistence/...`, and
`./internal/office/service/...` show 95 failures, and all 95 fail identically
at `b6263ea4c3` (workspace path, relocation, archive, and Git tests).

Design edits: the predicate section names `persistWorkflowSwitchTag` and
`GetActiveTaskSessionByTaskID`, and the task-state row says "write or block".

For tasks 04 and 05:

- Insert errand sessions with `Role=models.SessionRoleErrand` and
  `ParentSessionID`; storage forces `IsPrimary=false`.
- `SetSessionPrimary*` returns `ErrErrandSessionPrimary` for an errand.
- Reach an errand only by explicit session ID; default lookups skip it.
- Errand turn completion records `not_required` and settles
  `WAITING_FOR_INPUT` without touching task state.
- Task 05 reuses `setSessionWaitingForInputWithoutTaskState` for the
  outstanding parent and calls `OnErrandTurnSettled` from
  `settleDetachedAgentReadyTurn`, the errand branch of
  `finishAgentCompletedTurn`, and `handleRecoverableFailureLockedState`.
