# ADR-2026-08-13: Treat Workflow Move Overrides as One-Shot Transition Input

**Status:** accepted
**Date:** 2026-08-13
**Area:** backend, frontend, protocol, workflow
**Related spec:** [Workflow Step Move Overrides](../specs/workflow-step-move-overrides/spec.md)

## Context

Kandev already supports moving a task through the stepper, task APIs, and move_task_kandev. A move currently carries only its destination, so an operator must edit workflow defaults to handle an exceptional transition such as entering Work with a fresh context or using a different agent for one review.

Workflow step configuration is durable policy. It owns the normal profile, reset, prompt, model, and auto-start behavior for a step. It must not be mutated merely because one transition needs an exception.

The direct move path publishes task.moved and the active-agent path stores a durable PendingMove until the current turn ends. Any new override must therefore survive both paths and be applied at the target step entry, after the target session has been selected but before its prompt is dispatched.

## Decision

1. Add a typed EntryOptions value to the existing move contract. Version one supports reset_context, instructions, agent_profile_id, and model. The value is optional and applies to one requested transition only.
2. Keep workflow step configuration unchanged. A move override has higher precedence than the target step's corresponding profile, model, reset, and prompt behavior for this entry, but it is never written back as a workflow default.
3. Resolve the target session profile using explicit agent_profile_id first, then the target step's existing profile behavior. Reuse the current session-switch and session-reuse machinery. Apply an explicit model to the resulting session after profile selection, with the explicit model taking precedence over the selected profile's default.
4. Apply reset_context to the resulting session before target-step session configuration and prompt dispatch. A profile switch normally creates or reuses the fresh target session; reset_context remains an explicit request and is harmless when the selected session is already fresh.
5. Build the normal target workflow prompt first, then append one-time move instructions. When the target does not auto-start, deliver those instructions through the target session's existing queue exactly once. The old top-level MCP prompt remains a compatibility alias for EntryOptions.instructions.
6. Carry the same typed value through HTTP, WebSocket, MCP, private move-entry storage, and the durable PendingMove record. The public task.moved event carries only move_id. The orchestrator owns application at step entry so direct moves and deferred agent moves share reset, profile, model, and instructions ordering.
7. Preserve all existing authorization, reachable-target, same-workspace, archived-task, active-session, and WIP admission rules. Overrides add no permission and cannot bypass a transition guard.
8. Expose one shared options form from the workflow stepper and the next-step controls in chat and passthrough mode. Desktop uses the existing hover/popover pattern; touch devices use a bottom Drawer with the same controls and validation. The direct Move here and next-step actions remain available without opening the form.
9. Keep pull-request draft/readiness control out of version one. The typed override object is intentionally extensible so a later PR-specific field can be added without creating a second move endpoint or configuration system.

## Consequences

- Human and agent callers can make an exceptional transition without changing reusable workflow policy.
- Deferred moves retain their options across the active-turn boundary and backend restart through PendingMove persistence; direct entries remain private behind move_id and are deleted after consumption.
- Existing step-entry behavior remains the source of truth for on_exit, reset, session configuration, plan mode, auto-start, and prompt construction; move overrides are a small precedence layer.
- An explicit model is a one-shot move request, but its effect on the resulting task session follows the existing session runtime model semantics. It is not copied into workflow configuration.
- The UI has one options form and one payload shape across desktop, mobile, chat, passthrough, HTTP, and MCP.
- The move handler must normalize the legacy MCP prompt into instructions and reject conflicting duplicate values rather than silently choosing one.
- A move that supplies agent-facing overrides must have an existing target session or a target step that auto-starts an agent. A plain move remains valid when neither is present.

## Alternatives considered

### Mutate the target workflow step

Rejected. This turns a one-time exception into durable policy, changes later tasks, and makes the operator's intent difficult to audit.

### Add a separate override endpoint

Rejected. A second mutation contract would make it possible for the destination and the options to become inconsistent. The destination and its one-shot options belong to one move request.

### Keep prompting as a separately queued string

Rejected. It loses model/profile/reset intent, can be delivered to the wrong session during a profile switch, and makes direct and deferred moves behave differently.

### Save a user preference for future moves

Rejected for version one. The request is explicitly one move only; durable preferences can be considered separately if a real repeated-use need appears.

### Add PR draft/readiness to the first override object

Deferred. It is a separate provider automation concern and does not share target session entry semantics with the v1 fields.
