---
spec: docs/specs/workflow-step-move-overrides/spec.md
decision: docs/decisions/2026-08-13-workflow-move-overrides.md
created: 2026-08-13
status: implemented
---

# Implementation Plan: Workflow Step Move Overrides

## Outcome

Extend the existing task move path with a typed, one-shot EntryOptions object. Human UI moves and move_task_kandev use the same destination-plus-options contract, active moves retain the object in PendingMove, direct moves store it privately behind a move_id, and the orchestrator applies profile, reset, model, and instructions overrides at target step entry. The workflow stepper and chat/passthrough next-step controls share one responsive options form.

Version one includes reset_context, instructions, agent_profile_id, and model. Pull-request draft/readiness is explicitly out of scope.

## Current seams

- TaskService.MoveTaskWithOptions validates and persists task moves, then publishes task.moved. Extend its options and add the private move-entry store rather than creating a second move service.
- The HTTP and WebSocket task move handlers currently accept only workflow, step, and position. Add the optional entry_options object at those existing boundaries.
- move_task_kandev already distinguishes idle moves from RUNNING or STARTING moves. Replace the separately queued prompt with the complete typed object in PendingMove so reset, profile, model, and instructions stay together.
- The orchestrator already owns target entry in processOnEnter, profile switching, context reset, workflow session configuration, prompt construction, and auto-start. Add one entry-override parameter through those functions and keep their existing ordering.
- The workflow stepper exposes direct Move here actions and the plan-actions hook exposes the chat and passthrough next-step action. Add a shared options form and keep direct actions as the no-override fast path.
- Touch surfaces already use useTouchDrawer with Drawer exemplars such as MRStatusChipDrawer and McpIndicator. Reuse that interaction contract for mobile move options.

## Contract

The backend workflow move package owns the typed EntryOptions value:

    {
      "reset_context": true,
      "instructions": "Start QA with the failing checkout test reproduced locally.",
      "agent_profile_id": "profile-qa",
      "model": "gpt-5.6-sol"
    }

The object is optional and omitted fields retain normal target behavior. Explicit agent profile selection wins over target-step profile selection for this entry. Explicit model selection is applied after profile selection and before the target prompt. reset_context is additive with the target step's normal reset. instructions are appended once after the normal workflow prompt or queued once on an existing target session when the target does not auto-start.

The MCP handler accepts the legacy top-level prompt as an alias for entry_options.instructions and rejects conflicting values. HTTP, WebSocket, MCP, task move options, private move entries, watcher data, and PendingMove use the same normalized fields. The task.moved event itself carries only move_id.

## Architecture

### Backend move and entry path

Create the typed model, normalization/validation helpers, and private entry store in apps/backend/internal/workflow/move/. Extend service.MoveTaskOptions with the optional value, persist it under a generated move_id for a step-changing move, and publish only that identifier in task.moved. Keep bulk moves, automatic transitions, and queue promotion at their current defaults unless they explicitly gain a future caller contract.

Extend httpMoveTaskRequest and wsMoveTaskRequest with entry_options and pass them to MoveTaskWithOptions. Keep all existing move validation and active-primary behavior. Add MCP decoding for entry_options, legacy prompt normalization, profile/model validation, and the existing configuration-task authorization checks.

Extend watcher.TaskMovedEventData with move_id and messagequeue.PendingMove with the typed value. Add a replayable pending_moves migration for a serialized entry_options column, update SQLite and memory repositories, and ensure session transfer copies it. Existing rows decode to an empty value.

Remove the current pre-move queueMoveTaskPrompt dependency from move_task_kandev. For an active source session, persist the full value in PendingMove. For an immediate move, pass move_id through task.moved and let target entry load the private value after the target session is selected. Preserve cleanup on invalid targets, transition failure, and prompt-delivery failure so a hand-off cannot leak to the source session.

Update handleTaskMoved, handleTaskMovedNoSession, autoStartTaskForStep, handleTaskMovedWithSession, processStepExitAndEnter, finalizeStepEnter, processOnEnter, and buildWorkflowPrompt or its context-bearing variant to accept the move value. Resolve explicit agent profile before normal step profile resolution, switch or reuse the target session with existing behavior, apply explicit reset before target prompt setup, apply explicit model after profile/session configuration, then append the one-time instructions. Ensure no-on-enter, no-auto-start, passthrough, new-session, reused-session, and active-turn paths each deliver the entry options once.

Reject an agent-facing override before changing the task when there is no existing target session and the target step has no auto-start action. Plain moves remain valid in that state. Preserve the existing response shape and transition history actor semantics.

### Frontend options form and surfaces

Define the frontend WorkflowMoveEntryOptions type beside the existing move API types and extend moveTask payload typing. Add a shared WorkflowMoveOptions form that owns local form state, validation, loading, and submission payload; it must not directly mutate the live session model because these values are transition-scoped.

Populate agent profile and model suggestions from the existing app store capability data where available. The form must make precedence clear: selected profile is for this entry, selected model wins for the resulting session, reset is one-time, and instructions are appended to the target step prompt. It should allow the direct fast path to bypass the form.

Update workflow-stepper.tsx so desktop target hover content keeps Move here and offers an options affordance for any current manual-move target. Update use-plan-actions.ts, chat-input-area.tsx, and passthrough-toolbar.tsx so the next-step direct button remains unchanged in intent and a sidecar options button opens the shared form for the same target.

Use a desktop popover or hover-card surface and a touch Drawer selected through useTouchDrawer. The mobile form is one-column, has one intentional vertical scroll owner, respects safe-area padding, avoids document-level horizontal overflow, and gives primary/secondary controls at least 44px touch targets. Do not make hover the only path to the options on a phone.

Route every new label, help text, validation message, and button copy through the existing task/workflow i18n namespaces in en, pt-pt, pseudo, and zh-cn. Do not add literal user-facing strings to the component or tests.

### Verification, E2E, and documentation

Add backend unit coverage for normalization, precedence, direct event propagation, deferred persistence and restart, session transfer, invalid target/profile/model, prompt delivery, and exactly-once behavior. Add frontend component and hook coverage for direct versus options moves, payload shape, profile/model/reset/prompt state, chat/passthrough parity, and touch Drawer rendering.

Add desktop and mobile Playwright coverage using the existing workflow fixtures and semantic backend polling. The desktop scenario should open a target's options, submit an override, and verify the task enters the target with the selected fields. The mobile scenario should open the same options through touch, verify the Drawer controls, submit, and assert no horizontal overflow. Include an active-agent deferred move case where practical.

Update docs/public/tasks-and-workflows.md with one-time move options and the distinction from durable step defaults. Update docs/public/agent-communication.md and docs/public/websocket-api.md with the normalized move_task_kandev and transport contract, including the legacy prompt alias. Update apps/backend/config/prompts/config-context.md so configuration agents discover the options object. Add the spec and ADR to their indexes.

## Waves

### Wave 1

- [x] [task-01-backend-move-contract-and-entry](task-01-backend-move-contract-and-entry.md)

### Wave 2

- [x] [task-02-frontend-move-options](task-02-frontend-move-options.md) (depends on Task 01 contract)

### Wave 3

- [x] [task-03-e2e-docs-and-delivery](task-03-e2e-docs-and-delivery.md) (depends on Tasks 01 and 02)

## Exact verification

Backend focused checks (the full service package currently contains unrelated filesystem-test failures in this environment, so the move-specific tests are the signal for this change):

    cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/workflow/move ./internal/task/service -run 'TestService_MoveTaskWithEntryOptions|TestService_MoveTaskWithOptionsAllowsRunningPrimarySession|TestService_MoveTaskWithOptionsRejectsEntryOptionsWithoutTargetSession|TestNormalizeEntryOptions|TestEntryOptions' -count=1
    cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/mcp/handlers -run 'TestHandleMoveTask|TestDeferMoveTask|TestMoveTaskErrorMessage' -count=1
    cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/orchestrator/messagequeue ./internal/orchestrator -run 'TestSQLiteRepository_PendingMove|TestPendingMove|TestAppendWorkflowMove' -count=1
    cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/mcp/server ./internal/task/handlers -run '^TestServer|^TestSetPluginTools|Move|move|EntryOptions' -count=1

Frontend focused checks:

    cd apps/web && pnpm exec vitest run lib/api/domains/kanban-api.test.ts hooks/domains/kanban/use-workflow-move.test.ts components/task/workflow-move-options.test.ts components/task/workflow-stepper.test.ts hooks/domains/kanban/use-plan-actions.test.ts
    cd apps/web && pnpm run typecheck (passed before final verification rerun; the rerun was killed by the environment's memory limit)
    cd apps/web && pnpm run i18n:check
    cd apps/web && pnpm run i18n:ratchet

Focused browser checks: not added in this pass; the responsive Drawer and options payload are covered by component tests, while the existing workflow proceed E2E continues to cover the unchanged direct path.

Public documentation checks:

    node --test scripts/validate-public-docs.test.mjs
    node scripts/validate-public-docs.mjs

Before delivery, run git diff --check and verify staged Go files with gofmt -l according to the repository hooks. Broader repository verification remains a later explicit verification phase, not part of this design turn.

## Handoff

Implementation is complete for the approved v1 boundary. The direct and deferred move paths share the typed one-shot EntryOptions contract, and the public task.moved event exposes only the private move_id. Pull-request draft/readiness remains intentionally out of scope. The task is ready for Review after the focused checks and commit are recorded.
