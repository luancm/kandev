---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-009
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
  - REQ-TASKS-ERRANDS-013
created: 2026-10-23
owners:
  - kandev
---

# Errands System Design

## Purpose and boundaries

The task system owns errand identity, workspace binding, primary selection,
and workflow isolation. It consumes the agent runtime, message queue
(`internal/orchestrator/messagequeue`), session ceiling, MCP tool profiles
([ADR-2026-08-08-mcp-tool-profiles](../../../decisions/2026-08-08-mcp-tool-profiles.md)),
the [bounded task summary](../../platform/requirements/bounded-task-status-delivery.md),
and [attach-only workspace reuse](additional-session-workspace-reuse.md).
Native subagent observation remains
[UI-owned](../../ui/requirements/subagent-observability.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-ERRANDS-001` | [Start](#start), [MCP contract](#mcp-contract), [Error codes](#error-codes) |
| `REQ-TASKS-ERRANDS-002` | [Status projection](#status-projection) |
| `REQ-TASKS-ERRANDS-003` | [Errand turn settlement and results](#errand-turn-settlement-and-results) |
| `REQ-TASKS-ERRANDS-004` | [Await](#await), [Parent turns and delivery](#parent-turns-and-delivery), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-ERRANDS-005` | [Workflow-detached predicate](#workflow-detached-predicate) |
| `REQ-TASKS-ERRANDS-006` | [Errand status block](#errand-status-block), [Parent turns and delivery](#parent-turns-and-delivery) |
| `REQ-TASKS-ERRANDS-007` | [Cancellation](#cancellation) |
| `REQ-TASKS-ERRANDS-008` | [Start](#start), [Queued admission](#queued-admission) |
| `REQ-TASKS-ERRANDS-009` | [MCP contract](#mcp-contract), [Security](#security), [Prompts](#prompts) |
| `REQ-TASKS-ERRANDS-010` | [Frontend](#frontend) |
| `REQ-TASKS-ERRANDS-011` | [Phone composition](#phone-composition) |
| `REQ-TASKS-ERRANDS-012` | [Release toggle](#release-toggle) |
| `REQ-TASKS-ERRANDS-013` | [Observability](#observability) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `internal/errands` (new) | `Service` owns errand lifecycle/delivery through narrow `Store`, `Launcher`, `ParentDeliverer`, `SessionStopper`, `ProfileResolver`, `EventPublisher`, and `Clock` interfaces. |
| `internal/task/repository/sqlite` | `task_sessions` columns, errand rows and `Store` on SQLite/PostgreSQL. |
| `internal/task/models` | `TaskSession.Role`, `TaskSession.ParentSessionID`, `SessionRoleErrand`, `Errand`, and `IsWorkflowDetachedSession`. |
| `internal/orchestrator` | `LaunchErrandSession`, `promptTask` composition, settlement/cancellation, ceiling drain, and detached-session guards. |
| `internal/orchestrator/executor` | Detached preparation and errand MCP mode resolution. |
| `internal/mcp/server` | Parent tools, fixed child catalog, await keepalive/release. |
| `internal/mcp/handlers` | `mcp.start_errand`, `mcp.await_errands`, `mcp.cancel_errand`, `mcp.await_errands_release`; errand guards in existing mutation handlers; shared workspace-conflict mapping. |
| `internal/sysprompt`, `config/prompts` | `errand.md`; flag-gated delegation and errand tool sections in `kandev-context.md`. |
| `internal/task/statussummary` | `ErrandsSummary` on `TaskStatusSummary`. |
| `apps/web` | Shared errand state, transcript/inline surfaces, navigation and row metadata. |

## Data and contracts

### Persistence

Idempotent `ADD COLUMN` migrations in `runMigrations()` add
`task_sessions.role` and `task_sessions.parent_session_id`
(`TEXT NOT NULL DEFAULT ''`), then the index
`idx_task_sessions_parent_session_id`; `base_schema.go` may list both for
fresh databases. Every session insert writes both, and no update changes
them. `role` is `''` or `errand`.

`task_session_errands` is created by `initErrandsSchema` and owned by the task
repository:

| Column | Meaning |
| --- | --- |
| `id` | Errand ID, equal to the reserved session ID. Primary key. |
| `task_id` | Owning task; foreign key with `ON DELETE CASCADE`. |
| `parent_session_id` | Parent session. |
| `title`, `objective` | Start arguments. The objective is the first prompt. |
| `agent_profile_id` | Resolved effective profile. |
| `mcp_server_ids` | JSON list: the parent's `task_session` MCP selections at start. |
| `first_turn_id`, `submission_id` | Reserved launch identity. |
| `lifecycle` | `queued`, `launched`, or `cancelled`. |
| `cancel_source` | `''`, `parent`, `user`, or `cascade`. |
| `last_outcome` | `''`, `done`, or `failed` for the latest parent-initiated turn. |
| `awaiting_result` | True from admission of a parent-initiated prompt until that turn settles or the errand ends. |
| `result_text`, `result_truncated`, `result_turn_id`, `result_kind` | Current result; `result_kind` is `output`, `failure`, `interrupted`, or `cancelled`. |
| `result_state` | `none`, `pending`, `delivered`, `superseded`, or `discarded`. |
| `delivered_route`, `delivery_token`, `delivered_at` | `await`, `turn_context`, or `notification`; the await or turn token; and the time. |
| `latest_settled_turn_id` | Latest settled turn of any origin, for **Send result to parent**. |
| `version`, `created_at`, `updated_at`, `settled_at` | Compare-and-set version and timestamps. |

Indexes: `(task_id, lifecycle)` for live counts and `(parent_session_id)` for
parent queries. No foreign key points to `task_sessions`, because a queued
errand has no session row yet. Deleting an errand session deletes its errand
row in the same transaction. Every errand state change is version-guarded;
PostgreSQL read-modify-write paths take a transaction-scoped advisory lock on
the task ID, per the backend schema rules.
`task_session_errands` joins the `task` descriptor's `RequiredTables` in
`internal/persistence/requiredstores/catalog.go`, with a `storeconformance`
scenario and explicit-tag upgrade fixture.

### Models and API projection

`models.TaskSession` gains `Role` and `ParentSessionID`. `TaskSession.ToAPI`,
`dto.TaskSessionDTO`, and `dto.TaskSessionSummaryDTO` project `role`,
`parent_session_id`, and, for parents, `waiting_on_errands`. Errand rows
project as `dto.ErrandDTO`: `errand_id`, `task_id`, `parent_session_id`,
`title`, `objective`, `agent_profile_id`, `status`, `result_state`,
`result_preview` (first 280 characters), `result_truncated`, `created_at`,
`updated_at`, `settled_at`, and `can_send_result`, the
[send-result rule](#errand-turn-settlement-and-results). Full result text is
returned only to the parent through MCP.

`statussummary.TaskStatusSummary` gains `Errands *ErrandsSummary`
(`live`, `waiting`), omitted when both are zero so rows without errands stay
byte-identical.

### Status projection

`errands.ProjectStatus(errand, session, permissionPending)` is a pure function:

| Condition, first match wins | Status |
| --- | --- |
| `lifecycle = cancelled` | `cancelled` |
| `lifecycle = queued` | `queued` |
| Session missing or `FAILED` | `failed` |
| `sessionstate.IsWorking(session.State)` and a permission request is pending | `needs_attention` |
| `sessionstate.IsWorking(session.State)` | `running` |
| `last_outcome` is `done` or `failed` | `last_outcome` |
| Otherwise | `running` (reconciliation settles it) |

Pending permission uses the signal behind the pending-action indicator.

### MCP contract

Parent tools, in the `errands` group of `profileToolGroups`, enabled for the
kanban surface with the new `CapabilityErrands`:

| Tool | Arguments | Result |
| --- | --- | --- |
| `start_errand_kandev` | `title`, `prompt`, optional `agent_profile` (ID or exact name) | `errand_id`, `session_id`, `status`, `agent_profile_id`, `agent_profile_name` |
| `await_errands_kandev` | `errand_ids` (1 to 5), `mode` (`all` or `any`), optional `max_wait_seconds` (0 to 600, default 600) | `errands[]`: `errand_id`, `status` (or `pending`), `result` and `truncated` when undelivered, `delivered_via` when already delivered |
| `cancel_errand_kandev` | `errand_id` | `errand_id`, `status` |

Inherited MCP servers add no Kandev tools, since `AC-TASKS-ERRANDS-009.2`
restricts only the Kandev surface.
`list_task_sessions_kandev` entries (`taskSessionListEntry`) gain `role`,
`parent_session_id`, and `errand_status`. Failures follow
[Error codes](#error-codes). The workspace mapping now inline in
`handleSpawnSession` moves to one shared helper used by both handlers, with
unchanged spawn output.

### Error codes

Each failure is a `ws.NewError` with a code from `pkg/websocket/actions.go`
and the reason in `details.reason`, as `handleSpawnSession` does.

| `details.reason` | Code | Raised when |
| --- | --- | --- |
| `feature_disabled` | `NOT_FOUND` | An errand MCP or WebSocket action runs with the flag off, as in background work |
| `errand_not_supported`, `errand_not_permitted`, `errand_sender_not_parent` | `FORBIDDEN` | In order: an ineligible start caller or task, errands included; another guarded mutation from an errand; another agent messaging an errand |
| `errand_profile_not_found`, `errand_profile_ambiguous`, `errand_profile_incompatible` | `VALIDATION_ERROR` | Profile resolution fails, passthrough included; `details.available` lists up to 20 names |
| `errand_not_found` | `NOT_FOUND` | An ID is not an errand of the caller (MCP) or task (WebSocket) |
| `errand_limit_reached` | `CONFLICT` | 5 are live; `details` adds `limit` and `live` |
| `errands_outstanding` | `CONFLICT` | An outstanding parent calls `step_complete_kandev`; no signal recorded |
| `errand_result_not_eligible` | `CONFLICT` | `errand.send_result` outside its eligibility rule |
| `workspace_preparing`, `workspace_reuse_unsafe`, `workspace_reuse_unsupported` | `CONFLICT` | Shared workspace mapping, with `details.recoverable` and `details.retry` |

Undecodable payloads are `BAD_REQUEST` and bad arguments `VALIDATION_ERROR`,
without a reason. A notification refused with `queue_full` keeps its results
pending.

### WebSocket contract

| Name | Direction | Payload |
| --- | --- | --- |
| `errand.list` | request | `task_id`; returns `errands[]` (`ErrandDTO`) |
| `errand.cancel` | request | `session_id` |
| `errand.cancel_all` | request | `task_id`; returns `cancelled`, the count |
| `errand.send_result` | request | `session_id` |
| `errand.updated` | event | one `ErrandDTO` plus `workspace_id`, routed with `Hub.BroadcastToWorkspaceOrDrop` |

The gateway scopes requests through `task_id` and `session_id`. A queued
errand has no session row: cancel authorization resolves its task from the
errand record and applies the same workspace membership check. Session
events, including cancel and recovery, carry `role`, `parent_session_id`,
and `waiting_on_errands`. Existing `session.removed` events reach authorized
workspace viewers and remove errands from each client's store; session-only
synthetic payloads resolve their task from local state.

Each result-bearing parent user message persists an `errand_delivery`
object: `token`, `route`, and `items` (`errand_id`, `title`, `status`,
`preview`, at most 280 characters). Direct prompts, queued prompts,
notifications, and preserved deletion notices use the same snapshot in
storage and live events. Keep `errand_delivery_token` for recovery readback.
Cards use this immutable snapshot after reload or errand deletion.

## Control flow

### Start

1. `handleStartErrand` checks the flag and verified caller, rejecting missing,
   errand, or passthrough callers and Office, ephemeral, configuration,
   coordinator, and automation tasks with `errand_not_supported`.
2. `Service.Start` serializes per task and enforces five live errands.
3. `ProfileResolver` matches ID, then exact name, defaulting to the caller's
   profile. Passthrough profiles are excluded; executor compatibility uses
   `ValidateAgentProfileForExecutor`. Profile guidance uses the same filter.
4. Reserve errand/session, turn, and submission IDs, snapshot the parent's
   session MCP selections, persist `queued`, publish, and call the launcher
   with `models.WithReservedLaunchIdentity`.
5. `LaunchErrandSession` uses `IntentStart`, `ProfileExplicit`, the objective,
   enabled snapshot definitions, and an internal `Errand` identity. Admission
   stores selections through `session_mcp.go`. `admitOrDeferSeam1` returns
   `ErrSessionCeilingRefused` without using the task's `deferred_launch` slot.
6. Preparation writes role/parent and `IsPrimary=false`; detached sessions
   cannot be promoted. `applyErrandOriginContext` frames the first prompt.
7. Success persists `launched`; ceiling refusal retains `queued`; synchronous
   workspace/profile failure deletes the reservation and returns its typed
   error. Events follow durable writes.

### Queued admission

`ceilingSweepTick` (`ceiling_sweep.go`) calls `errands.Service.DrainQueued`
after `drainDeferredCeilingLaunches` (`ceiling_replay.go`), so
`signalCeilingSweep` also drains errands. The drain walks queued errands by
task, then `created_at`, reuses each row's reserved identity and MCP
snapshot, and stops a task's walk at its first ceiling refusal. Other launch
failures set `last_outcome=failed` with a `failure` result.

### Errand turn settlement and results

In the agent-completed path of `event_handlers_agent.go`, the completion
follow-up branch becomes the workflow-detached branch. For an errand it
records the `not_required` delivery workflow outcome, settles the session to
`WAITING_FOR_INPUT` through a `setSessionWaitingForInput` variant that never
calls `writeTaskReviewState`, and calls `errands.Service.OnErrandTurnSettled`
with the session, turn, and outcome.
Failure paths (`handleRecoverableFailureLockedState`, terminal session
transitions) call the same entry point with a failed outcome.

A settled turn is parent-initiated when its ID equals `first_turn_id`, or when
its earliest user-role message carries `sender_session_id` equal to the
errand's parent; the MCP server fills that field from its own session, never
from tool arguments. For such a turn the service reads the turn's last agent
text message, cuts it to 16 KiB at a rune boundary, and writes
`last_outcome`, the result fields, and `result_state=pending` in one
version-guarded update. A still pending earlier result becomes `superseded`.
Capture is idempotent on `result_turn_id`. Every settled turn updates
`latest_settled_turn_id`. After the write commits, the service wakes local
waiters and runs the parent delivery check.

`errand.send_result` is allowed when the projected status is `done` or
`failed`, a settled turn exists, and `latest_settled_turn_id` differs from
`result_turn_id` or the current result is not pending or delivered
(`Errand.CanSendResult`). It captures that turn the same way.

### Await

The MCP handler sends `mcp.await_errands` with a random token and runs
`emitKeepAlivePings` at `askQuestionKeepAliveInterval`. The backend verifies
parent ownership, registers a waiter, and evaluates rows until the mode or
deadline is satisfied. Returned pending results are claimed transactionally
with route `await`. Request abandonment sends `mcp.await_errands_release`,
returning claims under that token to pending. Queued and `needs_attention`
errands do not end `all`. `EndParentAwaits` ends the parent's waits with
statuses only, without claiming results.

### Errand status block

`errands.Service.ComposeParentTurn` runs under the per-parent mutex. It
returns nothing with the flag off, for an errand or passthrough session, or
when no errand of the parent is live or holds a pending result. Otherwise it
returns a turn token and one trusted `<kandev-system>` block:

1. A header saying that pending results follow.
2. One line per such errand: `errand_id`, title and profile name cut to 120
   and 60 bytes at a character boundary, projected status, whole minutes
   since `created_at`, and `result: pending` or `result: none`. Live errands
   come first by creation, then the rest by settlement.
3. At most 10 lines, then a line counting omitted errands and naming
   `list_task_sessions_kandev`. This status part is at most 4 KiB.
4. Each pending result, framed as errand output rather than a user
   instruction, with errand ID, title, kind, and truncation note. Embedded
   values pass through `sysprompt.StripTags`, as in `wrapAgentMessage`.

The same version-guarded write marks each attached result `delivered` with
the token and route `notification` or `turn_context`.

### Parent turns and delivery

`promptTask` composes direct, queued, peer, workflow, CI, and recovery
prompts after workflow/plan composition and before provider admission.
Persist the delivery snapshot with the user message before admission;
a later best-effort token write is insufficient for restart readback.
Release claims on definite refusal. Fresh-launch retries retain the block
and report accepted, refused, or uncertain admission through the existing
`startCreatedSessionOptions` callbacks, as Send now does. Launch return
success alone is not acceptance; uncertainty must not cause duplicate replay.
Errand state never gates a parent prompt.

A result that becomes pending during a parent turn stays pending; only an
await in that turn can claim it. `deliverPending(parent)` runs when a result
becomes pending, from `OnParentTurnSettled`, and from the ceiling sweep:

1. It does nothing while another parent turn runs or any queue entry, a
   notification included, waits; the next turn takes the results.
2. `ParentDeliverer` queues one empty `QueuedByServer` entry with
   `errand_notification` metadata. Refusal leaves results pending.
3. Successful admission invokes existing queue readiness/drain logic:
   Auto-run on wakes an idle parent; off leaves the entry queued. Normal
   dispatch and Send now both retain notification identity, compose all
   pending results at dispatch, and consume an empty notification without
   a provider turn. A newer ordinary prompt keeps its visible text.
4. A competing parent turn withdraws the waiting notification through the
   session-scoped queue API after claiming its results.

Before `drainQueuedBeforeWorkflowTransition` in the agent-completed and
cancelled-turn completion paths, the orchestrator calls
`errands.Service.OnParentTurnSettled(parent, cause)`, which returns whether
the parent is outstanding:

1. With the flag off or no errands for the parent, it returns false.
2. Unless `cause` is a user cancel, it runs `deliverPending`. A user cancel
   leaves queued work parked, as
   [cancelled turn completion](../requirements/workflow-cancelled-turn-completion.md)
   requires, so its pending results wait for a later parent turn.
3. The existing drain starts the head entry when Auto-run is on.
4. If no turn started and the parent is outstanding, the orchestrator records
   the delivery workflow no-op with reason `errands_outstanding`, deferring a
   `cancel_triggers_turn_complete` cancel too, settles the parent to
   `WAITING_FOR_INPUT` without `writeTaskReviewState`, and returns before
   `processOnTurnCompleteViaEngineWithCause`.

A parent is outstanding while one of its errands has `lifecycle=queued`, has
`awaiting_result=true`, or has `result_state=pending`. The start flow sets
`awaiting_result`, and `handleMessageTask` sets it through
`errands.Service.OnParentPromptAdmitted` when the verified sender is the
errand's parent. Parent cancel and cascade clear it without a result. Every
other outstanding item clears only through a deliverable result that a later
parent turn takes, and that turn's completion evaluates the workflow.

`waiting_on_errands` is derived from durable rows: the parent is
`WAITING_FOR_INPUT` and outstanding. It is recomputed on every errand change
and parent state change, then projected on the session DTO and into
`ErrandsSummary.waiting`. `handleStepComplete` refuses a caller with
outstanding errands with `errands_outstanding` before recording any signal.

### Cancellation

| Trigger | Entry point | Effect |
| --- | --- | --- |
| `cancel_errand_kandev` | `errands.Service.Cancel`, source `parent` | `StopSession` with reason `errand_cancelled`, `lifecycle=cancelled`, no result |
| Errand view **Stop** or generic session stop on an errand | `errand.cancel`, or the session-stop hook, source `user` | Same, plus a `cancelled` result when outstanding |
| **Stop all errands** | `errand.cancel_all` to `Service.CancelAll`, source `user` | Each live errand as for **Stop**, under the per-task mutex, then one `deliverPending` per parent |
| Errand session deleted | Session delete hook | Errand row deleted; a `cancelled` result first when outstanding |
| Parent stopped, failed, or deleted | `StopSession`, terminal transitions, delete | All live errands of the parent cancelled with source `cascade`; pending results `discarded` |
| A parent turn is cancelled, including during an await | `CancelAgent` and `CancelTurn` call `EndParentAwaits` and the outstanding settlement guard | Only the turn ends; errands keep running, awaits claim nothing, workflow stays deferred |
| An errand turn is cancelled | Turn-bound settlement hook | Parent-origin interruption produces a failed result and clears `awaiting_result`; a user-origin turn restores the prior outcome; stale callbacks cannot settle a successor |
| Task archived or deleted | Archive batch cancellation and `ClearParkedProjectionOnSessionTerminated`; delete cascade | All errands cancelled or removed |

Cancellation is version-guarded and idempotent. Deletion with the flag off
never queues a notice or result; retained sessions remain workflow-detached.

## Workflow-detached predicate

`models.IsWorkflowDetachedSession(session)` is true for
`Role == SessionRoleErrand` or `IsCompletionFollowUpSession(session.Metadata)`.
Every orchestrator `IsCompletionFollowUpSession` call site
(`event_handlers_agent.go`, `event_handlers_workflow.go`,
`event_handlers_streaming.go`, `workflow_move_preview.go`,
`ci_automation_dispatch.go`) moves to it, except
`prepareOnTurnStartSession` and `persistWorkflowSwitchTag`, which only set or
clear the marker. These selectors also exclude errands:

| Path | Change |
| --- | --- |
| `processOnTurnStartViaEngineResult`, `processOnTurnCompleteViaEngineWithCause` | Return early for detached sessions |
| `runtimeSessionOwnsTaskState`, `writeTaskReviewState` callers | Errands never write or block task state |
| `bestPrimarySessionCandidate`, `promoteSessionIfTaskHasNoPrimary`, `promoteNextPrimaryAfterRemoval`, `Repository.SetSessionPrimary` | Never select or accept an errand |
| `GetActiveTaskSessionByTaskID`, `resolvePrimaryOrActiveSession`, `feederCandidateSession` | Skip errands |
| `resolveMessageTargetSession` fallback | Skip errands; explicit `session_id` still works |
| `selectReusableWorkflowSession`, `ciAutomationSessionCanReceivePrompt` | Skip errands |
| Generated-title ownership | Errands never claim it |

`validateMoveSessions` keeps counting running errands, so moves stay blocked
while an errand writes to the shared worktree.

## Prompts

`config/prompts/errand.md` supplies a trusted `<kandev-system>` block with
objective, final-message result contract, no destructive Git operations
(reset, checkout, stash, rebase, force), and depth/question limits. Parent
follow-ups use an errand `wrapAgentMessage` variant instead of a sibling
reply hint.

`kandev-context.md` uses `{delegation_policy_section}` and
`{errand_tools_section}`, populated through `KandevContextOptions` fields
`IncludeErrandTools` and `ErrandProfiles` (at most 20 names/agents/models).
Guidance and tool descriptions recommend start, end the turn, then continue
with delivered results; await remains an optional in-turn join. Without the
capability, golden tests require byte-identical legacy context.

## Security

`mcpmode.Errand` (`"errand"`) joins `instanceModes`, and `SurfaceErrand` joins
`normalizeSurface`, `modeForProfile`, `surfaceForMode`, and
`isFixedCatalogMode`. `registerErrandTools` composes the catalog from existing
registrations and keeps exactly the allowlist in `AC-TASKS-ERRANDS-009.2`,
following `registerAutomationTools`; a catalog test pins it, and plugin tools
are not registered. `resolveTaskSessionMCPMode` returns `McpModeErrand` for an
errand session right after the task load, so a profile or configuration mode
cannot widen it.

Backend guards check the verified sender's role in `handleCreateTask`,
`handleSpawnSession`, `handleCancelErrand`, `handleStepComplete`,
`handleMoveTask`, and `handleMessageTask` and refuse an errand with
`errand_not_permitted`; `handleStartErrand` refuses one with
`errand_not_supported`. `handleMessageTask` refuses a non-parent agent sender
targeting an errand with `errand_sender_not_parent`; user messages use the
session prompt path. Errand actions authorize by task. The residual
REST risk is in the ADR.

## Failure and recovery

- **Startup reconciliation.** After session recovery, `Service.Reconcile`
  fails `launched` rows whose session is missing or `FAILED`. A row whose
  session is `WAITING_FOR_INPUT` with a completed, uncaptured result turn is
  captured. A row whose parent-initiated turn was abandoned by restart or
  stall recovery becomes `failed` with an `interrupted` result. A turn or
  notification claim is reconciled against durable admission and message
  evidence, including direct prompts. Definite non-admission releases the
  claim; accepted or uncertain admission must not be blindly replayed.
  Pending results of terminal parents become `discarded`; then
  `deliverPending` runs for every idle parent.
- **Ordering.** Durable state is written before any local waiter wakes or any
  event publishes.
- **Races.** Capture and delivery are version-guarded. The per-parent mutex
  orders await claims, turn composition, and notification admission; the
  per-task mutex orders cap checks and **Stop all errands**.
- **Process loss.** Awaits end with the MCP connection; release
  returns their claims.
- **Replay.** The block is part of the admitted prompt, so a replayed
  submission carries the same results under the same token.

## Observability

Expvar counters, also emitted as structured zap logs:

| Counter | Label | Values |
| --- | --- | --- |
| `errands_started_total` | `outcome` | `started`, `queued`, `limit_reached`, `rejected`, `failed` |
| `errands_settled_total` | `status` | `done`, `failed`, `cancelled` |
| `errands_result_delivery_total` | `route` | `await`, `turn_context`, `notification` |

Logs `errand.started`, `errand.admitted`, `errand.settled`,
`errand.delivered`, `errand.cancelled`, and `errand.reconciled` carry task,
session, errand, and parent IDs as fields only.

## Frontend

- **State/transport.** Session projections carry role, parent and waiting
  fields. `lib/types/errands.ts`, the `errands` slice, `errand.updated` and
  `use-task-errands.ts` own the shared list/live state and reconnect refresh;
  `use-errand-actions.ts` owns mutations. Mounting multiple surfaces must
  retain one shared list request. Historical result cards use message
  snapshots, not today's rows.
- **Tabs.** `runAutoSessionTabEffect`, `useAutoSessionTab`, and live session
  adoption skip errands while enabled. Explicit Open focuses `session:<id>`
  without changing primary ownership. Menus hide Primary/Handoff by role,
  including while the feature is off.
- **Transcript.** `messages/kandev/errand-renderers.tsx` registers separate
  start rows and await lists. `message-renderer.tsx` renders one finished
  card per `errand_delivery` item below visible user text. Strip system
  framing; retain each historical message's identity and placement.
- **Errand view.** `errand-header.tsx` shows parent/back, objective, profile,
  status, workspace/branch, Stop while live or Send result when eligible.
  The existing composer supports follow-up and MCP selection.
- **Chip/panel.** `errands-chip.tsx` in `chat-status-bar.tsx` opens the
  existing task-wide grouped popover/Drawer with Open and Stop-all actions.
  `errands-panel.tsx` and `errands-panel-row.tsx` supply shared list content.
  Errand state never disables the parent's composer.
- **Task rows.** Board/sidebar/Threads consume `statusSummary.errands`;
  `thread-session-switcher.tsx` nests sessions beneath parents.
- **Copy/accessibility.** Use existing locale namespaces and seven shipped
  locales, keyboard-operable actions and one polite status announcer per view.

### Inline current-errands summary

`errands/errands-inline-summary.tsx` uses `@kandev/ui/collapsible` and the
bordered shell/count/action hierarchy of `TurnChangedFilesCard`. Todo
messages use a separate `ExpandableRow`; its clickable `div` and hover-only
chevron need semantic controls here. Use native React, not rich-output catalog
interactions.

`TaskChatPanel` supplies a new optional `transcriptTail` React-node prop via
`MessageListProps` in `message-list-shared.tsx`. `NativeMessageListBody` mounts
it between `TranscriptMessageItems` and `NativeMessageListFooter`, inside
`data-chat-content` and its existing resize/scroll owner. Preserve upstream
`detailActive` and reconciliation; no generic widget registry is needed.

Mount once in eligible parent chats while enabled and task rows exist; hide
in child chats and empty/off states. Default collapsed, with local expansion
keyed to task/session. Shared live selectors drive total/live/attention and
parent groups; Open and task-wide Stop all reuse existing handlers. Reuse
list/action content, not the overlay wrapper. Keep all groups visible within
that disclosure so bulk scope matches the chip. No new endpoint, polling or
per-surface loader. Starts, awaits, results and exact historical diff anchors
retain their existing renderers; never bind mutable state to an old turn.

### Phone composition

Sessions nest children; Open navigates to full-screen errand chat with visible
Back and actions. The chip retains its grouped bottom Drawer, one internal
scroll owner and fixed Stop-all footer. The inline summary stays full-width
inside the chat, stacks header/actions as needed, and uses the chat scroller
rather than another overlay or nested scroller. Visible disclosure/Open/bulk
controls have 44px phone/coarse-pointer targets; desktop stays compact.
Preserve bottom-follow when enabled and reading position when scrolled up.
Finished cards fit phone width and the parent composer remains enabled.

## Release toggle

`features.errands` (`KANDEV_FEATURES_ERRANDS`) is registered once in
`runtimeflags/registry.go` with `RestartRequired: true`, backed by
`FeaturesConfig.Errands`, `"false"` in every `profiles.yaml` profile, and
exposed as `errands` in `lib/state/slices/features/types.ts`; E2E specs
enable it with `backend.useEnv`. It gates `CapabilityErrands`, the errand
prompt sections, the MCP actions, the WebSocket requests,
`ComposeParentTurn`, `OnParentTurnSettled`, delivery, and the frontend errand
surfaces. Errand mode resolution, the workflow-detached predicate, and
reconciliation of cancellations do not read the flag, so existing errand
sessions stay isolated and fail closed.

## Verification strategy

Unit tests cover status projection, capture, supersession, status block
bounds, turn attachment, notifications, release, turn cancel, cap, profile
resolution, MCP inheritance, and the detached predicate at every listed call
site; a table test pins the `promptTask` callers. Repository tests cover
SQLite and env-gated PostgreSQL replay, MCP catalog tests both surfaces, and
Vitest the slice, handlers, tab filter, and renderers. Playwright desktop
and `mobile-*` scenarios use mock-agent `e2e:mcp:kandev:` scripts.

## Related decisions

- [ADR-2026-10-23-errands-first-class-delegation](../../../decisions/2026-10-23-errands-first-class-delegation.md)
- [ADR-2026-08-08-mcp-tool-profiles](../../../decisions/2026-08-08-mcp-tool-profiles.md)
- [ADR-2026-08-31-workflow-profile-session-switch-policy](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [ADR-2026-09-18-session-open-resumes-conversation](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
- [ADR-2026-08-02-single-owner-agent-task-titles](../../../decisions/2026-08-02-single-owner-agent-task-titles.md)
- [ADR-2026-08-01-separate-task-summary-session-stream-traffic](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)
- [ADR-2026-10-22-entry-bound-delivery-turns](../../../decisions/2026-10-22-entry-bound-delivery-turns.md)
