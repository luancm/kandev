---
id: "02-repair-exact-pr-discovery"
title: "Repair exact GitHub PR discovery"
status: pending
wave: 2
depends_on: ["01-resolve-remote-roles"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 02: Repair exact GitHub PR discovery

## Acceptance

- One discovery key includes attached repository identity, literal local branch, whether an exact head exists, and normalized exact head host/repository/ref.
- Legacy headless watches use attached repository plus local branch as the expected head and never accept a same-branch sibling fork.
- GraphQL and client paths paginate until uniqueness is established or fail closed, dedupe by normalized canonical PR identity, compare repository identity with provider-appropriate case rules, preserve branch case, and validate the head host against the attached GitHub host.
- Batched and direct discovery return equivalent exact-head results and useful ambiguity/not-found diagnostics.

## TDD sequence

1. Add failing tests for two same-branch watches with different heads, a headless sibling fork, more than two candidates, duplicate candidates, case-only repository identity, literal branch-case distinction, and foreign host.
2. Add exact head identity to batched grouping and request/response mapping.
3. Paginate and dedupe before uniqueness checks, retaining fail-closed ambiguity.
4. Align PAT, `gh`, and mock clients with the same discovery contract.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run 'Test.*(ExactHead|Headless|Pagination|Ambiguous|BranchCase|HeadHost)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/github -count=1)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-02-repair-exact-pr-discovery.md`
- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/graphql_test.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr_watch_batched_exact_head_test.go` (new)
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/pat_client_test.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/gh_client_test.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/mock_client_test.go`

## Dependencies

Task 01 must establish normalized repository identity rules used by exact-head matching.

## Parallelism

Sequential before Task 03 because both tasks touch GitHub watch discovery.

## Output contract

Update only this task file's `## Results`. Report discovery-key semantics, pagination/dedup behavior, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
