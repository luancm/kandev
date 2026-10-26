---
id: "04-errand-launch-and-admission"
title: "Launch, admit, track, and cancel errands"
status: done
wave: 3
depends_on:
  - "01-flag-and-persistence"
  - "02-workflow-isolation"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-013
acceptance_criteria:
  - AC-TASKS-ERRANDS-001.1
  - AC-TASKS-ERRANDS-001.2
  - AC-TASKS-ERRANDS-001.3
  - AC-TASKS-ERRANDS-001.5
  - AC-TASKS-ERRANDS-001.6
  - AC-TASKS-ERRANDS-002.1
  - AC-TASKS-ERRANDS-002.2
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.2
  - AC-TASKS-ERRANDS-007.4
  - AC-TASKS-ERRANDS-007.5
  - AC-TASKS-ERRANDS-007.6
  - AC-TASKS-ERRANDS-008.1
  - AC-TASKS-ERRANDS-008.2
  - AC-TASKS-ERRANDS-008.3
  - AC-TASKS-ERRANDS-013.1
  - AC-TASKS-ERRANDS-013.2
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 04: Launch, admit, track, and cancel errands

## Summary

Create the `internal/errands` service and its orchestrator adapters so the
backend can start an errand session with an explicit profile, a reserved
identity, and a snapshot of its parent's session-level MCP servers, queue it
when the session ceiling refuses, project its status, publish
`errand.updated`, and cancel it singly, all at once, or by cascade. Agents
cannot reach this yet; tests drive the service and the `errand.list`,
`errand.cancel`, and `errand.cancel_all` WebSocket actions.

## In scope

- `internal/errands`: `Service` with `Start`, `DrainQueued`, `Cancel`,
  `CancelForParent`, `ProjectStatus`, `List`, and the narrow interfaces from
  the [design](../../specs/tasks/system-design/errands.md#components-and-responsibilities).
  `goleak.VerifyTestMain` in the package.
- Start flow steps 2 to 7 of the [design start section](../../specs/tasks/system-design/errands.md#start):
  per-task mutex and cap of 5, profile resolution by ID or exact name with
  passthrough and executor compatibility checks, reserved identity through
  `models.WithReservedLaunchIdentity`, row insert, and deletion on a
  synchronous workspace or profile failure.
- MCP inheritance: at start, read the parent's `task_session` MCP selections
  into the row's `mcp_server_ids`; at launch, pass the still-enabled
  definitions as `LaunchSessionRequest.MCPServerIDs` (nil when none), which
  `apps/backend/internal/orchestrator/session_mcp.go` stores as the errand's
  session scope after admission. Later parent changes never propagate.
- `orchestrator.LaunchErrandSession`, the internal `Errand` launch field,
  a new `ErrSessionCeilingRefused` sentinel (beside `ErrCeilingLaunchDeferred`
  in `ceiling_seams.go`) returned by `admitOrDeferSeam1` for errand launches,
  `prepareSessionAttempt` role, parent, and `IsPrimary=false`, and
  `applyErrandOriginContext` with a placeholder errand context until task 06
  supplies `errand.md`.
- `ceilingSweepTick` (`ceiling_sweep.go`) calls `DrainQueued` after
  `drainDeferredCeilingLaunches` (`ceiling_replay.go`); creation order per
  task, reusing the stored MCP snapshot; a non-ceiling launch failure fails
  the errand.
- Status projection table, including `needs_attention` from the pending
  permission signal.
- Cancellation entry points in the [design cancellation table](../../specs/tasks/system-design/errands.md#cancellation):
  `StopSession` with reason `errand_cancelled`, session-stop and delete
  hooks, `CancelAll` for **Stop all errands** under the per-task mutex,
  parent stop, failure, and delete cascade, task archive and delete, and the
  `CancelAgent` and `CancelTurn` hook that calls `EndParentAwaits` on the
  waiter registry interface (the registry arrives in task 05) and cancels no
  errand.
- `errand.list`, `errand.cancel`, and `errand.cancel_all` WebSocket actions in
  `internal/task/handlers`, `errand.updated` publication with
  `Hub.BroadcastToWorkspace`, and the error codes in the
  [design table](../../specs/tasks/system-design/errands.md#error-codes),
  including `feature_disabled` while the flag is off.
- `errands_started_total` and `errands_settled_total` with closed labels, and
  the structured logs for start, admission, settlement, and cancellation.
- Extract the typed workspace conflict mapping from `handleSpawnSession` into
  a shared helper with unchanged spawn output.

## Out of scope

- Result capture, await, the status block, delivery routes, parent workflow
  deferral, and reconciliation (task 05).
- MCP tools, errand mode, guards, and prompts (task 06).

## Acceptance

1. A started errand creates exactly one errand-role, non-primary session on
   the task's existing environment with the explicit profile, the reserved
   session ID, and the parent's session MCP selections as of the start; a
   later parent change does not reach it; cap, profile, and workspace
   failures create nothing.
2. At the session ceiling, errands queue, start in creation order with their
   stored snapshot when the sweep finds capacity, and never write the task's
   `deferred_launch` record.
3. Single, Stop-all, and cascade cancellation produce the projected statuses,
   keep transcripts, and are idempotent; cancelling a parent turn cancels no
   errand; counters use only the closed label sets.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -race -trimpath ./internal/errands/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Errand|Ceiling|SpawnOrigin|MCPServerSelection|CancelTurn' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/executor -run 'Errand|PrepareSession|ReservedLaunch' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/handlers -run 'Errand' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/handlers -run 'SpawnSession|WorkspaceConflict' -count=1)
(cd apps/backend && go test -trimpath ./internal/gateway/websocket -run 'DispatchScope|Errand' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- New `apps/backend/internal/errands/` (service, status, start, cancel,
  metrics, interfaces, tests).
- `apps/backend/internal/orchestrator/session_launch.go`, `session_mcp.go`
  (read side for the parent's selections), `ceiling_seams.go`,
  `ceiling_sweep.go`, `task_operations.go`, `cancel_turn.go`, and new errand
  adapter and test files.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`.
- New `apps/backend/internal/task/handlers/errand_ws_handlers.go`;
  `apps/backend/pkg/websocket/actions.go`.
- `apps/backend/internal/mcp/handlers/spawn_session.go` and a new shared
  workspace conflict helper.
- `apps/backend/internal/backendapp/` service wiring.

## Dependencies

Tasks 01 and 02.

## Risks

- Only automation retry uses `ReservedLaunchIdentity` today; prove the
  `IntentStart` path honors the reserved session, turn, and submission IDs.
- The ceiling seam is shared by every start; the errand-only refusal must
  not change other launches.
- PR [#4652](https://github.com/kdlbs/kandev/pull/4652) changes stop of
  replaced runtimes; rebase the errand cancel path if it lands first.
- PR [#4764](https://github.com/kdlbs/kandev/pull/4764) keeps the exact
  assigned profile through launch on the `ProfileExplicit` path; PR
  [#4765](https://github.com/kdlbs/kandev/pull/4765) guards helper-session
  deletion and archives helper history, which may change the errand-row
  deletion rule; PR [#4721](https://github.com/kdlbs/kandev/pull/4721) adds a
  shared turn allowance refusal on the errand prompt path. Re-check each.
- `MCPSelectionWriter.Replace` refuses disabled definitions, so the launcher
  must drop definitions disabled after the parent selected them.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/errands.md) start, queued
  admission, status projection, and cancellation sections.
- `automation_retry_launch.go`, `spawn_session.go`, and the ceiling tests.

## Results

Completed on 2026-10-26 in the primary session, test-first, in four code commits
(`7f3b7557d1`, `4b1f5bc3ca`, `8da167f445`, `680600ba1f`) and one documentation commit. No delegation.

- Service: `internal/errands` (`Service`, `Store`, `Launcher`, `SessionStopper`,
  `ProfileResolver`, `MCPSelections`, `ParentDeliverer`, `EventPublisher`,
  `Clock`; `goleak.VerifyTestMain`). `Start` mints the reserved identity,
  enforces the 5-live cap under a per-task mutex, inserts the queued row with
  `awaiting_result=true`, publishes `errand.updated`, then launches. Success,
  ceiling refusal (stays `queued`), and failure are each recorded with
  version-guarded writes; a launch that created no session deletes the row and
  returns the launcher's own error. `DrainQueued` walks queued rows by task and
  creation order with their stored identity and MCP snapshot, stops a task's
  walk at its first refusal, and fails a row on any other launch error with a
  `failure` result. `Cancel`, `CancelAll`, `CancelForParent`, `CancelForTask`,
  `CancelOnSessionStop`, `List`, `Lookup`, and `ProjectStatus` implement the
  cancellation table and the status projection (`needs_attention` from the
  pending permission signal). Cascades from session transitions and task
  archive run on a worker owned by the service (`StartWorkers`/`StopWorkers`).
  Counters `errands_started_total` and `errands_settled_total` use closed
  labels; the structured logs are `errand.started`, `errand.admitted`,
  `errand.settled`, and `errand.cancelled`.
- Orchestrator: `Service.LaunchErrandSession`, the `ErrandLaunch` field on
  `LaunchSessionRequest` and `startTaskOptions`, `ErrSessionCeilingRefused`,
  `ErrandAdapter` (launcher, stopper, MCP reader), `ErrandController` and
  `ParentAwaitEnder` seams, `SetMCPSelectionReader`, and
  `SelectionService.EnabledDefinitionIDs`. An errand start skips the deferred
  launch claim, task SCHEDULING, workflow step binding, and the launch-error
  card, honors the reserved session, turn, and submission IDs (proved through
  `startTask`), and gets a placeholder errand context block that
  `applyErrandOriginContext` whitelists; the completion-signal instruction is
  off. The executor stamps `Role`, `ParentSessionID`, and `IsPrimary=false`
  from `executor.WithErrandSession`, and an errand launch failure never writes
  task state.
- Hooks: `StopSession` (user stop before, parent cascade after), `DeleteSession`,
  terminal session transitions (both chokepoints), `CancelAgent`/`CancelTurn`
  (`EndParentAwaits` only), the ceiling sweep tick (`DrainQueued`), and task
  archive (`TaskErrandCanceller`).
- Transport: `errand.list`, `errand.cancel`, `errand.cancel_all`, and
  `errand.updated` (flat `ErrandDTO` plus `workspace_id`, routed with
  `BroadcastToWorkspaceOrDrop` and ordered with session state events). Every
  session created, waiting, state, cancelled, and recovered event builder adds
  `role` and `parent_session_id` for errand sessions through
  `models.AddSessionRoleFields`. `ErrandDTO.can_send_result` comes from
  `models.Errand.CanSendResult`. `workspaceConflictError` is shared by
  `handleSpawnSession` with unchanged output.

Verification passed (Go 1.26.3):

- `go test -race ./internal/errands/...`, the orchestrator `Errand|Ceiling|SpawnOrigin|MCPServerSelection|CancelTurn`
  run, the executor, `task/handlers`, `mcp/handlers`, and `gateway/websocket`
  runs from the list above.
- Mutation checks: removing the errand guards on task scheduling, launch-error
  clearing, and workflow step binding each failed the launch test.
- `bash scripts/lint-go-changed` over every Go file of the four commits (0
  issues), `gofmt -l`, `git diff --check`, `list-docs.py validate`, and
  `lint-spec-files.py --all`. No SQL changed, so `sqlguard` did not run.
- Wider runs of `./internal/orchestrator/...`, `./internal/task/...`,
  `./internal/mcp/...`, `./internal/gateway/...`, `./internal/agent/mcpconfig/...`,
  and `./internal/backendapp/...` show 109 failures. 108 fail identically at
  `b6263ea4c3` (workspace relocation, archive, checkout, Git, and terminal
  tests) and
  `TestExecutorHostGHBridgeNoninteractiveExecution` failed only under suite
  load and passed on every isolated run. PostgreSQL variants skip without
  `KANDEV_TEST_POSTGRES_DSN`; this work order adds no dialect-specific SQL.

Deviations and limits:

- `admitOrDeferSeam1` keeps its signature. `startTask` calls `admitStartSeam1`,
  which returns `ErrSessionCeilingRefused` for errand launches and delegates
  to the unchanged `admitOrDeferSeam1` otherwise, so the 6 existing callers
  and every other start are untouched.
- Task archive reaches errands through a task-level `TaskErrandCanceller`
  (queued errands have no session for `ClearParkedProjectionOnSessionTerminated`
  to see); task delete relies on the foreign-key cascade.
- A synchronous launch failure that created no session publishes a final
  `errand.updated` with status `failed` for the removed row, so clients drop
  the queued placeholder. If the failed launch left a session row, the errand
  stays as a `failed` errand and `Start` returns it.
- `errand.cancel` takes the errand ID in `session_id`. A queued errand has no
  session row, so with enforced auth the gateway backstop refuses it; **Stop
  all errands** (`task_id`) covers queued errands in that case.
- `CanSendResult` also requires a settled turn to exist, which the design
  rule implied.
- `DeleteSession` records a user cancellation (and a `cancelled` result when
  outstanding) before the row and its errand row are deleted together, so
  task 05 must deliver that notice before the delete commits or accept that
  the notice is lost with the row.
- The errand first-turn block is a placeholder string in
  `errand_launch.go`; task 06 replaces it with `errand.md`.
- Design edits: `can_send_result`, the `workspace_id` on `errand.updated`, the
  publish-before-launch ordering, and the settled-turn precondition are in the
  system design, tightened elsewhere to stay under the size limit.

For task 05:

- `ParentDeliverer.DeliverPending(ctx, parentSessionID)` is called after a
  user cancel, `Stop all errands`, and a failed launch, and is nil-safe.
  Implement it and `Service.SetDeliverer`.
- `ParentAwaitEnder.EndParentAwaits(parentSessionID)` is called by `CancelAgent`
  and `CancelTurn`; wire it with `Service.SetParentAwaitEnder`.
- Add `OnErrandTurnSettled`, `OnParentTurnSettled`, capture, and
  `OnParentPromptAdmitted` on `errands.Service`, using
  `Service.update` (compare-and-set), `IsOutstanding`, and
  `countSettled(done|failed)`; hook them at `settleDetachedAgentReadyTurn`, the
  errand branch of `finishAgentCompletedTurn`, and
  `handleRecoverableFailureLockedState`.
- `errand.send_result` is unregistered; reuse `Errand.CanSendResult` and the
  `errand.cancel` handler shape, with `errand_result_not_eligible`.

For task 06:

- `errands.Service.Start(ctx, StartRequest{TaskID, ParentSessionID, Title,
  Objective, Profile})` returns `StartResult{Snapshot, ProfileName}`; map
  errors with `errands.WSError` (typed reasons, `limit` and `live` details,
  profile `available` names) and `workspaceConflictError` for launcher errors.
- `Service.Cancel(CancelRequest{ErrandID, TaskID, ParentSessionID, Source:
  parent})` scopes by the verified caller session and task.
- Replace `errandOriginContent` with `errand.md`, return `McpModeErrand` from
  `resolveTaskSessionMCPMode`, and check the flag before calling the service.
