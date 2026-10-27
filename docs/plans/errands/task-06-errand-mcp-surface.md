---
id: "06-errand-mcp-surface"
title: "Expose errands to agents through MCP and prompts"
status: done
wave: 5
depends_on:
  - "05-errand-results-and-delivery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-009
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-001.4
  - AC-TASKS-ERRANDS-002.3
  - AC-TASKS-ERRANDS-004.3
  - AC-TASKS-ERRANDS-004.4
  - AC-TASKS-ERRANDS-006.4
  - AC-TASKS-ERRANDS-009.1
  - AC-TASKS-ERRANDS-009.2
  - AC-TASKS-ERRANDS-009.3
  - AC-TASKS-ERRANDS-009.4
  - AC-TASKS-ERRANDS-009.5
  - AC-TASKS-ERRANDS-009.6
  - AC-TASKS-ERRANDS-012.2
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 06: Expose errands to agents through MCP and prompts

## Summary

Give eligible parent sessions the three errand tools and the flag-gated
delegation policy, give errand sessions a fixed read-only `errand` MCP
surface and their own context prompt, and add the backend guards that keep
errands at depth one. With the flag off, the tools are absent and the
task-mode context is byte-identical to today.

## In scope

- `mcpmode.Errand`, `SurfaceErrand`, `CapabilityErrands`, and their entries in
  `instanceModes`, `normalizeSurface`, `Legacy`, `modeForProfile`,
  `surfaceForMode`, and `isFixedCatalogMode`.
- The `errands` tool group in `profileToolGroups` (kanban surface plus
  `CapabilityErrands`), and `registerErrandTools` for the fixed catalog,
  following `registerAutomationTools`, with no plugin tools.
- `resolveTaskSessionMCPMode` returns `McpModeErrand` for an errand session;
  `resolveTaskSessionMCPProfile` grants `CapabilityErrands` only with the flag
  on, an eligible task, and a non-errand, non-passthrough session.
- MCP server handlers for start, await (keepalive through
  `emitKeepAlivePings` and `askQuestionKeepAliveInterval`, release on an
  abandoned context), and cancel; backend handlers `mcp.start_errand`,
  `mcp.await_errands`, `mcp.cancel_errand`, `mcp.await_errands_release` with
  eligibility and the codes in the
  [design error table](../../specs/tasks/system-design/errands.md#error-codes).
- Guards in `handleCreateTask`, `handleSpawnSession`, `handleCancelErrand`,
  `handleStepComplete` (`errands_outstanding`), `handleMoveTask`, and
  `handleMessageTask` (`errand_not_permitted`, `errand_sender_not_parent`);
  `handleStartErrand` answers an errand caller with `errand_not_supported`;
  and the errand variant of `wrapAgentMessage`.
- `taskSessionListEntry` gains `role`, `parent_session_id`, `errand_status`.
- `config/prompts/errand.md`; `kandev-context.md` placeholders
  `{delegation_policy_section}` and `{errand_tools_section}`;
  `KandevContextOptions.IncludeErrandTools` and `ErrandProfiles` (at most 20
  compatible, non-passthrough profiles). The errand tools section and the
  `start_errand_kandev` and `await_errands_kandev` descriptions explain the
  status block, recommend ending the turn after starting errands, and
  present await as an optional in-turn join. Replace the task 04 placeholder
  in `applyErrandOriginContext`.
- MCP servers an errand inherits from its parent (task 04) come from the
  workspace catalog and never change the fixed `errand` Kandev catalog.

## Out of scope

- Service behavior (tasks 04 and 05) and UI rendering of the tool calls
  (task 08).

## Acceptance

1. Catalog tests pin the parent's three tools only when the capability is
   granted and pin the errand catalog to exactly the eight allowlisted tools,
   also for an errand with inherited MCP servers.
2. Every listed guard refuses an errand sender, a step completion with
   outstanding errands, and a non-parent agent message, with typed errors and
   no side effects; flag-off calls return `feature_disabled`.
3. A golden test proves the flag-off task-mode context is byte-identical to
   the current text, and the flag-on context lists compatible profiles, the
   errand delegation policy, and the end-the-turn guidance; the await tool
   sends progress notifications and releases on an abandoned call.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/common/mcpmode ./internal/mcp/profile -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server -run 'Errand|SurfaceAutomation|SurfaceCoordinator|KeepAlive' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/handlers -run 'Errand|SpawnSession|StepComplete|MessageTask|ListTaskSessions|CreateTask|MoveTask' -count=1)
(cd apps/backend && go test -trimpath ./internal/sysprompt ./config/prompts -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/executor -run 'MCPMode|MCPProfile|Errand' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/common/mcpmode/mode.go`,
  `apps/backend/internal/mcp/profile/profile.go`.
- `apps/backend/internal/mcp/server/server.go`, `handlers.go`, a new
  `errand_tools.go`, and `server_errand_mode_test.go`.
- `apps/backend/internal/mcp/handlers/handlers.go`, `spawn_session.go`,
  `config_task_handlers.go`, `list_task_sessions.go`,
  `wrap_agent_message.go`, a new `errands.go`, and tests.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`,
  `apps/backend/internal/orchestrator/task_operations.go`.
- `apps/backend/internal/sysprompt/sysprompt.go`,
  `apps/backend/config/prompts/kandev-context.md`, new `errand.md`.
- `apps/backend/pkg/websocket/actions.go`.

## Dependencies

Task 05.

## Risks

- `kandev-context.md` is shared by every task session; any whitespace change
  in the flag-off render breaks the byte-identity requirement.
- The agentctl instance API validates modes through `mcpmode.IsInstanceMode`;
  a missing entry rejects every errand launch.
- Direct WebSocket callers can claim any `sender_session_id`; the guards rely
  on the MCP server filling it, and the residual REST risk stays as recorded
  in the ADR.

## Parallelism

`parallel-safe` with task 07.

## Inputs

- [System design](../../specs/tasks/system-design/errands.md) MCP contract,
  prompts, and security sections.
- [ADR-2026-08-08-mcp-tool-profiles](../../decisions/2026-08-08-mcp-tool-profiles.md)
  and the coordinator and automation catalog tests.

## Results

Complete. Status is `done`.

Commits on `feat/errands` for this work order:

- Mode, surface, capability, and the fixed errand catalog: `fdff1703b7`,
  `142fcc3b96`.
- Errand prompt and flag-gated parent guidance: `d7018e1d69`.
- Errand MCP actions, parent guards, and the `errands_outstanding` refusal:
  `7af9bb6b59`.
- Errand first-turn replay carries the `errand.md` block, rebuilt from the
  durable errand row through a narrow lookup seam that fails closed:
  `994c61d3d5`.
- Errand sessions see only the read-only errand catalog in their Kandev
  context, from the new `errand-kandev-context.md` template: `527ed934dd`.
- Follow-up and workflow prompts pass the errand role, so the ask-question tool
  is never offered to an errand: `4938eb4f84`, with a follow-up test in
  `2925535f21`.

Ordinary task-mode context is unchanged. The flag-off golden and the
`errand-session.golden` file pin the two renders.

Verification (Go 1.26.3):

- `go test ./internal/common/mcpmode ./internal/mcp/profile -count=1`: pass.
- `go test ./internal/mcp/server -run 'Errand|SurfaceAutomation|SurfaceCoordinator|KeepAlive'`: pass.
- `go test ./internal/mcp/handlers -run 'Errand|SpawnSession|StepComplete|MessageTask|ListTaskSessions|CreateTask|MoveTask'`: pass.
- `go test ./internal/sysprompt ./config/prompts`: pass. `config/prompts` has no
  test files.
- `go test ./internal/orchestrator/executor -run 'MCPMode|MCPProfile|Errand'`: pass.
- `go build ./...` in `apps/backend`: pass.
- `scripts/lint-go-changed` over the 14 backend Go files changed since
  `b6217de61f`: 0 issues.
- `git diff --check`: clean.
- Full `go test ./internal/orchestrator -count=1`: 8 failures, all in the
  session-open and completed-relocation tests (`TestCompletedRelocation*`,
  `TestSessionOpenResumeWaitsForWorkspaceInspection`,
  `TestSessionOpenResumeHonorsEarlierCallerDeadline`,
  `TestSessionOpenLaunchFailureExemptsOnlyVerifiedInspectionDeferral`). They
  fail with `worktree directory is corrupted ... not a directory` under the
  macOS `/var` temp path. The same 8 tests fail identically on base
  `b6263ea4c3`, checked in a temporary detached worktree that was then removed.
  No other orchestrator test fails.

Remaining:

- The `ClaimTaskTitleSession` call on errand launch is a no-op. The SQL query
  excludes errand sessions (`titleClaimSessionNotErrand`), so errands never
  claim the generated title.
- The REST residual risk for `sender_session_id` stays as recorded in the ADR.
- The backendapp WorkspaceSource materializer failures were not rerun in this
  pass. They are pre-existing on macOS.
