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

The task system owns task sessions, their workspace binding, primary
selection, and workflow turn hooks, so it owns errands. This design uses
without owning: the agent runtime, the message queue
(`internal/orchestrator/messagequeue`), the session ceiling, MCP tool
profiles
([ADR-2026-08-08-mcp-tool-profiles](../../../decisions/2026-08-08-mcp-tool-profiles.md)),
the [bounded task summary](../../platform/requirements/bounded-task-status-delivery.md),
and [attach-only workspace reuse](additional-session-workspace-reuse.md).
Native subagent observation stays with
[subagent observability](../../ui/requirements/subagent-observability.md),
unchanged.

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
| `internal/errands` (new) | `Service`: start, cap, profiles, status, results, status block, delivery, await, cancellation, queued admission, reconciliation, metrics. Depends only on narrow interfaces: `Store`, `Launcher`, `ParentDeliverer`, `SessionStopper`, `ProfileResolver`, `EventPublisher`, `Clock`. |
| `internal/task/repository/sqlite` | Columns on `task_sessions`, the `task_session_errands` table, and the `errands.Store` implementation, on SQLite and PostgreSQL. |
| `internal/task/models` | `TaskSession.Role`, `TaskSession.ParentSessionID`, `SessionRoleErrand`, `Errand`, and `IsWorkflowDetachedSession`. |
| `internal/orchestrator` | `LaunchErrandSession`, errand-aware session preparation, parent turn composition in `promptTask`, turn-settle, cancel, and stop hooks, ceiling-sweep hook, workflow-detached call sites. |
| `internal/orchestrator/executor` | Errand role and parent on the prepared session, `IsPrimary=false`, `McpModeErrand` from `resolveTaskSessionMCPMode`. |
| `internal/mcp/server` | `errands` tool group for the parent, fixed `errand` catalog, keepalive for await, await release on abandonment. |
| `internal/mcp/handlers` | `mcp.start_errand`, `mcp.await_errands`, `mcp.cancel_errand`, `mcp.await_errands_release`; errand guards in existing mutation handlers; shared workspace-conflict mapping. |
| `internal/sysprompt`, `config/prompts` | `errand.md`; flag-gated delegation and errand tool sections in `kandev-context.md`. |
| `internal/task/statussummary` | `ErrandsSummary` on `TaskStatusSummary`. |
| `apps/web` | Types, `errands` store slice, WebSocket handlers, hooks, transcript cards, errand header, chip and panel, tab filter, phone nesting, board and Threads indicators. |

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
`updated_at`, and `settled_at`. Full result text is returned only to the
parent through MCP.

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
| `errand.updated` | event | one `ErrandDTO`, routed with `Hub.BroadcastToWorkspace` |

The gateway backstop scopes all four requests through `task_id` and
`session_id`. Existing session events now carry `role`, `parent_session_id`,
and `waiting_on_errands`. A parent user message that delivers results carries
`errand_delivery` metadata: `token`, `route`, and `items` (`errand_id`,
`title`, `status`, and a preview of at most 280 characters).

## Control flow

### Start

1. `handleStartErrand` checks the flag, then resolves the verified caller
   session as `resolveSpawnerSession` does. It refuses a missing, errand, or
   passthrough (`IsPassthrough`) caller and Office, ephemeral,
   configuration, coordinator-origin, and automation-origin tasks with
   `errand_not_supported`.
2. `errands.Service.Start` takes a per-task mutex, counts live errands
   (`lifecycle != cancelled` and status live), and refuses beyond 5.
3. `ProfileResolver` matches an ID, then an exact name, refuses passthrough
   profiles (no structured assistant messages to capture), and checks
   executor compatibility with `ValidateAgentProfileForExecutor`, as task
   creation does. The default is the caller's `AgentProfileID`. The
   first-turn profile list applies the same filter.
4. The service mints `errand_id`, `first_turn_id`, and `submission_id`,
   snapshots the parent's `task_session` MCP selections into
   `mcp_server_ids`, inserts the row as `queued`, and calls
   `Launcher.LaunchErrand` with `models.WithReservedLaunchIdentity`, as in
   `automation_retry_launch.go`.
5. `orchestrator.LaunchErrandSession` calls `LaunchSession` with
   `Intent: IntentStart`, `ProfileExplicit: true`, the objective as the prompt,
   `MCPServerIDs` set to the snapshot's enabled definitions (nil when
   none), and an internal `Errand` field (`json:"-"`) with the errand ID,
   parent session, and title. `session_mcp.go` stores the IDs as the errand's
   session scope after admission. The field also makes `admitOrDeferSeam1`
   return a new `ErrSessionCeilingRefused` sentinel instead of writing the
   task's `deferred_launch` record and returning `ErrCeilingLaunchDeferred`.
6. `prepareSessionAttempt` sets `Role`, `ParentSessionID`, and
   `IsPrimary=false` on the new row. `promoteSessionIfTaskHasNoPrimary`
   ignores detached sessions. The first turn uses `applyErrandOriginContext`,
   the counterpart of `applySpawnOriginContext`.
7. Success moves the row to `launched`; a ceiling refusal leaves it `queued`;
   a synchronous workspace or profile failure deletes it and returns the
   typed error. `errand.updated` follows each durable change.

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
`failed` and `latest_settled_turn_id` differs from `result_turn_id` or the
current result is not pending or delivered. It captures that turn the same
way.

### Await

The MCP server handler sends `mcp.await_errands` with a random await token and
runs `emitKeepAlivePings` with `askQuestionKeepAliveInterval`, as
`askUserQuestionHandler` does. The backend checks that every ID is an errand
of the caller, registers a parent waiter, and loops: read rows, evaluate the
mode, or wait for a local notification or the deadline.
On return it marks each claimed `pending` result `delivered` with route
`await` and the token in one transaction, then responds. If the MCP request
context ends first, the server sends `mcp.await_errands_release` with the
token, returning rows still delivered under it to `pending`. Ceiling-queued
and `needs_attention` errands do not end an `all` wait. `EndParentAwaits`
ends every in-flight await of a parent: each returns statuses without result
text and claims nothing.

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

Every parent turn reaches `promptTask`: direct, queued, peer, workflow, CI,
and recovery prompts. For a parent session it calls `ComposeParentTurn` after
workflow and plan-mode composition and before delivery admission, prepends
the block, and records `errand_delivery` on the user message. A rejected
admission calls `ReleaseTurnClaim(token)`, returning rows still delivered
under that token to `pending`. No errand state gates a parent prompt.

A result that becomes pending during a parent turn stays pending; only an
await in that turn can claim it. `deliverPending(parent)` runs when a result
becomes pending, from `OnParentTurnSettled`, and from the ceiling sweep:

1. It does nothing while another parent turn runs or any queue entry, a
   notification included, waits; the next turn takes the results.
2. Otherwise `ParentDeliverer` admits one notification entry
   (`QueuedByServer`, empty content, metadata `errand_notification`). A
   refusal, `queue_full` included, leaves the results pending for the sweep.
3. With Auto-run on, the existing drain starts it at once; with Auto-run
   off, it waits. At dispatch, `ComposeParentTurn` carries every result then
   pending; an entry that finds none is consumed without a turn.
4. When another turn attaches results, the orchestrator deletes the waiting
   notification with `DeleteByIDForSession`.

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
| A parent turn is cancelled, including during an await | `CancelAgent` and `CancelTurn` call `EndParentAwaits` | Only the turn ends: no errand is cancelled and the await claims nothing |
| Task archived or deleted | Archive batch cancellation and `ClearParkedProjectionOnSessionTerminated`; delete cascade | All errands cancelled or removed |

Cancellation is a version-guarded update, so a repeat cancel succeeds without
change.

## Workflow-detached predicate

`models.IsWorkflowDetachedSession(session)` is true for
`Role == SessionRoleErrand` or `IsCompletionFollowUpSession(session.Metadata)`.
Every orchestrator `IsCompletionFollowUpSession` call site
(`event_handlers_agent.go`, `event_handlers_workflow.go`,
`event_handlers_streaming.go`, `workflow_move_preview.go`,
`ci_automation_dispatch.go`) moves to it, except
`prepareOnTurnStartSession`, which only marks follow-ups. These selectors also
exclude errands:

| Path | Change |
| --- | --- |
| `processOnTurnStartViaEngineResult`, `processOnTurnCompleteViaEngineWithCause` | Return early for detached sessions |
| `runtimeSessionOwnsTaskState`, `writeTaskReviewState` callers | Errands never write task state |
| `bestPrimarySessionCandidate`, `promoteSessionIfTaskHasNoPrimary`, `promoteNextPrimaryAfterRemoval`, `Repository.SetSessionPrimary` | Never select or accept an errand |
| `resolvePrimaryOrActiveSession`, `feederCandidateSession` | Skip errands |
| `resolveMessageTargetSession` fallback | Skip errands; explicit `session_id` still works |
| `selectReusableWorkflowSession`, `ciAutomationSessionCanReceivePrompt` | Skip errands |
| Generated-title ownership | Errands never claim it |

`validateMoveSessions` keeps counting running errands, so moves stay blocked
while an errand writes to the shared worktree.

## Prompts

`config/prompts/errand.md` holds the errand context: objective framing, the
result contract, shared-workspace caution (no reset, checkout, stash, rebase,
or force operations), and the depth and question limits. It is inserted as a
trusted `<kandev-system>` block. Parent follow-ups to an errand use an errand
variant of `wrapAgentMessage` that restates the result contract instead of
the sibling reply hint.

`kandev-context.md` gains `{delegation_policy_section}` and
`{errand_tools_section}`. `sysprompt.KandevContextOptions` gains
`IncludeErrandTools` and `ErrandProfiles` (name, agent, model; at most 20).
The errand tools section explains the status block and recommends starting
errands, ending the turn, and continuing when their results arrive with a
later turn; it reserves `await_errands_kandev` for results needed inside the
current turn. The tool descriptions agree: `start_errand_kandev` returns at
once, and `await_errands_kandev` is an optional in-turn join. Without the
capability the placeholders resolve to the current policy text and to empty,
so the rendered context stays byte-identical, which a golden test pins.

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
session prompt path. Errand actions authorize through the task, like other
session-keyed entry points. The residual REST risk is in the ADR.

## Failure and recovery

- **Startup reconciliation.** After session recovery, `Service.Reconcile`
  fails `launched` rows whose session is missing or `FAILED`. A row whose
  session is `WAITING_FOR_INPUT` with a completed, uncaptured result turn is
  captured. A row whose parent-initiated turn was abandoned by restart or
  stall recovery becomes `failed` with an `interrupted` result. A turn or
  notification delivery whose token is in no parent message returns to
  `pending`. Pending results of terminal parents become `discarded`; then
  `deliverPending` runs for every idle parent.
- **Ordering.** Durable state is written before any local waiter wakes or any
  event publishes, as the backend event-bus rule requires.
- **Races.** Capture and delivery are version-guarded. The per-parent mutex
  orders await claims, turn composition, and notification admission; the
  per-task mutex orders cap checks and **Stop all errands**.
- **Process loss during await.** Awaits end with the MCP connection, and
  release returns their claims for the next parent turn.
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

- **Types and state.** `TaskSession` (`lib/types/http.ts`) gains `role`,
  `parent_session_id`, and `waiting_on_errands`; `lib/types/errands.ts`
  defines `Errand`. A `lib/state/slices/errands` slice stores
  `byTaskId[taskId][errandId]` with selectors for task errands, errands by
  parent, live count, and errand by session.
- **Transport.** `lib/ws/handlers/errands.ts` applies `errand.updated`;
  `upsertTaskSessionFromEvent` keeps the new session fields.
  `hooks/domains/session/use-task-errands.ts` loads `errand.list` once per
  task while the flag is on; `use-errand-actions.ts` sends the other requests.
- **Tabs.** `runAutoSessionTabEffect` and `useAutoSessionTab` skip sessions
  with `role === "errand"` while the flag is on. Opening an errand adds or
  focuses its `session:<id>` panel without calling `session.set_primary`.
  `session-tab-menu.tsx` hides **Set as Primary** and **Handoff** for errands.
- **Transcript.** `components/task/chat/messages/kandev/errand-renderers.tsx`
  registers renderers for `start_errand_kandev` (one row card) and
  `await_errands_kandev` (a list card with live statuses) in `registry.ts`.
  `message-renderer.tsx` matches `errand_delivery` metadata and renders one
  **Errand finished** card per item (title, status, preview, **Open**) below
  any visible user text. The status block is stripped like any
  `<kandev-system>` content.
- **Errand view.** `components/task/chat/errands/errand-header.tsx` renders
  above the chat for errand sessions with the back link, objective, profile,
  status, workspace path and branch, and **Stop** while live or
  **Send result to parent** when eligible, never both.
- **Chip and panel.** `errands-chip.tsx` (labelled with the live count) and
  `errands-panel.tsx` mount in `chat-status-bar.tsx` only when the task has
  errands, following `background-work-chip.tsx`: popover on desktop. The
  panel footer shows **Stop all errands** while any errand is live.
- **Composer.** Errand state never disables the parent's composer. Errand
  views keep the composer MCP explorer, so an idle errand takes added servers
  like any session.
- **Task rows.** Kanban card, sidebar row, and Threads row components read
  `statusSummary.errands`. `thread-session-switcher.tsx` nests errands under
  their parent.
- **Copy.** Keys live in `task.json`, `chat.json`, `kanban.json`, and
  `threads.json` in all seven shipped locales. One `role="status"` region per
  view announces errand status changes.

### Phone composition

`mobile-sessions-section.tsx` nests errands under their parent with status.
Tapping one makes it the active session and renders `errand-header.tsx` in
its phone form with **Back to parent** and a visible actions menu that holds
**Stop** while live or **Send result to parent** when eligible. The chip
opens the panel in a bottom `Drawer`, as `background-work-chip.tsx` does on
phones, with the list as its only scroll owner and **Stop all errands** in a
fixed footer.
**Errand finished** cards use the shared renderer at full width. Touch
targets are at least 44px through the existing coarse-pointer variants.

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
