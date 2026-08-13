---
id: "06-compute-comparison-status"
title: "Compute status and counts from comparison context"
status: completed
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

Implemented and verified comparison status/count projection from the delivered provider-neutral comparison context. Workspace status resolves the explicit comparison target through the role resolver, uses an identity-qualified stored base anchor only while resolution is unavailable, and never substitutes `origin` when a context is present. Ahead/behind and cumulative additions/deletions are computed from the resolved comparison ref; action-head and tracking-upstream evidence remain separate. Unknown, ambiguous, absent, and stale target observations retain typed unresolved evidence and clear live comparison counts rather than presenting zero as a successful comparison. Carry-forward is qualified by context generation, target identity, resolved ref, base commit, and current HEAD, preventing stale counts from crossing a retarget or repository boundary. The task-status summary projector and frontend WebSocket/store paths preserve omitted comparison observations while applying explicit clears atomically, including multi-repository summaries.

The post-audit remediation also made role evidence first-class in status: `remote_roles_generation`, atomic `action_head`, and atomic `tracking_upstream` observations are populated from checkout-local evidence, carried through agentctl API/runtime/lifecycle/orchestrator events, persisted with repository scope, and merged in the frontend without substituting tracking for the writable head. Nullable comparison counts remain nullable in summary evidence, and live snapshot hydration is keyed by `(session, repository)` so sibling worktrees cannot borrow one another's comparison state. The fixes are committed as `e94c70bde` and `12f48a185`; the downstream status propagation is included in the next checkpoint commit.

The post-merge audit incorporated upstream `6aeaf4ad2` (`fix(web): reconcile provider history in Changes timeline`): provider commit history remains optional enrichment, provider-only and checkout-only commits are represented separately by SHA, provider-ahead Pull remains disabled without an explicit tracking upstream, and desktop/mobile Changes surfaces share the same action policy and reason semantics. No role-contract regression was found in the upstream Changes-panel, remote-contribution relation, or provider-commit resource changes.

Files changed: `apps/backend/internal/agentctl/server/process/{comparison_base.go,comparison_status.go,workspace_git_status.go,workspace_git_remote.go}`, agentctl API/runtime/lifecycle status DTOs and events, repository-scoped live snapshots and status-summary projector/model/helpers, orchestrator persistence, and the frontend git-status/session runtime paths. Earlier implementation commits are `1cee6ccb4` and `442cffb5c`; remediation commits are `e94c70bde` and `12f48a185`; upstream integration commits are `191ed4a0d` and `aa1f79e15`.

Evidence:

- `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/task/statussummary -run 'Test.*(Comparison|BaseBranch|BaseCommit|Diff|Summary)' -count=1` passed.
- `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test.*(Comparison|BaseCommit|GitStatus)' -count=1` passed.
- `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/orchestrator/executor ./internal/task/service` passed after the latest upstream merge.
- `cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts hooks/domains/session/use-session-git-summary.test.ts components/task/changes-panel-timeline-grouping.test.tsx components/task/changes-panel-remote.test.ts hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/domains/github/use-pr-commits.test.ts` passed (8 files, 81 tests).
- `cd apps/web && pnpm exec vitest run components/task/changes-panel-remote.test.ts components/task/changes-panel.test.ts components/task/changes-panel-header.test.tsx components/task/changes-panel-helpers.test.ts components/task/changes-panel-data.test.tsx hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/domains/github/use-pr-commits.test.ts components/task/commit-row.test.tsx` passed (8 files, 93 tests).
- `cd apps/backend && go test -tags fts5 ./internal/task/... ./internal/orchestrator/... ./internal/agent/runtime/... ./internal/agentctl/server/...` passed.
- `cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts` passed (2 files, 16 tests), and `NODE_OPTIONS=--max-old-space-size=4096 pnpm exec tsc --noEmit` passed.
- `git diff --cached --check` and post-merge marker audits passed. The local hook path is unavailable in this Linux worktree, so commit-hook success remains a CI responsibility.
