---
id: "03-repair-watch-lifecycle"
title: "Repair GitHub watch lifecycle"
status: complete
wave: 3
depends_on: ["02-repair-exact-pr-discovery"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 03: Repair GitHub watch lifecycle

## Acceptance

- Retry applies the newly observed action head to an intervening searching watch without stealing another watch's numbered ownership.
- Branch-only mutation and branch-switch reset clear stale `head_*` fields atomically; a terminal reset for the same branch retains the observed head.
- Every searching route resolves attached owner/repository from durable `repository_id`. Mutable canonical base fields remain only the numbered-change address.
- Numbered watch identity is immutable through restart, retry, and association races, and status/event publication remains monotonic.

## TDD sequence

1. Add failing store/service/orchestrator tests for intervening searching watch, branch-only mutation, branch switch, same-branch terminal reset, canonical-base-to-search reset, restart, and numbered-watch race.
2. Make reset/update statements clear or retain head fields according to the explicit transition.
3. Route every searching lookup through the attached repository anchor.
4. Run focused race coverage and then the complete owned packages.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(Watch|Searching|Reset|Anchor|RemoteHead)' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(Watch|Searching|Reset|Anchor|RemoteHead)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/github ./internal/orchestrator -count=1)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-03-repair-watch-lifecycle.md`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_pr_watch_remote_test.go`
- `apps/backend/internal/github/service_automation_auth.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/github/service_pr_watch_anchor_test.go` (new)
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_remote_head_test.go`

## Dependencies

Task 02 establishes exact-head discovery semantics consumed by searching and retry transitions.

## Parallelism

Sequential before Task 04 because both tasks touch the GitHub store and watch service.

## Output contract

Update only this task file's `## Results`. Report the transition matrix, repository-anchor behavior, race evidence, files changed, RED/GREEN evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Implemented the watch lifecycle repair across store, direct and batched discovery, polling, and push-association retry paths.

Transition matrix: a searching branch-only mutation or branch-switch reset updates the local branch and clears `head_host`, `head_owner`, `head_repo`, and `head_branch` atomically; an observed head update replaces all runtime head fields together; a same-branch terminal reset changes only `pr_number` and retains the observed head; stale terminal resets and branch resets cannot overwrite a numbered watch or a newer association.

Searching lookup always resolves the attached GitHub owner/repository from persisted `repository_id` after canonical PR resolution, with legacy empty-`repository_id` rows retaining their direct owner/repo fallback. Mutable canonical owner/repo fields are used only for numbered PR status. The runtime source owner/repository is matched case-insensitively while source branch/ref and local branch lookups preserve case. Retry rechecks apply the newly observed head to an intervening searching watch and leave an already-numbered watch untouched.

Changed files: `apps/backend/internal/github/store.go`, `store_pr_watch_remote_test.go`, `service_pr_watch_anchor_test.go`, `service_automation_auth.go`, `service_pr_watch.go`, `service_pr_watch_batched.go`, `poller.go`, `apps/backend/internal/orchestrator/event_handlers_github.go`, `event_handlers_github_remote_head_test.go`, and this task file.

RED/GREEN evidence: the new store transition tests first failed because branch-only and branch-reset mutations retained stale runtime head fields; the implemented transition guards and anchor tests then passed, including canonical-base reset, unresolved-anchor rejection, exact source lookup, literal branch case, and numbered-watch race cases.

Verification passed: `cd apps/backend && go test -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(Watch|Searching|Reset|Anchor|RemoteHead)' -count=1`; `cd apps/backend && go test -race -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(Watch|Searching|Reset|Anchor|RemoteHead)' -count=1`; `cd apps/backend && go test -tags fts5 ./internal/github ./internal/orchestrator -count=1`; `make -C apps/backend lint`; and `git diff --check`.
