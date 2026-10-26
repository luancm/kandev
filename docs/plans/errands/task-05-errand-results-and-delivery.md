---
id: "05-errand-results-and-delivery"
title: "Capture errand results and deliver them to a conversational parent"
status: pending
wave: 4
depends_on:
  - "04-errand-launch-and-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-013
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.1
  - AC-TASKS-ERRANDS-003.2
  - AC-TASKS-ERRANDS-003.3
  - AC-TASKS-ERRANDS-003.4
  - AC-TASKS-ERRANDS-003.5
  - AC-TASKS-ERRANDS-004.2
  - AC-TASKS-ERRANDS-004.4
  - AC-TASKS-ERRANDS-004.5
  - AC-TASKS-ERRANDS-004.7
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
  - AC-TASKS-ERRANDS-004.11
  - AC-TASKS-ERRANDS-004.12
  - AC-TASKS-ERRANDS-006.1
  - AC-TASKS-ERRANDS-006.2
  - AC-TASKS-ERRANDS-006.3
  - AC-TASKS-ERRANDS-006.5
  - AC-TASKS-ERRANDS-006.6
  - AC-TASKS-ERRANDS-007.5
  - AC-TASKS-ERRANDS-013.1
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 05: Capture errand results and deliver them to a conversational parent

## Summary

Turn settled parent-initiated errand turns into bounded results and deliver
each result at most once, by the first of a backend await, the parent's next
turn (attached with the errand status block), or a notification turn. Keep
the parent an ordinary idle session while its errands run, defer its
workflow while errands are outstanding, and reconcile all of it after a
restart. The await is exposed as a backend action here; task 06 adds the MCP
tool around it.

## In scope

- `OnErrandTurnSettled` from the detached branch and failure paths: the
  parent-initiated test (`first_turn_id` or verified `sender_session_id`),
  16 KiB rune-safe capture, `superseded` handling, idempotency on
  `result_turn_id`, `latest_settled_turn_id`, and the cancellation notice
  for user cancel or delete.
- `awaiting_result` bookkeeping: set at start and through
  `OnParentPromptAdmitted`, called from `handleMessageTask` when the verified
  sender is the errand's parent; cleared when the turn settles or the errand
  ends. Outstanding is computed only from queued rows, `awaiting_result`, and
  pending results.
- `errand.send_result` WebSocket action, its eligibility rule, and
  `errand_result_not_eligible`.
- Await backend: waiter registry per parent, mode `all` and `any`, deadline
  from `max_wait_seconds`, claim to `delivered` with an await token, release
  by token, and `EndParentAwaits` for the task 04 cancel hook.
- `ComposeParentTurn` and the errand status block (at most 10 errands and
  4 KiB, byte-cut titles and profile names, `sysprompt.StripTags` on embedded
  values), called from `promptTask` for parent sessions before delivery
  admission, with `errand_delivery` metadata on the user message and
  `ReleaseTurnClaim` on rejected admission.
- `deliverPending`: one notification entry per idle parent with an empty
  queue (`QueuedByServer`, metadata `errand_notification`), composition at
  dispatch, consumption of an entry that finds nothing pending, withdrawal
  with `DeleteByIDForSession` when another turn takes the results, and sweep
  retry after a refusal.
- `OnParentTurnSettled(parent, cause)` before
  `drainQueuedBeforeWorkflowTransition` in the agent-completed and
  cancelled-turn completion paths: no notification after a user cancel, and
  the outstanding branch that records the delivery workflow no-op with reason
  `errands_outstanding` and settles the parent without `writeTaskReviewState`.
- Per-parent mutex and derived `waiting_on_errands` projected on the session
  DTO and session events.
- `Service.Reconcile` at startup as listed in the
  [design failure section](../../specs/tasks/system-design/errands.md#failure-and-recovery).
- `errands_result_delivery_total{route}` with `await`, `turn_context`, and
  `notification`, plus delivery and reconciliation logs.

## Out of scope

- The MCP tool wrappers, keepalive pings, and release call from the MCP
  server (task 06). The `step_complete_kandev` guard and prompt guidance
  (task 06). Row indicators (task 07). Rendering of **Errand finished** cards
  (task 08).

## Acceptance

1. Parent-initiated turns produce exactly one bounded result; user turns do
   not; a later parent turn supersedes an undelivered result.
2. Each result is delivered once, by the first of await, the next parent
   turn with its status block, or a notification turn that carries every
   result then pending; a result settling mid-turn is held; a released await
   or rejected admission makes results deliverable again; a full queue or a
   user cancel loses nothing.
3. A parent with outstanding errands stays an ordinary idle session that
   accepts prompts, skips workflow completion while outstanding, keeps the
   task in progress, and runs workflow normally on its first turn completing
   with nothing outstanding, including after a restart.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -race -trimpath ./internal/errands/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Errand|CompletionFollowUp|Clarification|DrainQueued|PromptTask|CancelledTurn' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/messagequeue -run 'Errand|Admission|DeleteByID' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'Errand' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/handlers -run 'Errand' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/handlers -run 'MessageTask|Errand' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/dto -run 'Errand|WaitingOnErrands' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/errands/` (results, status block, delivery, await,
  reconcile, metrics, tests).
- `apps/backend/internal/orchestrator/event_handlers_agent.go`,
  `event_handlers_streaming.go`, `task_operations.go` (`promptTask`
  composition), `cancel_turn.go` and the cancelled-turn completion path,
  `agent_delivery_effect.go`, `ceiling_sweep.go`, and new errand settle,
  composition, notification, and turn-cancel tests.
- `apps/backend/internal/task/handlers/errand_ws_handlers.go`.
- `apps/backend/internal/mcp/handlers/handlers.go` (`handleMessageTask` hook).
- `apps/backend/internal/task/dto/dto.go`, `apps/backend/pkg/websocket/actions.go`.
- `apps/backend/internal/backendapp/` startup reconciliation wiring.

## Dependencies

Task 04.

## Risks

- Every parent prompt path must reach `promptTask`; a table test pins its
  callers so a new path cannot skip the status block.
- The turn claim and the user message are separate writes; reconciliation
  must return claims whose token is in no parent message. PR
  [#4720](https://github.com/kdlbs/kandev/pull/4720) may provide a durable
  readback to use instead.
- PR [#4721](https://github.com/kdlbs/kandev/pull/4721) adds a shared turn
  allowance refusal on the prompt path; a refused notification or turn must
  release its claims. PR [#4765](https://github.com/kdlbs/kandev/pull/4765)
  may change deletion of terminal helper sessions with queued work, which
  affects notification entries on a deleted parent.
- Event-bus publication is not an acknowledgement; wake waiters only after
  the durable write, and test with an asynchronous bus fake.
- The outstanding branch must not record an outcome for a turn that the
  drain already handed to a queued message.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/errands.md) results, await,
  status block, parent turns and delivery, and failure sections.
- [Cancelled turn completion](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md)
  for the parked queue after a user cancel.
- `apps/backend/AGENTS.md` event-bus and goroutine ownership rules.

## Results

Pending.
