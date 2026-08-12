---
id: "05-transport-comparison-context"
title: "Transport comparison context into every runtime"
status: in_progress
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

Implemented the backend-owned comparison-context contract. Selection is fail-closed and ordered: a unique linked change whose complete source repository/ref exactly matches the writable action head wins; when no linked change competes, a validated remote-contribution target wins; otherwise the attached repository and selected base ref are used. Zero, multiple, missing, or incomplete exact matches remain unresolved rather than falling back by row order, PR number, or branch name.

The transported value is credential-free `gitremote.ComparisonContext`, containing a validated repository/ref target, optional identity-qualified base commit, opaque generation, and presence-aware `replace` or `clear` update. Launch, fresh resume, multi-repository resume, executor metadata, standalone, Docker, SSH, Sprites, agentctl-ready recovery, and live refreshes carry the same deterministic worktree-keyed map. Omitted updates retain existing observations, explicit empty observations clear them, sibling updates remain isolated, and invalid input is rejected atomically.

Provider association, update, and removal events for GitHub, GitLab, and Azure DevOps rehydrate the task-owned selection and push it to every live execution. Base-branch edits share the same live refresh path. Agentctl config and instance creation seed the map before discovery, while the process manager and workspace tracker apply per-worktree changes without exposing raw URLs or credentials.

Changed files cover `internal/common/gitremote`, orchestrator launch/resume projection and provider refresh handlers, task-service selection and live delivery, backend adapters, lifecycle launch/recovery/executor plumbing, agentctl client/config/instance/API/process/workspace transport, and focused comparison-context tests.

RED/GREEN evidence includes exact linked-change uniqueness and fallback selection tests, malformed repository/ref and credential rejection tests, normalized provider-host tests, lifecycle metadata preservation tests, agentctl atomic/sibling/explicit-clear tests, and process-manager race coverage.

Verification results:

- PASS: `(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor ./internal/task/service ./internal/backendapp ./internal/orchestrator -run 'Test.*ComparisonContext' -count=1)`.
- PASS: `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*ComparisonContext.*(Launch|Resume|Metadata|Restart|Standalone|Docker|SSH|Sprites)' -count=1)`.
- PASS: `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*ComparisonContext' -count=1)`.
- PASS: `go test -race -tags fts5 ./internal/agentctl/server/process -run 'TestManagerUpdateComparisonContexts' -count=1`.
- PASS: focused common identity, orchestrator executor, backend adapter, and lifecycle comparison-context tests.
- PASS: `git diff --check`.
- The complete owned-package command passed for every package except `internal/task/service`, where unrelated existing filesystem tests failed (`TestCreateDirectoryRejectsInvalidOrExistingChild` and `TestServiceInitializeLocalRepository*`) with `parent directory cannot be accessed`; all Task 05 focused gates and changed-package tests passed.
