---
id: "07-route-git-operations"
title: "Route Git operations by remote role"
status: pending
wave: 6
depends_on: ["05-transport-comparison-context"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 07: Route Git operations by remote role

## Acceptance

- Push targets the exact writable action remote/ref. Pull targets only the explicit tracking upstream; when no usable tracking observation exists, the operation is rejected and the comparison target is never used as a fallback.
- Rebase and Merge target the delivered comparison repository/ref.
- Create PR/MR pushes the writable source and passes explicit source and target identities to GitHub, GitLab, or Azure DevOps adapters without reconstructing identities from `origin`.
- First Push distinguishes a proven absent destination ref from observation failure. Ambiguous/unresolved roles fail closed with actionable errors.
- Literal branch case, remote-contribution credential scope, force-push restrictions, and multi-repository routing remain intact.

## TDD sequence

1. Add failing temporary-repository tests for triangular push/tracking/comparison remotes, first push, missing tracking rejection, ambiguous roles, branch case, stale role generation, mismatched identity/state/head rejection, and sibling repository isolation.
2. Route Push/Pull/Rebase/Merge through the Task 01 resolver and Task 05 context.
3. Add failing provider creation tests for GitHub, GitLab, and Azure source/target arguments, then update existing adapters at their provider seams.
4. Run the complete process package after the final refactor.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'Test.*(Push|Pull|Rebase|Merge|CreatePR|CreateMR|Azure|FirstPush|Ambiguous)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*(Generation|ExpectedTarget|Pull|Push)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -count=1)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-07-route-git-operations.md`
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_test.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers_test.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_handlers_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_test.go`
- `apps/web/hooks/use-git-operations.ts`
- `apps/web/hooks/use-git-operations.test.ts`

## Dependencies

Task 05 delivers comparison context built from Task 04's complete provider source/base identities.

## Parallelism

Parallel-safe with Task 06 after the primary confirms ownership remains disjoint.

## Output contract

Update only this task file's `## Results`. Report routing semantics per operation/provider, first-push and ambiguity behavior, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
