# ADR-2026-10-23-errands-first-class-delegation: Errands as first-class delegation

**Status:** accepted
**Date:** 2026-10-23
**Area:** backend, frontend, protocol, workflow, security

## Context

Agents split work in three ways today, and none of them gives a user a
visible, steerable unit of delegated work:

- **Native subagents** (Claude Task, Cursor subagents, Codex collaboration,
  OpenCode, Auggie) run inside the parent agent process. Kandev only observes
  them over ACP and renders them as cards in the parent transcript. Most
  providers send no child text, no child thinking, and no follow-up input.
  [Subagent observability](../specs/ui/requirements/subagent-observability.md)
  explicitly excludes modelling a subagent as a task session.
- **Additional sessions** started with `spawn_session_kandev` attach to the
  task's shared workspace and can use another agent profile, but they are
  peers. They run workflow turn hooks, can become primary, can be reused by
  workflow profile switches, and give the spawning agent no way to join on a
  result.
- **Subtasks** carry their own workflow state, board card, pull request, and
  optional workspace. They suit persistent work, not an in-turn fan-out.

The task-mode context prompt
([`kandev-context.md`](../../apps/backend/config/prompts/kandev-context.md),
set by PR #2716) therefore says: use native subagents only when the user
explicitly authorized delegation, and never use Kandev task or session tools
as generic workers. That policy was correct while Kandev sessions could not
be isolated from workflow ownership.

The user confirmed two choices for this feature: errands run only in the
task's shared workspace in v1, and when `features.errands` is on, errands
become the default delegation mechanism.

## Decision

### The errand concept

An errand is a task session with the immutable role `errand` and one parent
session. The parent agent starts it through Kandev MCP tools. It has its own
transcript, thinking, tool calls, composer, agent profile, and runtime, and it
attaches to the task's canonical workspace under the existing attach-only
contract. It returns results to its parent. It never owns workflow, never
becomes primary or title owner, is never a default message target, and cannot
start errands, sessions, or tasks (depth 1). Its identity is a reserved
session ID, so `errand_id` equals `session_id` even before the session row
exists.

| Concept | Runs where | Visible and steerable | Owns workflow | Returns a result |
| --- | --- | --- | --- | --- |
| Native subagent | Inside the parent agent process | No | No | Provider-defined |
| Errand | Own session in the task's shared workspace | Yes | No | Yes, to its parent |
| Additional session | Own session in the task's shared workspace | Yes | Yes, as a peer | No |
| Subtask | Own task, optionally own workspace | Yes | Its own workflow | Through task completion |
| Office agent | Office runs and routing | Office surfaces | Office policy | Office policy |
| Workspace coordinator | Ephemeral coordinator task | Coordinator page | No | Proposals only |

These remain separate concepts. Errands do not change native subagent
observation, peer sessions, subtasks, Office, or the
[workspace coordinator](2026-09-26-workspace-coordinator.md). Errands are not
offered to Office, quick chat, configuration, automation, or coordinator
tasks in v1.

### Delegation policy under the flag

When `features.errands` is on and the session receives the errand tools, the
task-mode context states that errands are the default way to delegate work.
Native subagents remain allowed only for quick read-only lookups that finish
within the current turn. `create_task_kandev` and `spawn_session_kandev`
keep their explicit-request rules. This reverses the PR #2716 delegation
paragraph for those sessions only. When the flag is off, or the session is
not eligible, the context text stays byte-identical to the current policy.

### Workflow ownership

The parent session keeps workflow ownership. Errand turns never run
`on_turn_start` or `on_turn_complete`, never write task state, and never
record step completion. While a parent has outstanding errands, its own turn
completion defers workflow evaluation and task review-state settlement, and
`step_complete_kandev` from that parent is refused. The workflow evaluates
on the first parent turn that completes with nothing outstanding. Every
errand-specific exclusion from workflow, primary, title, routing, and reuse
paths goes through one workflow-detached session predicate that also covers
the existing completion follow-up marker.

### The parent stays conversational

Waiting on errands is not a session state. A parent whose turn ended with
outstanding errands is an ordinary idle session with an informational
indicator: its composer stays enabled, and it accepts user prompts and agent
messages. Every parent turn that starts with errands in flight receives a
bounded, server-authored errand status block. Each result is delivered at
most once, by the first of an in-progress await, the parent's next turn
(attached to its context), or a notification turn that starts when the
parent is idle with nothing else queued; a user cancel never queues one by
itself, matching cancelled turn completion. Results never enter a running turn
except through an await, and a notification carries every result pending
when its turn starts. `await_errands_kandev` remains an optional in-turn
join. Cancelling a parent turn, even during an await, cancels only that turn;
stopping, failing, or deleting the parent session still cancels its errands,
and **Stop all errands** stops them explicitly.

### Authority

An errand receives a fixed, read-only Kandev MCP surface, pinned by a catalog
test. Backend handlers for task, session, errand, step-completion, move, and
messaging mutations refuse a caller whose verified sender session is an
errand. Messages to an errand are accepted only from its parent session or
from the user. An errand's Kandev authority never exceeds its parent's.

### Admission

A fixed per-task cap bounds live errands. A session-ceiling refusal leaves the
errand queued in errand-owned state and retries it from the existing ceiling
sweep, instead of using the task's single `deferred_launch` slot. That slot
holds one deferral per task and refuses a different second deferral with
`ErrCeilingLaunchConflict`, which would break parallel errands.

## Residual risks

- **The agent's own tools.** An errand agent has the shell, MCP servers, and
  permission settings that its profile grants. While `features.auth` is off,
  Kandev's REST API and external `/mcp` endpoint accept any local caller, so
  an errand could create a task or message a session through REST without
  going through its MCP surface. This matches the residual accepted for
  ordinary sessions and for the workspace coordinator. Errands are exactly as
  capable as any Kandev session on the same profile; the restricted MCP
  surface narrows the Kandev-mediated path only. Accepted for v1.
- **Shared worktree writes.** A parent and its errands can write the same
  files concurrently. Kandev records co-residency under
  `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004` and the errand prompt
  forbids destructive Git operations, but nothing enforces that. Accepted for
  v1; isolated work remains a subtask concern.

## Consequences

- Users see delegated work as real sessions with transcripts, follow-up
  input, and a chosen profile, and agents get results back without blocking
  their conversation with the user.
- Core gains a session role and parent columns, an errand bookkeeping table,
  an `errand` MCP mode and surface, two prompts, WebSocket events, and
  frontend surfaces. All of it is reachable only while the flag is on.
- Any new session-selection path (primary promotion, routing, reuse, task
  state, workflow hooks) must consult the workflow-detached predicate.
- Turning the flag off stops new errands, status blocks, delivery, and the
  parent's workflow deferral.
  Existing errand sessions keep their role, so they stay workflow-detached
  and remain viewable as ordinary session tabs.
- Isolated errand worktrees, errands of depth greater than one, and errands
  in Office or coordinator tasks each need a new decision.

## Alternatives Considered

- **Retrofit native subagents as sessions.** They are provider-owned and
  opaque, and the subagent observability contract excludes this. Rejected.
- **Add a wait option to `spawn_session_kandev`.** Peer sessions participate
  in workflow, so every isolation rule would still be needed, and the change
  would blur an explicit peer session with delegation. Rejected.
- **Delegate through subtasks.** Each one brings workflow state, a board card,
  and pull request semantics. Too heavy for an in-turn fan-out. Rejected as
  the default; subtasks stay available for persistent work.
- **Isolated worktree per errand in v1.** Needs merge-back semantics and
  extra disk and setup cost. Deferred; subtasks cover isolated work.
- **Keep native subagents as the default and only improve observation.**
  Still no child thinking, follow-up, or profile choice. Rejected.
- **Route through Office or the coordinator.** Both carry autonomy and
  approval models that Kanban tasks did not ask for. Rejected.
- **Store the errand role in session metadata, like the completion follow-up
  marker.** Live counts and parent lookups need indexed, immutable columns,
  not a mutable JSON bag. Rejected.
- **Reuse the task-level ceiling deferral record.** One slot per task cannot
  hold parallel errands. Rejected.
- **Park the parent in a blocking wait state.** It would stop the user from
  steering the parent while errands run and make delivery depend on an
  await. Rejected.
- **Wake the parent once per result, only after all errands, or after a
  batching window.** Taking the first of an await, the next turn, or a
  notification turn, with one notification carrying every pending result,
  needs no timer and never splits results that settle together. Rejected.
- **Cancel live errands when the user cancels the parent's turn during an
  await.** That couples a turn-level action to delegated work;
  **Stop all errands** states the intent explicitly. Rejected.
