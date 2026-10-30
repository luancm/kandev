---
status: draft
system: tasks
created: 2026-10-23
owners:
  - kandev
---

# Errands Requirements

## Overview

An errand is a visible, steerable child session started by a task's agent.
It uses the task's shared workspace, has its own transcript, profile, and
follow-up input, and returns results to a conversational parent. The task
system owns its identity, workspace binding, workflow isolation, and parent
relationship. The rationale and boundaries are in
[ADR-2026-10-23-errands-first-class-delegation](../../../decisions/2026-10-23-errands-first-class-delegation.md).

## Terminology

- **Parent session:** The session whose agent started the errand.
- **Parent-initiated turn:** An errand turn started by its objective or a
  parent message.
- **Result:** A parent-initiated turn's outcome or a cancellation notice.
- **Live errand:** Status `queued`, `running`, or `needs_attention`.
- **Outstanding:** A parent has a queued errand, an in-flight parent-initiated
  errand turn, or an undelivered result. User-initiated turns do not count.
- **Errand status block:** A server-authored summary of in-flight errands
  in a parent turn's context.
- **Notification:** A server-queued message delivering all results pending
  when its parent turn starts.

## Requirements

### REQ-TASKS-ERRANDS-001: Errand start

**Intent:** Let an agent delegate to a chosen profile without blocking its turn.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-001.1:** When an eligible task session starts an errand
  with a title and an objective, the system shall create a child session of
  the same task whose parent is the caller, and shall return the errand ID,
  which equals its session ID, and its status without waiting for the errand's
  turn.
- **AC-TASKS-ERRANDS-001.2:** The errand shall use exactly the requested agent
  profile, given as an ID or an exact profile name, or the caller's profile
  when none is given. A workflow step profile shall not override it, and the
  response shall report the effective profile.
- **AC-TASKS-ERRANDS-001.3:** An errand shall attach to the task's existing
  workspace under the attach-only contract of
  [additional session workspace reuse](additional-session-workspace-reuse.md).
  A workspace that is preparing, unsafe, or unsupported shall fail the start
  with the existing typed reason and create no errand.
- **AC-TASKS-ERRANDS-001.4:** A start from an errand, from a passthrough
  terminal session, or from a session of an Office, quick chat,
  configuration, automation, or coordinator task, shall fail with
  `errand_not_supported` and create no session.
- **AC-TASKS-ERRANDS-001.5:** An unknown, ambiguous, passthrough, or
  executor-incompatible profile shall fail with `errand_profile_not_found`,
  `errand_profile_ambiguous`, or `errand_profile_incompatible`, list at most
  20 eligible profile names, and create no errand.
- **AC-TASKS-ERRANDS-001.6:** An errand shall start with a snapshot of its
  parent's session-level MCP server selections, added to its own effective
  set under REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003. Later parent changes
  shall not reach it; the user can add servers from the errand's composer.

### REQ-TASKS-ERRANDS-002: Errand status

**Intent:** Expose one consistent errand status.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-002.1:** An errand shall report exactly one status:
  `queued` (waiting for session capacity, no runtime), `running` (starting or
  executing a turn), `needs_attention` (running with a permission request
  waiting for the user), `done` (its latest parent-initiated turn completed),
  `failed` (launch failed, its latest parent-initiated turn ended in error or
  interruption, or its session failed), or `cancelled`.
- **AC-TASKS-ERRANDS-002.2:** While a user-initiated errand turn is active,
  the errand shall report `running` or `needs_attention`. When that turn
  settles, the errand shall return to its previous settled status.
- **AC-TASKS-ERRANDS-002.3:** The parent agent's session listing shall report
  each session's role, parent session, and errand status.

### REQ-TASKS-ERRANDS-003: Errand results

**Intent:** Capture one bounded outcome per delegated turn.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-003.1:** When a parent-initiated turn completes, the
  system shall record one result containing the errand's final assistant
  message for that turn. When it fails, the result shall contain a sanitized
  failure summary.
- **AC-TASKS-ERRANDS-003.2:** A result shall be at most 16 KiB of UTF-8 text,
  cut at a character boundary, and shall state when it was truncated and that
  the full transcript is readable with `get_task_conversation_kandev`.
- **AC-TASKS-ERRANDS-003.3:** A user-initiated turn shall not produce a
  result. The errand view shall offer **Send result to parent**, which records
  the final assistant message of the errand's latest settled turn as a result.
- **AC-TASKS-ERRANDS-003.4:** When a new parent-initiated turn starts while an
  earlier result is undelivered, the earlier result shall be superseded and
  never delivered.
- **AC-TASKS-ERRANDS-003.5:** When the user cancels or deletes an errand while
  it is outstanding, the parent shall receive a cancellation notice as a
  result. A cancellation requested by the parent or caused by a cascade shall
  produce no result.

### REQ-TASKS-ERRANDS-004: Result delivery

**Intent:** Deliver each result once, when the parent can use it.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-004.1:** Superseded by criterion 004.9.
- **AC-TASKS-ERRANDS-004.2:** An await call names errands and a mode. Mode
  `all` shall return when every named errand is settled; mode `any` shall
  return when at least one named errand has an undelivered result or all are
  settled. The response shall report every named errand's status and include
  the text of each undelivered result.
- **AC-TASKS-ERRANDS-004.3:** An await call shall wait at most 600 seconds, or
  a shorter `max_wait_seconds` from 0 to 600. When the wait ends first, the
  response shall mark unsettled errands `pending`. The call shall send
  periodic progress notifications while it waits.
- **AC-TASKS-ERRANDS-004.4:** When an await call is abandoned before its
  response reaches the parent, the results it claimed shall become deliverable
  again.
- **AC-TASKS-ERRANDS-004.5:** An await for an errand whose result was already
  delivered shall report its status and delivery route without repeating the
  text.
- **AC-TASKS-ERRANDS-004.6:** Superseded by criteria 004.10 and 004.11.
- **AC-TASKS-ERRANDS-004.7:** When the parent's queue is full, results shall
  remain pending and be retried without loss.
- **AC-TASKS-ERRANDS-004.8:** After a backend restart, undelivered results
  shall still be delivered. An errand whose parent-initiated turn was
  interrupted shall become `failed` and produce an interruption result.
- **AC-TASKS-ERRANDS-004.9:** Each result shall be delivered at most once, by
  the first of: an await call in progress that names its errand, the parent's
  next turn, or a notification.
- **AC-TASKS-ERRANDS-004.10:** When a parent turn starts with results
  pending, whatever starts it, every pending result shall be attached to its
  context with the errand status block instead of a separate message.
- **AC-TASKS-ERRANDS-004.11:** When a result is pending, the parent is idle,
  and no other entry waits in its queue, the system shall queue one
  notification, and no second one while it waits. With Auto-run on, it shall
  start the parent's next turn at once; with Auto-run off, it shall wait in
  the queue. Its turn shall carry every result then pending. When another
  parent turn takes those results first, the notification shall be removed
  without starting a turn.
- **AC-TASKS-ERRANDS-004.12:** A result that becomes pending while a parent
  turn is running shall not enter that turn unless an await call in that turn
  returns it.

### REQ-TASKS-ERRANDS-005: Workflow and ownership isolation

**Intent:** Keep workflow, task state, and routing with parents and peers.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-005.1:** An errand turn shall not run workflow turn
  actions, change task state, record step completion, or complete a step.
- **AC-TASKS-ERRANDS-005.2:** An errand shall never become primary, at
  creation or by automatic promotion, and shall never own the task title.
- **AC-TASKS-ERRANDS-005.3:** An errand shall never be the default or fallback
  target of task messages, workflow prompts, CI or pull-request automation,
  queue feeding, or workflow profile-session reuse.
- **AC-TASKS-ERRANDS-005.4:** A running errand shall block a workflow move the
  same way any running session does.
- **AC-TASKS-ERRANDS-005.5:** Opening, messaging, or stopping an errand shall
  not transfer workflow ownership. Session menus shall not offer
  **Set as Primary** or **Handoff** for an errand.

### REQ-TASKS-ERRANDS-006: Parent session while errands run

**Intent:** Keep parents conversational and defer workflow for outstanding errands.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-006.1:** When a parent turn completes while the parent
  has outstanding errands, the system shall skip that turn's workflow
  completion actions and task review-state change, and the task shall keep its
  in-progress state.
- **AC-TASKS-ERRANDS-006.2:** The first parent turn that completes with
  nothing outstanding shall run the workflow completion actions normally.
- **AC-TASKS-ERRANDS-006.3:** While the parent is idle with outstanding
  errands, the parent session and its task shall report that they are waiting
  on errands. The report shall clear when nothing is outstanding or the parent
  starts a turn.
- **AC-TASKS-ERRANDS-006.4:** A step completion signal from a parent with
  outstanding errands shall fail with `errands_outstanding` and record no
  signal.
- **AC-TASKS-ERRANDS-006.5:** A parent with outstanding errands and no
  running turn shall be an ordinary idle (`WAITING_FOR_INPUT`) session. It
  shall accept user prompts and agent messages as any idle session does, and a
  turn started this way can start, message, cancel, and await errands.
- **AC-TASKS-ERRANDS-006.6:** Every parent turn that starts while the parent
  has an errand that is `queued`, `running`, or `needs_attention`, or that
  holds an undelivered result, shall receive one server-authored errand status
  block. The block shall list each such errand's ID, title, profile, status,
  time since creation, and whether a result is pending; list at most 10
  errands and count the rest; and be at most 4 KiB. Other turns, and all
  turns while the flag is off, shall receive none.

### REQ-TASKS-ERRANDS-007: Cancellation

**Intent:** Cancel delegated work predictably.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-007.1:** The parent agent's cancel tool and the errand
  view's **Stop** shall stop one errand's runtime, set it `cancelled`, and
  keep its transcript. Cancelling an already cancelled errand shall succeed
  without change.
- **AC-TASKS-ERRANDS-007.2:** When the parent session stops, fails, or is
  deleted, its live errands shall be cancelled and its undelivered results
  discarded.
- **AC-TASKS-ERRANDS-007.3:** Superseded by criterion 007.5.
- **AC-TASKS-ERRANDS-007.4:** Archiving or deleting the task shall cancel or
  remove all of its errands, including queued ones.
- **AC-TASKS-ERRANDS-007.5:** Cancelling a parent turn, including one waiting
  in an await call, shall cancel only that turn. The parent's errands shall
  keep running, the await shall claim no result, and pending results shall
  wait for a later parent turn; the cancellation itself shall queue no
  notification.
- **AC-TASKS-ERRANDS-007.6:** **Stop all errands** shall cancel every live
  errand of the task as **Stop** does for one, and each parent shall receive
  the resulting notices in one delivery. With no live errand, it shall
  succeed without change.

### REQ-TASKS-ERRANDS-008: Limits and admission

**Intent:** Bound fan-out and respect install capacity.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-008.1:** A task shall hold at most 5 live errands. A
  start beyond that shall fail with `errand_limit_reached`, report the limit
  and live count, and create no errand.
- **AC-TASKS-ERRANDS-008.2:** When the session ceiling refuses an errand
  start, the errand shall be `queued` and shall start automatically when
  capacity frees, in creation order within its task, without affecting other
  deferred launches of the task.
- **AC-TASKS-ERRANDS-008.3:** A queued errand shall be awaitable and
  cancellable. A queued errand that later fails to launch shall become
  `failed` and produce a result.

### REQ-TASKS-ERRANDS-009: Agent tool surface and delegation policy

**Intent:** Expose parent delegation tools and a restricted errand tool surface.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-009.1:** Eligible task sessions shall have
  `start_errand_kandev`, `await_errands_kandev`, and `cancel_errand_kandev`.
  Follow-ups to an errand shall use `message_task_kandev` with its session ID.
- **AC-TASKS-ERRANDS-009.2:** An errand shall receive only these Kandev tools:
  `list_workspaces_kandev`, `list_workflows_kandev`,
  `list_workflow_steps_kandev`, `list_tasks_kandev`,
  `list_related_tasks_kandev`, `list_task_sessions_kandev`,
  `get_task_conversation_kandev`, and `get_task_plan_kandev`. The backend
  shall refuse any task, session, errand, step-completion, move, or message
  mutation whose verified sender is an errand with `errand_not_permitted`.
- **AC-TASKS-ERRANDS-009.3:** A message to an errand from any agent session
  other than its parent shall fail with `errand_sender_not_parent`. The user
  can always message an errand.
- **AC-TASKS-ERRANDS-009.4:** An errand's first turn shall state its
  objective, that its final message in each parent-initiated turn is its
  result, that it shares the workspace and must not run destructive Git
  operations, and that it cannot ask the user or start other work.
- **AC-TASKS-ERRANDS-009.5:** An eligible parent's first-turn context shall
  list at most 20 profiles compatible with the task's executor, each with name,
  agent, and model. It shall state that errands are the default way to
  delegate, and that native subagents are allowed only for quick read-only
  lookups that finish within the current turn.
- **AC-TASKS-ERRANDS-009.6:** The parent's errand guidance and the errand tool
  descriptions shall recommend ending the turn after starting errands, and
  shall reserve `await_errands_kandev` for results needed inside the current
  turn.

### REQ-TASKS-ERRANDS-010: Desktop task surfaces

**Intent:** Show errands in existing task surfaces.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-010.1:** Superseded by criterion 010.7.
- **AC-TASKS-ERRANDS-010.2:** Errands shall not be added as session tabs
  automatically. **Open** shall open or focus the errand's session view and
  shall not change the primary session or selected workflow owner.
- **AC-TASKS-ERRANDS-010.3:** The errand view shall show the full session
  transcript and composer, plus a header with a link back to the parent,
  title, objective, profile, status, shared workspace path and branch,
  **Stop** while live, and **Send result to parent** when eligible.
- **AC-TASKS-ERRANDS-010.4:** The chat status bar shall show an errands chip
  only when the task has at least one errand. The chip shall open a panel
  that groups live and completed errands by parent session with status and
  **Open**, and shall remain available after all errands settle.
- **AC-TASKS-ERRANDS-010.5:** Board cards, sidebar rows, and Threads rows
  shall show the live errand count and a waiting-on-errands status from the
  bounded task status summary, and shall show nothing when there are none.
  The Threads session switcher shall nest errands under their parent.
- **AC-TASKS-ERRANDS-010.6:** All errand copy shall be localized in every
  shipped locale. Status changes shall be announced through one polite status
  region, and every errand control shall be keyboard operable with an
  accessible name.
- **AC-TASKS-ERRANDS-010.7:** Parent transcripts shall render each start as a
  compact entry (title, profile, live status, **Open**), await as a live-status
  list, and each result as an **Errand finished** card (title, status, preview,
  **Open**). Delivered cards shall keep their placement and result snapshots.
  Results and status blocks shall not render as raw system text.
- **AC-TASKS-ERRANDS-010.8:** The errands panel shall offer
  **Stop all errands** while any errand of the task is live.
- **AC-TASKS-ERRANDS-010.9:** The parent's composer shall stay enabled while
  its errands run, and the errands chip shall show the live errand count.

- **AC-TASKS-ERRANDS-010.10:** Superseded by criterion 010.11.
- **AC-TASKS-ERRANDS-010.11:** The chip/panel shall own task-wide management;
  transcripts shall omit a duplicate current inventory. Start/await entries
  and delivered cards shall remain separate; finishing an errand shall not
  replace its start entry.

### REQ-TASKS-ERRANDS-011: Phone composition

**Intent:** Provide phone-native access to the same errand capabilities.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-011.1:** The phone Sessions drawer shall nest errands
  under their parent with status. Tapping an errand shall open its view full
  screen with a visible way back to the parent.
- **AC-TASKS-ERRANDS-011.2:** The errands chip shall open a bottom drawer with
  the grouped errand list. Rows and actions shall have touch targets of at
  least 44px, the list shall scroll inside the drawer, and the page shall not
  scroll horizontally.
- **AC-TASKS-ERRANDS-011.3:** **Stop** and **Send result to parent** shall be
  reachable from the phone errand view through a visible actions control.
- **AC-TASKS-ERRANDS-011.4:** The phone errands drawer shall offer
  **Stop all errands** with a touch target of at least 44px while any errand
  is live.
- **AC-TASKS-ERRANDS-011.5:** On a phone, **Errand finished** cards shall fit
  the screen width, and the parent's composer shall stay enabled while its
  errands run.

- **AC-TASKS-ERRANDS-011.6:** Superseded by criterion 011.7.
- **AC-TASKS-ERRANDS-011.7:** On phones the chip/drawer shall replace the
  transcript inventory. Start/result entries shall fit phone width; Open shall
  show full-screen child chat with Back. Preserve one chat scroller, reading
  position and a usable composer.

### REQ-TASKS-ERRANDS-012: Release toggle

**Intent:** Ship errands dark and keep a kill switch.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-012.1:** `features.errands` shall be off in every shipped
  profile and shall require a restart to change.
- **AC-TASKS-ERRANDS-012.2:** While the flag is off, errand tools shall be
  absent, errand actions shall fail with `feature_disabled`, the task-mode
  context shall be unchanged, no parent shall wait on errands, and no result
  shall be delivered.
- **AC-TASKS-ERRANDS-012.3:** While the flag is off, existing errand sessions
  shall remain viewable as ordinary session tabs and shall keep the isolation
  of REQ-TASKS-ERRANDS-005.

### REQ-TASKS-ERRANDS-013: Operational counters

**Intent:** Count volume and outcomes without exposing task data.

#### Acceptance criteria

- **AC-TASKS-ERRANDS-013.1:** The system shall count errand starts by outcome,
  settlements by status, and deliveries by route. Every label shall come from
  a closed set, and no task, session, errand, or profile identifier shall be a
  label value.
- **AC-TASKS-ERRANDS-013.2:** Start, admission, settlement, delivery,
  cancellation, and reconciliation shall each emit a structured log entry
  that carries identifiers as fields.

## Out of scope

- Isolated errand worktrees. Isolated work remains a subtask concern.
- Errands that start errands (depth greater than one).
- Errands in Office, quick chat, configuration, automation, and coordinator
  tasks.
- User-started errands from the UI.
- Intercepting, disabling, or restyling native subagent tools and cards.
- Per-errand model overrides beyond the agent profile.
- A filesystem writer lock between the parent and its errands.
- Delivery batching windows or timers.

## System design

- [Errands](../system-design/errands.md)

## Implementation plans

- [Errands delivery and continuation](../../../plans/errands/plan.md)
