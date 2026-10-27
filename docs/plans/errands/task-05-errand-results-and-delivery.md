---
id: "05-errand-results-and-delivery"
title: "Capture errand results and deliver them to a conversational parent"
status: done
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

Complete. Status is `done`.

Commits on `feat/errands` for this work order:

- Capture, await, status block, claims, delivery, and `errand.send_result`:
  `8495fcea4f`, `861ffc2ca1`, `dfa2a14df5`.
- Settlement hooks, delete preservation, sweep, and prompt admission:
  `17cff71e64`, `3ee8a08c78`, `f36da82d7a`.
- Message-queue deliverer, `promptTask` composition, startup reconcile, and the
  parent delivery token: `9b69517f0f`, `9f73dc65b4`, `ff2f018ce3`, `2fe6f52917`,
  `7bf2214b96`.
- Final pass: `a56d89a5fd` (`waiting_on_errands` on session DTOs and events),
  `0c618ba3d5` (status block limits, startup reconcile, waiting events),
  `4a04ea529e` (idle parent prompts, workflow after settlement),
  `b3c4abd99b` (result delivery after a restart).

Delivered and tested (`internal/errands`, race-clean; orchestrator and DTO tests):

- `results.go`: parent-initiated capture (`first_turn_id` or verified sender),
  16 KiB rune-safe bound, idempotent on `result_turn_id`, supersede on a new
  parent prompt, `SendResult` gated by `Errand.CanSendResult`.
- `await.go`, `await_registry.go`: `all`/`any` waits, `max_wait_seconds` 0
  evaluates once, claim under the parent mutex, `ReleaseAwait`, `EndParentAwaits`.
- `compose.go`: status block (10 lines, 4 KiB, `StripTags`), claim with a
  token, `ReleaseTurnClaim`.
- `delivery.go`: `deliverPending` checks the flag and pending results, honours
  `NotificationBlocked`, and drops the parent lock before calling the deliverer;
  `OnParentTurnSettled`, `ParentOutstanding`, `ParentErrandState`, `SweepDelivery`,
  and `PreserveCancellationNotice`.
- Orchestrator wiring: settlement hooks on the completed, ready, failure, and
  cancelled paths; `ComposeParentTurn` in `promptTask`; `Service.Reconcile` at
  startup; `SetDeliverer`, `SetTurnReader`, `SetDeliveryLog`, and `SetEnabled`.
- `waiting_on_errands` (`*bool`, `omitempty`): projected on the full and summary
  session DTOs and on the `session.state_changed`, waiting, and created events.
  It is nil when the parent has no errand row. It is `true` when the parent is
  `WAITING_FOR_INPUT` with an outstanding errand.
- Metric `errands_result_delivery_total{route}` with closed labels.

Acceptance coverage added in this work order:

- AC-006.2, 006.3, 006.5 (orchestrator tests), and 006.6 (status block limits).
- AC-004.8 (startup reconcile wiring and restart delivery of a pending result).
- AC-004.10 and AC-004.11 annotated on existing orchestrator tests.
- AC-006.1 and AC-007.5 annotated on existing tests.

Known gaps (recorded, not fixed):

1. A direct-prompt turn-context claim is not re-delivered after a restart when
   the server crashes right after the turn.
2. The send-now path with a notification entry is not handled.
3. The 529 retry path keeps the status block: a transient 529 replays the raw
   cached prompt through `promptTask`, which recomposes the block. The block is
   dropped only on the `ErrExecutionNotFound` fresh-launch fallback in
   `handlePromptDispatchFailure`, which relaunches with the raw prompt. A fix
   that adds the block there would need a fallback acceptance marker first:
   `promptAccepted` is not set on that path, so the claim would be released
   after its block was delivered and results would arrive twice. Not small and
   safe, so not changed.

Not built (outside this work order, or not implemented):

- AC-006.4, the `errands_outstanding` refusal in step completion. It is not in
  the WO-05 frontmatter.
- Task service session publishers (`publishSessionsCancelled`,
  `publishSessionRecovered` in `internal/task/service/service_events.go`) do not
  project `waiting_on_errands`.
- Errand-session `CancelTurn` does not settle the errand turn (no outcome is
  defined for a user-cancelled errand turn); `awaiting_result` stays set until
  its next settled turn.

Deviations:

- `OnParentTurnSettled` takes a `SettleCause` (`turn_done` or `user_cancel`)
  instead of a free-form cause string.
- `PreserveCancellationNotice` admits the notice directly because the row is
  deleted afterward; it returns an error on refusal so the caller can keep the row.
- The `ErrandService` interface in `internal/task/handlers` gained `SendResult`,
  and its test fake was updated.
- An existing test that asserted `errand.send_result` is unregistered was
  flipped to assert it is registered.

Verification:

- `go test -race -trimpath ./internal/errands/... -count=1`: pass.
- `go test ./internal/orchestrator -run 'Errand|CompletionFollowUp|Clarification|DrainQueued|PromptTask|CancelledTurn'`: pass.
- `go test ./internal/orchestrator/messagequeue -run 'Errand|Admission|DeleteByID'`: pass.
- `go test ./internal/task/repository/sqlite -run Errand`: pass.
- `go test ./internal/task/handlers -run Errand`: pass.
- `go test ./internal/mcp/handlers -run 'MessageTask|Errand'`: pass.
- `go test ./internal/task/dto -run 'Errand|WaitingOnErrands'`: pass (no WaitingOnErrands tests yet).
- `go build ./...`: pass.
- `bash scripts/lint-go-changed <changed files>`: 0 issues.
- `gofmt -l` on changed files: clean.
- Final pass on HEAD `b3c4abd99b`, all passing: `go test -race -trimpath ./internal/errands/... -count=1`; `go test -trimpath ./internal/orchestrator -run 'Errand|CompletionFollowUp|Clarification|DrainQueued|PromptTask|CancelledTurn'`; `go test -trimpath ./internal/orchestrator/messagequeue -run 'Errand|Admission|DeleteByID'`; `go test -trimpath ./internal/task/repository/sqlite -run Errand`; `go test -trimpath ./internal/task/handlers -run Errand`; `go test -trimpath ./internal/mcp/handlers -run 'MessageTask|Errand'`; `go test -trimpath ./internal/task/dto -run 'Errand|WaitingOnErrands'`; `go build ./...`; `git diff --check` on `apps/backend`.
- Pre-existing failures on base `b6263ea4c3` (not caused by this work order, confirmed in a temporary worktree):
  `internal/task/handlers` `TestCursorMCPAuthenticateAfterCompletedTerminalReconnectStartsFreshLogin`, and 53 test names across `internal/orchestrator`, `internal/orchestrator/executor`, and `internal/task/service`. The failing-name list is identical between HEAD and base.
- `bash scripts/lint-go-changed <changed files>`: 0 issues on each commit's files.
- `gofmt -l` on changed files: clean.
- Task 05 is ticked in `docs/plans/errands/plan.md`.
