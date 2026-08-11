---
id: "04-preserve-immediate-watch-head"
title: "Preserve the head on immediate PR association"
status: complete
wave: 4
depends_on: ["02-link-exact-remote-pr"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 04: Preserve the head on immediate PR association

## Root cause

When push detection finds a PR while no watch exists, the orchestrator creates
an already-numbered watch with the canonical base owner/repository and local branch, but omits the exact runtime head. After a terminal PR resets the watch to searching, background discovery has no head identity and falls back to the local branch against the attached repository, which misses a cross-fork PR or can select the wrong same-named branch.

## Acceptance

- Immediate push association persists `head_host`, `head_owner`, `head_repo`, and `head_branch` atomically with the resolved watch.
- The exact head remains available after terminal reset and is used by the next discovery.
- Existing direct-repository and branch-only callers remain compatible.
- No `is_fork`, parent-repository, or remote-name semantics are introduced.

## TDD sequence

1. Add a failing orchestrator regression for a push-found PR with a distinct runtime head and no existing watch.
2. Add a failing GitHub service/store test proving the head-bearing creation API persists the head with a numbered watch.
3. Implement the smallest head-bearing watch creation contract and thread the runtime head through immediate and on-demand association.
4. Run the focused orchestrator and GitHub watch tests, then refactor only if the tests remain green.

## Files

- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_remote_head_test.go`
- `apps/backend/internal/github/service_pr_watch_multi_branch_test.go`

## Verification

- `cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*(Immediate|RuntimeHead|Push.*AssociatePR)' -count=1`
- `cd apps/backend && go test -race ./internal/github -run 'Test.*(PRWatch|CreatePRWatch)' -count=1`

## Results

Added a head-bearing workspace watch creation path and threaded the observed
runtime head through immediate push association and on-demand association.
The regression first failed because the new resolved watch omitted the head,
then passed after the fix. Focused orchestrator and GitHub tests, including
race-enabled regressions, passed; the persisted head also survives resetting
the watch to searching. No `is_fork` or parent-repository metadata was added.
