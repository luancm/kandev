---
id: "02-link-exact-remote-pr"
title: "Link the exact remote pull request"
status: complete
wave: 2
depends_on: ["01-stream-runtime-head-ref"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 02: Link the exact remote pull request

## Acceptance

- A searching PR watch durably stores the runtime head host, owner, repository,
  and remote branch separately from its local task branch and attached
  `repository_id`.
- Runtime-head updates affect only searching watches. Discovery atomically
  changes a searching watch to the PR's canonical base owner/repository and
  number, so the first numbered status check addresses the correct repository.
- Branch-only discovery returns one open PR connecting the authorized attached
  repository to the exact observed head ref. It handles both attached-fork and
  attached-main layouts, regardless of the configured remote name.
- Another fork's same-named branch is ignored, duplicate representations of one
  PR are deduplicated, multiple eligible PRs remain ambiguous, and a base
  outside workspace scope or automation access is rejected.
- PAT, GitHub App/token, and named GitHub CLI automation use the same GraphQL
  matching behavior, while rows without runtime head data preserve safe direct
  repository discovery.

## TDD sequence

1. Add failing store tests for additive defaults, guarded searching-target
   updates, branch collision, and atomic canonical watch resolution.
2. Add failing GraphQL tests for attached-fork, attached-main, sibling-fork,
   duplicate, ambiguous, and renamed-head-ref cases.
3. Add failing service/orchestrator tests proving the runtime head persists from
   git status and both immediate and polled paths resolve an upstream PR under
   its base repository.
4. Implement persistence, query, client, service, and orchestration changes in
   that order; refactor after all focused regressions are green.

## Verification

- RED/GREEN watch persistence: `cd apps/backend && go test -tags fts5 ./internal/github -run 'Test.*PRWatch.*(RuntimeHead|CanonicalBase|Searching)' -count=1`
- RED/GREEN exact GraphQL matching: `cd apps/backend && go test -tags fts5 ./internal/github -run 'Test.*BranchQuery.*(Fork|ExactHead|Ambiguous|Duplicate)' -count=1`
- Client parity: `cd apps/backend && go test -tags fts5 ./internal/github -run 'Test(PATClient|GHClient)_FindPRByBranch' -count=1`
- Association path: `cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Test.*(CheckSessionPR|Push.*AssociatePR|RuntimeHead)' -count=1`
- Focused packages: `cd apps/backend && go test -tags fts5 ./internal/github ./internal/orchestrator -count=1`

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_watch_reset_test.go`
- `apps/backend/internal/github/store_pr_watch_remote_test.go`
- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/graphql_test.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/pat_client_test.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/gh_client_test.go`
- `apps/backend/internal/github/service_automation_auth.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/event_handlers_git_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `docs/specs/ui/ci-pr-automation.md`
- `docs/plans/fork-pr-task-linking/plan.md`
- `docs/plans/fork-pr-task-linking/task-02-link-exact-remote-pr.md`

## Dependencies

Task 01 must establish the credential-free runtime-head stream contract.

## Parallelism

Sequential. Schema, query, service, and orchestrator changes share one watch
state transition and should land with one coherent regression suite.

## Inputs

- The runtime head contract from Task 01.
- The behavioral scenarios in `docs/specs/ui/ci-pr-automation.md`.
- Existing ambiguity guards in `apps/backend/internal/github/graphql.go` and
  searching-watch paths in `apps/backend/internal/github/service_pr_watch.go`.
- Workspace repository-scope rules in
  `apps/backend/internal/github/service_routing.go`.

## Output contract

Report changed files, permanent RED/GREEN evidence, migration and atomic-race
coverage, exact focused package outcomes, ambiguity/scope results, and
synchronized task/plan statuses in the primary session.

## Results

RED/GREEN evidence: focused store, GraphQL, PAT, GitHub CLI, service, and
orchestrator tests were added while the new behavior was absent, then passed
after implementation. The package command `go test -tags fts5 ./internal/github
./internal/orchestrator -count=1` passed, including exact fork/base matching,
sibling-fork rejection, duplicate deduplication, ambiguity fail-closed
behavior, guarded persistence, atomic canonical resolution, and scope/access
checks. The broader backend command including agentctl, lifecycle, API,
runtime-agentctl, GitHub, and orchestrator packages also passed. `git diff
--check` passed and the throwaway diagnostic fixture was removed.
