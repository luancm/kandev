---
id: "06-compute-comparison-status"
title: "Compute status and counts from comparison context"
status: pending
wave: 6
depends_on: ["05-transport-comparison-context"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 06: Compute status and counts from comparison context

## Acceptance

- Workspace status resolves the delivered comparison repository/ref through the Task 01 resolver and computes `BaseCommit`, cumulative diff, ahead/behind comparison evidence, and additions/deletions from its merge-base.
- An unrelated `origin`, a contributor fork's stale base, custom remote names, and multiple repositories cannot change the selected comparison target.
- Missing, unfetched, or ambiguous comparison refs return typed unresolved evidence and retain the last valid stored base anchor instead of substituting `origin` or another integration branch.
- Comparison evidence crosses agentctl API, runtime decode, lifecycle events, orchestrator projection, and task-status summary for single and multi-repository workspaces.

## TDD sequence

1. Add failing temporary-repository tests for `origin=fork` with a custom canonical remote, stale fork base, absent/multiple comparison refs, and sibling repository isolation.
2. Make status and diff use the delivered context plus resolver result; remove owned-file fallbacks that assign comparison meaning to `origin`.
3. Add failing API/runtime/lifecycle/orchestrator propagation tests and status-summary count tests.
4. Re-run all owned packages after the final refactor.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/task/statussummary -run 'Test.*(Comparison|BaseBranch|BaseCommit|Diff|Summary)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test.*(Comparison|BaseCommit|GitStatus)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/statussummary -count=1)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-06-compute-comparison-status.md`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_base_branch_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`
- `apps/backend/internal/agentctl/server/process/comparison_base.go`
- `apps/backend/internal/agentctl/server/process/comparison_base_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_log_merge_base_test.go`
- `apps/backend/internal/agentctl/server/api/git_multi_repo_review_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git_partial_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/events_git_status_test.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/event_handlers_git_test.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_test.go`

## Dependencies

Task 05 delivers the comparison context after Task 04 guarantees linked provider identity is complete and omission-safe.

## Parallelism

Parallel-safe with Task 07 after the primary confirms ownership remains disjoint.

## Output contract

Update only this task file's `## Results`. Report comparison resolution semantics, unresolved behavior, propagation/count coverage, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
