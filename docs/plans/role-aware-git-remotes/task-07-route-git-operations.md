---
id: "07-route-git-operations"
title: "Route Git operations by remote role"
status: completed
wave: 7
depends_on: ["05-transport-comparison-context", "06-compute-comparison-status"]
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

Sequential after Task 06 because the upstream audit found shared agentctl Git client/API files.

## Output contract

Update only this task file's `## Results`. Report routing semantics per operation/provider, first-push and ambiguity behavior, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Implemented role-aware Git mutation routing across agentctl process, HTTP/API and ACP runtime transport, websocket handlers, and the web operation hook. Push resolves the exact writable action remote and configured destination refspec, including first-push absent evidence and triangular push URLs; Pull requires the explicit tracking upstream and never falls back to comparison; Rebase and Merge use the delivered comparison identity/ref. Mutations validate role generation, identity, observation state, remote head, and comparison generation while holding the serialized Git-operation lock, failing closed for stale, ambiguous, unresolved, unknown, or absent evidence. Create PR/MR routes exact provider-neutral source/base identities through GitHub, GitLab, and Azure adapters without reconstructing either side from origin, while preserving provider credential scope, contribution force-push restrictions, literal branch case, and repository scoping. RED/GREEN coverage includes triangular remotes, configured destination refspec case, missing tracking, stale tracking head, stale generation, and first-push observation. Verification passed: `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'Test.*(Push|Pull|Rebase|Merge|CreatePR|CreateMR|Azure|FirstPush|Ambiguous)' -count=1)`, `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*(Generation|ExpectedTarget|Pull|Push)' -count=1)`, `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -count=1)`, full affected API/handler/runtime packages, focused web hook Vitest (5 tests), web TypeScript check, scoped golangci-lint, Prettier check, and `git diff --check`.
