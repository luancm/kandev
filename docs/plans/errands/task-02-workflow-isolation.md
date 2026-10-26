---
id: "02-workflow-isolation"
title: "Detach errand sessions from workflow, primary, and routing"
status: pending
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

Pending.
