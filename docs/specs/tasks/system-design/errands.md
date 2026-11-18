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

| Requirement             | Design section                                                                                                          |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `REQ-TASKS-ERRANDS-001` | [Start](#start), [MCP contract](#mcp-contract), [Error codes](#error-codes)                                             |
| `REQ-TASKS-ERRANDS-002` | [Status projection](#status-projection)                                                                                 |
| `REQ-TASKS-ERRANDS-003` | [Errand turn settlement and results](#errand-turn-settlement-and-results)                                               |
| `REQ-TASKS-ERRANDS-004` | [Await](#await), [Parent turns and delivery](#parent-turns-and-delivery), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-ERRANDS-005` | [Workflow-detached predicate](#workflow-detached-predicate)                                                             |
| `REQ-TASKS-ERRANDS-006` | [Errand status block](#errand-status-block), [Parent turns and delivery](#parent-turns-and-delivery)                    |
| `REQ-TASKS-ERRANDS-007` | [Cancellation](#cancellation)                                                                                           |
| `REQ-TASKS-ERRANDS-008` | [Start](#start), [Queued admission](#queued-admission)                                                                  |
| `REQ-TASKS-ERRANDS-009` | [MCP contract](#mcp-contract), [Security](#security), [Prompts](#prompts)                                               |
| `REQ-TASKS-ERRANDS-010` | [Frontend](#frontend)                                                                                                   |
| `REQ-TASKS-ERRANDS-011` | [Phone composition](#phone-composition)                                                                                 |
| `REQ-TASKS-ERRANDS-012` | [Release toggle](#release-toggle)                                                                                       |
| `REQ-TASKS-ERRANDS-013` | [Observability](#observability)                                                                                         |

## Components and responsibilities

`internal/errands.Service` owns lifecycle and delivery through `Store`, `Launcher`, `ParentDeliverer`, `SessionStopper`, `ProfileResolver`, `EventPublisher` and `Clock`. The task repository owns schema and atomic decisions; task models own identity/projection; the orchestrator and executor own runtime admission, composition and detached guards. MCP server/handlers own authenticated tools, keepalive/release and mutation guards. Sysprompt/config own trusted context, statussummary owns bounded aggregates, and web owns state, navigation and presentation.

## Errands design map

These companions own the detailed algorithms; the contracts below remain shared entry points.

- [Recovery and delivery](errand-recovery-and-delivery.md): exact settlement, atomic claims, queue provenance, teardown and reserved launches.
- [Await receipts](errand-await-receipts.md): positive reception and retained uncertainty; [await duration](errand-await-duration.md): end reasons and elapsed duration.
- [Completion cleanup](errand-completion-cleanup.md): confirmed success-close and explicit retained-conversation resume.
- [Status and continuation](errand-status-and-continuation.md): attention, activity counts and continuation refusal.
- [History loading](errand-history-loading.md): browser reads, retention and reconnect ownership.
- [Conversation navigation](errand-conversation-navigation.md): host-scoped desktop/phone Open/Back and transcript history.
- [Context window](errand-context-window.md), [panel order](errand-panel-order.md) and [result duration](errand-result-duration.md): their respective projection and rendering contracts.

## Data and contracts

### Persistence

The [Errand ownership decision](../../../decisions/2026-11-17-errand-owned-conversations.md) defines the persistence boundary. The Errand holds its own conversation and executions. Its parent remains an ordinary task session.

`task_session_errands` is created by `initErrandsSchema` and owned by the task
repository:

- `id`: Errand ID. Primary key.
- `task_id`: Owning task; foreign key with `ON DELETE CASCADE`.
- `parent_session_id`: Parent session.
- `title`, `objective`: Start arguments. The objective is the first prompt.
- `agent_profile_id`: Resolved effective profile.
- `mcp_server_ids`: JSON object with the captured MCP additions and exclusions.
- `first_turn_id`, `submission_id`: Reserved launch identity.
- `lifecycle`: `queued`, `launched`, or `cancelled`.
- `cancel_source`: `''`, `parent`, `user`, or `cascade`.
- `last_outcome`: `''`, `done`, or `failed` for the latest parent-initiated turn.
- `awaiting_result`: True from admission of a parent-initiated prompt until that turn settles or the errand ends.
- `result_text`, `result_truncated`, `result_turn_id`, `result_kind`: Current result; `result_kind` is `output`, `failure`, `interrupted`, or `cancelled`.
- `result_state`: `none`, `pending`, `delivered`, `superseded`, or `discarded`.
- `delivered_route`, `delivery_token`, `delivered_at`: `await`, `turn_context`, or `notification`; the await or turn token; and the time.
- `latest_settled_turn_id`: Latest settled turn of any origin, for **Send result to parent**.
- `version`, `created_at`, `updated_at`, `settled_at`: Compare-and-set version and timestamps.

The lifecycle and parent indexes support per-task counts and per-parent reads. Errand rows are owned by the task foreign key. Errand-owned turns, messages and execution generations use the Errand ID. Version checks guard state changes, and PostgreSQL task-scoped writes use the existing advisory lock.

`task_session_errands` joins the `task` descriptor's `RequiredTables` in
`internal/persistence/requiredstores/catalog.go`, with a `storeconformance`
scenario and explicit-tag upgrade fixture.

### Models and API projection

Ordinary parent sessions project `waiting_on_errands`. Errand rows project as `dto.ErrandDTO`: `errand_id`, `task_id`, `parent_session_id`,
`title`, `objective`, `agent_profile_id`, `status`, `result_state`,
`result_preview` (first 280 characters), `result_truncated`, `created_at`,
`updated_at`, `settled_at`, and `can_send_result`, the
[send-result rule](#errand-turn-settlement-and-results). Full result text is
returned only to the parent through MCP. Optional `delivery_state` is
`uncertain` or `received` for applicable current-revision await evidence;
omission does not imply reception.

Task summary reads repair the bounded aggregate under the feature flag; CAS conflicts
re-observe the aggregate. An unavailable reader is unobserved, while a
disabled feature explicitly projects zero.

`statussummary.TaskStatusSummary` gains `Errands *ErrandsSummary`
(`live`, `waiting`), omitted when both are zero so rows without errands stay
byte-identical.

### Status projection

[Errand status and continuation](errand-status-and-continuation.md) owns the
shared evidence-aware projection, attention reasons, count parity and explicit
continuation refusal. Recovery attention confers no terminal authority.

### MCP contract

Parent tools, in the `errands` group of `profileToolGroups`, enabled for the
kanban surface with the new `CapabilityErrands`:

| Tool                   | Arguments                                                                                           | Result                                                                                                                                                                                                       |
| ---------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `start_errand_kandev`  | `title`, `prompt`, optional `agent_profile` (ID or exact name)                                      | `errand_id`, `session_id`, `status`, `agent_profile_id`, `agent_profile_name`                                                                                                                                |
| `await_errands_kandev` | `errand_ids` (1 to 5), `mode` (`all` or `any`), optional `max_wait_seconds` (0 to 600, default 600) | `errands[]`: `errand_id`, `status` (or `pending`), `result` and `truncated` when undelivered, `delivered_via` when already delivered; `duration_ms`, `end_reason`, and `await_delivery` correlation metadata |
| `cancel_errand_kandev` | `errand_id`                                                                                         | `errand_id`, `status`                                                                                                                                                                                        |

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

- `NOT_FOUND`: `feature_disabled` for gated MCP, legacy list or mutating WS actions; `errand_not_found` for an ID outside the caller's errands or requested task.
- `FORBIDDEN`: `errand_not_supported` for ineligible start callers/tasks; `errand_not_permitted` for guarded child mutations; `errand_sender_not_parent` for another agent messaging a child.
- `VALIDATION_ERROR`: `errand_profile_not_found`, `errand_profile_ambiguous` or `errand_profile_incompatible` (including passthrough); `details.available` lists at most 20 names.
- `CONFLICT`: `errand_limit_reached` at five live (with `details.limit`/`live`); `errands_outstanding` before step-completion signal recording; `errand_result_not_eligible` outside Send-result eligibility; `workspace_preparing`, `workspace_reuse_unsafe` or `workspace_reuse_unsupported` with `details.recoverable`/`retry` from the shared workspace mapping.

Undecodable payloads are `BAD_REQUEST` and bad arguments `VALIDATION_ERROR`,
without a reason. A notification refused with `queue_full` keeps its results
pending.

### WebSocket contract

| Name                     | Direction | Payload                                                                                                         |
| ------------------------ | --------- | --------------------------------------------------------------------------------------------------------------- |
| `errand.list`            | request   | `task_id`; complete `errands[]` (`ErrandDTO`)                                                                   |
| `errand.browser_summary` | request   | `task_id`; compact outstanding rows and authoritative task/parent counts                                        |
| `errand.history_page`    | request   | `task_id`, `parent_session_id`, `limit` (default/max 25), optional cursor; compact finished rows and pagination |
| `errand.inspect`         | request   | `task_id`, `child_session_id`; verified child/parent identity and compact errand, no transcript                 |
| `errand.cancel`          | request   | `session_id`                                                                                                    |
| `errand.cancel_all`      | request   | `task_id`; returns `cancelled`, the count                                                                       |
| `errand.send_result`     | request   | `session_id`                                                                                                    |
| `errand.updated`         | event     | one `ErrandDTO` plus `workspace_id`, routed with `Hub.BroadcastToWorkspaceOrDrop`                               |

The [bounded history design](errand-history-loading.md) owns browser reads,
guards and retention. These reads require workspace authorization, work with
the flag off, and never fall back to complete lists. API, internal and MCP
inventories remain complete; mutations retain feature and ownership gates.

The gateway authorizes Errand requests through their owning task. Cancellation resolves the Errand’s task from its durable row, including while queued. Errand updates refresh the owned browser state. The ordinary parent’s session events carry `waiting_on_errands`.

Each result-bearing parent user message persists an `errand_delivery`
object: `token`, `route`, and `items` (`errand_id`, `title`, `status`,
`preview`, at most 280 characters). Direct prompts, queued prompts,
notifications, and preserved deletion notices use the same snapshot in
storage and live events. Keep `errand_delivery_token` for recovery readback.
Cards use this immutable snapshot after reload or errand deletion.

Historical result timing follows [the elapsed-time contract](errand-result-duration.md).

## Control flow

### Start

`handleStartErrand` verifies the flag/caller and rejects missing, errand or passthrough callers and Office, ephemeral, configuration, coordinator and automation tasks. `Service.Start` serializes per task and caps live errands at five. Profiles resolve by ID, then exact name, defaulting to the caller; passthrough is excluded and executor compatibility uses `ValidateAgentProfileForExecutor`. Profile guidance uses the same filter.

Reserve the Errand, turn and submission identities and capture the parent’s MCP selections. The launcher resolves the exact attached task workspace, validates the owning task and parent, applies the ceiling and reserves the execution generation. It launches with an explicit Errand execution owner and the captured profile. The first prompt uses trusted Errand framing. A ceiling refusal keeps the reservation queued. A proved pre-admission failure can remove the initial reservation and return its typed error. [Reserved launch recovery](errand-recovery-and-delivery.md#reserved-launch-recovery) governs uncertain and adopted starts. Publish state after the durable writes.

### Queued admission

`ceilingSweepTick` drains errands after ordinary deferred ceiling launches; `signalCeilingSweep` also wakes it. Drain by task then creation time, reuse reserved identities/MCP selections, and stop that task at its first ceiling refusal. Definitive launch failure records `last_outcome=failed` and a failure result; [reserved launch recovery](errand-recovery-and-delivery.md#reserved-launch-recovery) governs uncertain/adopted launches.

### Errand turn settlement and results

[Exact settlement](errand-recovery-and-delivery.md#result-identity-and-settlement) owns durable terminal capture. [Completion cleanup](errand-completion-cleanup.md) then closes the captured execution and conditionally projects `COMPLETED` after acknowledged close or authenticated absence, without task REVIEW writes. `WAITING_FOR_INPUT` alone is not successful cleanup. Failed outcomes use the same exact settlement authority.

A turn is parent-initiated when it equals `first_turn_id` or its earliest user message has the authenticated parent as `sender_session_id`; MCP supplies that identity. Capture its last agent text, bounded to 16 KiB at a rune boundary, atomically with outcome/result fields and `result_state=pending`. An earlier pending result becomes superseded. Capture is idempotent on `result_turn_id`; every settled turn updates `latest_settled_turn_id`. Committed result/session writes wake waiters, including capture-before-idle ordering; result writes check parent delivery.

`Errand.CanSendResult` permits `errand.send_result` only for projected done/failed with a settled turn, where the latest settled turn differs from `result_turn_id` or the result is neither pending nor delivered. It captures that exact turn through the same authority.

### Await

Authenticated parent awaits use a random token and keepalive while evaluating owned rows until mode/deadline. [Atomic claims](errand-recovery-and-delivery.md#atomic-claims-and-await-attempt-outcome) and [reception receipts](errand-await-receipts.md) govern transactionally claimed results, offer and abort. Abandonment sends token-bound release intent but proves no non-reception: only a fenced proven abort releases eligible claims; uncertainty remains retained. Queued and needs-attention rows do not satisfy all-mode. `EndParentAwaits` ends parent waits with statuses only, without claiming results.

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

[Recovery and delivery](errand-recovery-and-delivery.md) owns prompt snapshots, claims, admission and queue provenance. Compose direct, queued, peer, workflow, CI and recovery prompts after workflow/plan context and before provider admission. Persist result-bearing user-message snapshots before admission; release only on definite refusal, retaining accepted/uncertain evidence through fresh-launch and Send Now paths. Errands never gate an otherwise admissible parent prompt.

Results becoming pending during a running turn wait for an in-turn await or later parent turn. Delivery checks run on new results, parent settlement and ceiling sweeps. With no running turn or waiting queue entry, queue one empty server-owned `errand_notification`; refusal leaves results pending. Auto-run controls dispatch. Compose at dispatch, consume stale empty notifications without a provider turn, preserve ordinary text, and withdraw a waiting notification when a competing turn claims its results. User turn cancellation keeps queued work parked.

Outstanding means queued, `awaiting_result=true` or `result_state=pending`. Start and verified parent prompt admission set awaiting-result; cancellation/cascade clears it without a result, otherwise outstanding work clears only through a deliverable result taken by a later parent turn, whose completion evaluates workflow. Outstanding or unreadable evidence defers workflow/review; a `WAITING_FOR_INPUT` outstanding parent projects `waiting_on_errands` in session DTO and summary. Recompute on errand/parent-state changes. `handleStepComplete` rejects outstanding work before recording a signal. Ordinary readable no-errand completion follows its existing workflow path.

### Cancellation

- Parent `cancel_errand_kandev` calls `Service.Cancel` with source `parent`: stop reason `errand_cancelled`, cancelled lifecycle, no result.
- User Stop (`errand.cancel` or generic session stop) uses source `user` and additionally captures a cancellation result when outstanding. Stop all calls `CancelAll` under per-task serialization, then checks delivery once per parent.
- Child deletion preserves an outstanding cancellation notice before deleting the errand row.
- Parent stop/failure/deletion cascades cancellation and discards pending results. Task archive/deletion cancels or removes children through existing archive/delete hooks.
- Parent turn cancellation (`CancelAgent`/`CancelTurn`) ends awaits without claims, leaves children running and defers outstanding workflow.
- Errand turn cancellation settles only the exact observed turn: parent-origin interruption fails its result and clears awaiting-result; user-origin interruption restores the prior outcome. Stale callbacks cannot settle a successor.

[Exact teardown](errand-recovery-and-delivery.md#teardown-and-lifecycle-recovery) and [prepared deletion notices](errand-recovery-and-delivery.md#prepared-deletion-notice-queue-is-the-row-independent-authority) own durable, version-guarded idempotence. Deletion while disabled queues no new notice; retained role isolation survives.

## Workflow-detached predicate

Workflow actions, primary selection, runtime task state and generated titles are ordinary session responsibilities. Errand executions use an explicit Errand owner and remain outside that selection and transition path. Workflow moves consult live Errand work before transferring the task’s shared workspace.

## Prompts

`config/prompts/errand.md` supplies a trusted `<kandev-system>` block with
objective, final-message result contract, no destructive Git operations
(reset, checkout, stash, rebase, force), and depth/question limits. Parent
follow-ups use an errand `wrapAgentMessage` variant instead of a sibling
reply hint.

Independent Errands return questions and blockers in their result to the parent. The host-owned agentctl instance identity refuses Codex native user-input callbacks and classified provider-native human-input permission requests before they create clarification or permission waiters. Ordinary task-session questions retain their existing path. The independent Errand helper capability requires this policy as well as MCP owner projection. Unsupported helpers fail admission. See [the independent ownership decision](../../../decisions/2026-11-17-errand-owned-conversations.md).

The transcript uses shared action controls. `errand.permission.respond` requires task control and exact Errand, request and pending IDs. Manual and saved automatic approvals check the offered choice and retained execution, harness and prompt submission before claiming the durable request. Finalize that claim against its original source. An unclaimed automatic proposal can become interactive if the request is still pending and its choice is unavailable. Publish transcript and parent attention updates.

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

[Recovery and delivery](errand-recovery-and-delivery.md) owns interrupted settlement, exact teardown, reserved launches and admission readback; [completion cleanup](errand-completion-cleanup.md) owns success-close retries, and [await receipts](errand-await-receipts.md) owns reception/uncertainty. Durable decisions precede publication and waiter wakeup. Capture/delivery races remain transaction/version guarded; per-parent serialization orders claims/composition/notifications and per-task serialization orders caps/Stop all. Missing evidence never authorizes replay or physical close. Admitted replay carries the same result block/token; terminal-parent results are discarded.

## Observability

Expvar counters, also emitted as structured zap logs:

| Counter                         | Label     | Values                                                     |
| ------------------------------- | --------- | ---------------------------------------------------------- |
| `errands_started_total`         | `outcome` | `started`, `queued`, `limit_reached`, `rejected`, `failed` |
| `errands_settled_total`         | `status`  | `done`, `failed`, `cancelled`                              |
| `errands_result_delivery_total` | `route`   | `await`, `turn_context`, `notification`                    |

Logs `errand.started`, `errand.admitted`, `errand.settled`,
`errand.delivered`, `errand.cancelled`, and `errand.reconciled` carry task,
session, errand, and parent IDs as fields only.

## Frontend

[History loading](errand-history-loading.md) owns summary/history/inspection state and reconnect freshness; [conversation navigation](errand-conversation-navigation.md) owns host-scoped selection and Back. Historical cards use immutable saved snapshots without child reads; complete inventories remain compatibility APIs. Preserve seven-locale copy, keyboard actions and one polite announcer per view, based on actual transcript/detail visibility independently of read acknowledgement.

### Current management and transcript history

Panels manage live work; transcripts retain history. [Conversation navigation](errand-conversation-navigation.md) preserves ordinary tabs, retained child links and runtime ownership. [Task29](../../../plans/errands-review-repairs/task-29-conversation-navigation-and-history.md) records evidence.

### Inline current errands summary

The former transcript inventory is superseded by [current management and transcript history](#current-management-and-transcript-history).

### Phone composition

[Phone composition](errand-conversation-navigation.md#phone-composition) owns the existing drawer/full-screen child/Back pattern, safe areas, one internal scroller, fixed Stop-all footer, 44px targets, no document overflow and preserved parent composer/reading position. Sessions contains normal conversations; the parent's drawer shows active errands then collapsed completed history. Selection/filtering is shared with desktop; mobile does not persist a substitute desktop layout.

## Release toggle

`features.errands` (`KANDEV_FEATURES_ERRANDS`) is registered once in
`runtimeflags/registry.go` with `RestartRequired: true`, backed by
`FeaturesConfig.Errands`, `"false"` in every `profiles.yaml` profile, and
exposed as `errands` in `lib/state/slices/features/types.ts`; E2E specs
enable it with `backend.useEnv`. It gates `CapabilityErrands`, the errand
prompt sections, the MCP actions, the legacy `errand.list` and mutating
WebSocket requests, `ComposeParentTurn`, `OnParentTurnSettled`, delivery,
and frontend errand management and mutation surfaces. Retained child
transcripts, parent Back navigation, and persisted-role restrictions remain
available while disabled. Errand mode resolution, the workflow-detached
predicate, and reconciliation of cancellations do not read the flag, so
existing errand sessions stay isolated and fail closed.

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

## Recovery and conformance repairs

The [repair package](../../../plans/errands-review-repairs/plan.md) records implementation and regression evidence for the companion authority map.
