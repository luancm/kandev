---
status: approved
created: 2026-08-13
owner: kandev
---

# Workflow Step Move Overrides

## Why

Kandev can move a task to another workflow step, but every move uses the target step's durable defaults. Operators and agents need a one-time exception for a particular hand-off without editing workflow policy or creating a second task configuration.

## What

- A human or agent can move a task to any target exposed by the existing manual-move policy, including explicitly allowed non-adjacent steps. The request does not bypass current workspace, archive, active-session, WIP, or target-membership validation.
- One move can carry these optional overrides:
  - reset_context: clear the resulting session's context before the target prompt;
  - instructions: append a custom hand-off message after the normal target workflow prompt;
  - agent_profile_id: select a different configured agent profile for this entry using existing session switch and reuse behavior; and
  - model: select a model for the resulting session, with an explicit model taking precedence over the selected profile's default.
- The override object is consumed by this transition. It never changes the target workflow step, workflow default, or later task defaults. Runtime effects such as a selected session model follow the existing task-session runtime semantics rather than being copied into workflow configuration.
- The target step's existing on_exit and on_enter actions still run once. The effective order is target profile resolution, session switch or reuse, explicit reset when requested or configured, target session configuration, explicit model application, normal workflow prompt construction, and appended one-time instructions delivery.
- A profile override may reuse an existing target-profile session or create a new one according to the current workflow behavior. A model override never creates a new task session by itself.
- When a profile override creates a new target session that is still in `CREATED` state, the later auto-start launch must preserve that explicit profile rather than reapplying the target step's durable profile.
- If the target auto-starts an agent, the one-time instructions are appended to the auto-start prompt. If the target has an existing session but does not auto-start, the instructions are queued for that target session's next prompt exactly once. A move that supplies agent-facing entry options is rejected before changing the task when there is neither a target session nor target auto-start.
- The HTTP move endpoint, WebSocket task move action, and move_task_kandev accept the same nested entry_options object. The existing top-level MCP prompt remains accepted as a compatibility alias and is normalized into entry_options.instructions; conflicting prompt values are rejected.
- The active-agent move path stores the complete typed entry options in PendingMove, so a move requested during a RUNNING or STARTING turn applies after that turn rather than racing the current prompt. Direct moves persist the private value under a move_id and publish only that identifier in task.moved.
- The stepper keeps a direct Move here action. Its options affordance opens the shared move form for the hovered or selected target. Chat and passthrough next-step controls keep their direct action and add a sidecar options action that opens the same form.
- On touch devices, target-step options open a bottom Drawer rather than depending on hover. The form remains one-column, scrollable within the surface, safe-area aware, and usable with touch targets of at least 44 pixels. Desktop and mobile use the same form state, validation, and request payload.
- The agent-facing architecture is ready for the same contract to be used by move_task_kandev. Version one does not add pull-request draft/readiness controls; a future move-specific field can extend the typed object.

Decision: [ADR-2026-08-13-workflow-move-overrides](../../decisions/2026-08-13-workflow-move-overrides.md).

## Data model

The shared move override value is an optional object with this shape:

    {
      "reset_context": true,
      "instructions": "Start QA with the failing checkout test reproduced locally.",
      "agent_profile_id": "profile-qa",
      "model": "gpt-5.6-sol"
    }

Empty strings are omitted. reset_context is opt-in and does not suppress a reset already configured on the target step. An omitted model leaves the selected profile or current session model unchanged.

The backend owns one typed EntryOptions definition in the workflow move package. HTTP and WebSocket request structs, task move options, private move-entry persistence, watcher data, MCP decoding, and PendingMove persistence use that shape rather than independent maps. The public task.moved event carries only move_id; entry instructions and model/profile choices stay in the private move-entry store.

PendingMove persistence adds a serialized move-overrides column with an empty-object default and a replayable migration. Existing rows decode as an empty override. The in-memory repository follows the same value semantics.

## API surface

The existing POST task move endpoint adds an optional entry_options object. The existing WebSocket task move action adds the same object. The existing move_task_kandev tool adds an optional entry_options object containing reset_context, instructions, agent_profile_id, and model.

The legacy MCP prompt argument remains accepted for compatibility. A non-empty legacy prompt is copied to entry_options.instructions only when the nested field is empty; providing both values is a validation error. Responses include the normalized entry_options when supplied and identify immediate versus deferred acceptance with disposition. A deferred MCP response returns the authoritative source task until turn-end applies the move.

The task.moved event includes move_id when a private entry is present; it never exposes instructions, profile IDs, or model overrides. Deferred moves include the complete typed value in PendingMove. No new task or workflow endpoint is introduced.

## Precedence and state machine

| Stage | Normal source | One-move override | Result |
| --- | --- | --- | --- |
| Target profile | Target step and workflow profile resolution | agent_profile_id | Explicit profile wins for this entry. |
| Session | Existing current session or existing target-profile session | None | Existing switch, reuse, and primary-session rules remain authoritative. |
| Context | Target step reset_agent_context action | reset_context true | Reset when either source requests it. |
| Session settings | Target step configure_session action and profile defaults | model | Explicit model is applied after profile selection and before the target prompt. |
| Prompt | Workflow instructions, step prompt, and task prompt | instructions | Move instructions are appended once and never replace the normal workflow prompt. |

The transition follows this sequence:

1. Authenticate the caller and validate the target using existing move rules.
2. Normalize and validate overrides, including the requirement for a target session or auto-start path when agent-facing fields are present.
3. Apply the move immediately when safe, or persist PendingMove when the source agent is active.
4. Persist the task step change and publish or process the transition through the existing event path.
5. Resolve or switch the target session, apply reset and model overrides, and dispatch or queue the effective prompt.
6. Consume the private entry object. A failed pre-transition validation leaves the task and entry untouched; a failed target-entry operation does not retry a partially applied prompt or model as a second move.

## Permissions

The existing task move authorization and MCP session authorization remain unchanged. Human HTTP and WebSocket callers use the authenticated owner boundary. Agent move_task_kandev calls retain their session-scoped task authorization. Selecting a different configured agent profile or model does not grant access to profiles, models, workspaces, or tasks that the caller could not already use.

## Failure modes

- Invalid target, cross-workspace target, archived task, WIP rejection, active-session conflict, or invalid override leaves the task in its original step.
- An invalid or unavailable agent profile is rejected before the move is persisted.
- An unsupported model is rejected by the existing model capability/configuration boundary before the target prompt is dispatched.
- A pending move survives backend restart with its override object. Invalid target data discovered at turn end is dropped using the existing pending-move cleanup path and cannot deliver its prompt to the source session.
- A target profile switch transfers the pending override with existing queued state to the resulting session, preventing delivery to the previous profile.
- If prompt queueing fails after a direct move has been persisted, the task remains at the target and the backend records a sanitized warning; the prompt is not silently delivered to the source session.
- A target without auto-start and without a target session rejects agent-facing overrides rather than accepting an option that has no recipient.
- Repeated task.moved or agent-ready signals do not apply the same override twice. Existing transition and queue idempotence remains the guard.

## Persistence guarantees

- Workflow defaults never change as a side effect of a move override.
- Pending active-turn moves and their complete override object survive the message-queue repository's normal restart/reload path.
- Direct moves use the existing task.moved delivery path; the event includes only move_id and the orchestrator loads the private entry before applying it at entry.
- A queued custom prompt is tagged as a move hand-off and is consumed at most once by the target session.
- The private move entry is deleted after target entry is dispatched. Any session runtime model state that remains is governed by existing task-session model persistence, not by workflow configuration.

## Scenarios

- **GIVEN** a task is in Spec and Work is reachable, **WHEN** the operator chooses Work with reset_context enabled, **THEN** the task moves once and the target session is reset before Work's auto-start prompt.
- **GIVEN** Review is the current step and QA is reachable, **WHEN** the operator chooses a different configured agent profile, **THEN** Kandev reuses or creates the target-profile session using existing workflow semantics and starts QA with that profile.
- **GIVEN** a profile override creates a `CREATED` target session for an auto-start step, **WHEN** the target prompt launches that prepared session, **THEN** the session and agent process retain the one-time override profile instead of being rewritten to the step's durable profile.
- **GIVEN** the target profile is selected and a model override is supplied, **WHEN** QA entry runs, **THEN** the explicit model is applied after profile selection and before QA's prompt.
- **GIVEN** a target step has its own workflow prompt, **WHEN** a move includes a custom prompt, **THEN** the normal workflow prompt remains intact and the custom text is appended exactly once.
- **GIVEN** the source agent is RUNNING, **WHEN** it calls move_task_kandev with profile, model, reset, and instructions overrides, **THEN** the tool returns a successful deferred move with disposition `deferred` and the complete override is applied after the current turn ends.
- **GIVEN** a deferred move is persisted and the backend restarts, **WHEN** the source session becomes ready, **THEN** the target and all override fields are restored and applied once.
- **GIVEN** the task has no session and the target step auto-starts, **WHEN** a human move includes an agent profile or model override, **THEN** the first target session and prompt use those values.
- **GIVEN** the task has no session and the target step does not auto-start, **WHEN** a move includes an agent-facing override, **THEN** the request is rejected without changing the task.
- **GIVEN** an existing target session is waiting on a step without auto-start, **WHEN** a move includes a custom prompt, **THEN** the prompt is queued for that session's next input and is not duplicated by on_enter.
- **GIVEN** the target step normally resets context, **WHEN** reset_context is omitted, **THEN** the normal reset still happens; the override does not disable workflow policy.
- **GIVEN** a user opens a stepper target on desktop, **WHEN** they choose options, **THEN** the shared form can set profile, model, reset, and prompt before submitting the move.
- **GIVEN** a user taps a target on a touch device, **WHEN** they open options, **THEN** a Drawer exposes the same controls without requiring hover or horizontal scrolling.
- **GIVEN** a user proceeds from chat or passthrough, **WHEN** they use the sidecar options action, **THEN** it targets the same next step as the direct proceed action and submits the same move contract.
- **GIVEN** the legacy MCP prompt is supplied without entry_options, **WHEN** move_task_kandev runs, **THEN** it behaves as entry_options.instructions and existing agents remain compatible.
- **GIVEN** both the legacy prompt and entry_options.instructions are non-empty, **WHEN** move_task_kandev runs, **THEN** it returns validation error and does not move the task.
- **GIVEN** an invalid profile, model, target, or WIP admission, **WHEN** a move is submitted, **THEN** the task remains in its source step and no override is queued.
- **GIVEN** a move completes, **WHEN** a later task enters the same target step without overrides, **THEN** it uses the target workflow defaults rather than the exceptional move values.

## Out of scope

- Pull-request creation mode such as draft versus ready for review.
- Persisting move overrides as workflow defaults or user preferences.
- Bypassing existing target reachability, authorization, WIP, archive, workspace, or active-session checks.
- A new transition endpoint separate from the existing move APIs.
- Arbitrary executor, credential, MCP, permission-mode, or provider configuration overrides.
