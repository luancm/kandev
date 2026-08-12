---
id: "05-transport-comparison-context"
title: "Transport comparison context into every runtime"
status: pending
wave: 5
depends_on: ["01-resolve-remote-roles", "04-persist-provider-source-identities"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 05: Transport comparison context into every runtime

## Acceptance

- Extend the existing base-branch delivery path into one per-repository comparison-context contract instead of adding a parallel selector. The contract contains credential-free repository identity, literal target ref, and deterministic worktree subpath.
- Selection precedence is a unique linked change whose exact source repository/ref matches the worktree's writable action head and branch context, then a validated remote-contribution target when no linked change competes, then the attached repository plus selected base branch. Zero, multiple, or incomplete exact matches are unresolved and never selected by row order, number, or branch name alone. Incomplete hydration or refresh cannot erase a known context.
- Full launch, single-repository resume, multi-repository resume, agentctl-ready restart/recovery, and live branch/association/sync/removal retargeting deliver the same context to standalone, Docker, SSH, and Sprites runtimes.
- Agentctl config and instance state seed the context before workspace discovery; process manager and workspace tracker apply updates to the matching worktree only.
- The update envelope is presence-aware and atomic: omission retains the last observation, an explicit complete empty observation clears stale contexts, and an incomplete sibling map cannot wipe another worktree.
- The agentctl control client and API endpoint validate repository/ref shapes, reject credential-bearing identities, and return actionable per-worktree update failures.
- GitHub/GitLab update and removal events plus the Azure TaskPR event trigger provider rehydration and push a new full observation to every running execution, so association, sync, and detach retarget without restart.
- Tasks 05 and 06 can consume the delivered context without reaching into provider stores or reimplementing selection precedence.

## TDD sequence

1. Add failing tests for unique exact source matching (zero, one, multiple, and incomplete candidates), validated fallback precedence, multi-repository subpath mapping, fresh launch, single-repository resume top-level context, multi-repository resume repo contexts, agentctl-ready restart, every executor configuration, live association/sync/removal retargeting, omission versus explicit clear, sibling isolation, malformed refs, and credential-bearing input.
2. Introduce the backend comparison-context value and selection helper by deepening `manager_base_branches.go` and its task-service provider rather than creating a second selector.
3. Carry the value through lifecycle launch/restart and each executor backend into agentctl config/instance state.
4. Add the presence-aware process-manager/workspace-tracker update path and control/API endpoint, then make live branch edits plus GitHub/GitLab/Azure association, sync, and removal events invoke a full rehydration.
5. Re-run the complete owned-package suite after the final refactor.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor ./internal/task/service ./internal/backendapp ./internal/orchestrator -run 'Test.*ComparisonContext' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*ComparisonContext.*(Launch|Resume|Metadata|Restart|Standalone|Docker|SSH|Sprites)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*ComparisonContext' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/common/gitremote ./internal/orchestrator/executor ./internal/task/service ./internal/backendapp ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -count=1)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-05-transport-comparison-context.md`
- `apps/backend/internal/common/gitremote/comparison_context.go` (new; consumes Task 01 identity types)
- `apps/backend/internal/common/gitremote/comparison_context_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/comparison_context_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab.go`
- `apps/backend/internal/orchestrator/event_handlers_azuredevops.go`
- `apps/backend/internal/orchestrator/event_handlers_comparison_context_test.go` (new)
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_branch_update.go`
- `apps/backend/internal/task/service/service_branch_update_test.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/adapters_test.go`
- `apps/backend/internal/backendapp/comparison_contexts.go` (new)
- `apps/backend/internal/backendapp/comparison_contexts_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_base_branches.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_comparison_contexts.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/comparison_context_metadata_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/base_branches_metadata_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_config_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_control_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_remote_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_comparison_context_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_submodules.go`
- `apps/backend/internal/agentctl/server/process/manager_rescan.go`
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/manager_update_base_branches_test.go`
- `apps/backend/internal/agentctl/server/process/manager_update_comparison_contexts_test.go` (new)
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/workspace_base_branches.go`
- `apps/backend/internal/agentctl/server/api/workspace_base_branches_test.go`
- `apps/backend/internal/agentctl/server/api/workspace_comparison_contexts.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_comparison_contexts_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/agentctl/workspace_base_branches.go`
- `apps/backend/internal/agent/runtime/agentctl/workspace_comparison_contexts.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/client_misc_test.go`

## Dependencies

Task 01 supplies the normalized identity accepted by this transport. Task 04 supplies complete omission-safe provider identities and update/removal events, including the new Azure TaskPR update event.

## Parallelism

Sequential contract gate. No Task 06, 07, or later consumer work begins until this task is integrated.

## Output contract

Update only this task file's `## Results`. Report the selection precedence, transport shape, all launch/restart/live-update paths covered, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
