---
id: "01-stream-runtime-head-ref"
title: "Stream the runtime Git head ref"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 01: Stream the runtime Git head ref

## Acceptance

- Agentctl reports the current branch's exact push remote and remote branch,
  falling back to its upstream tracking target, without depending on the remote
  being named `origin` or `upstream`.
- An explicit push URL takes precedence over the fetch URL. Multiple push URLs
  are accepted only when they normalize to the same provider repository;
  conflicting, local-only, unsupported, or invalid targets yield no runtime
  head identity.
- The git-status stream carries only normalized provider, host, owner,
  repository, and branch fields. Credentials and raw remote URLs never leave
  agentctl.
- The runtime agent and lifecycle event pipeline preserve the complete
  credential-free shape without changing existing `RemoteBranch` or
  ahead/behind behavior.

## TDD sequence

1. Add failing agentctl tests for a custom remote name, distinct push and fetch
   URLs, push-ref-to-upstream fallback, a remote branch name that differs from
   the local branch, and conflicting multi-push URLs.
2. Add a failing lifecycle propagation test for the normalized runtime head.
3. Implement the smallest remote-resolution helper and protocol fields needed
   to make those tests pass.
4. Refactor repeated provider parsing only where the tests demonstrate shared
   behavior; keep remote URL grammar inside agentctl.

## Verification

- RED/GREEN runtime selection: `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'TestGetGitStatus_RuntimeHead' -count=1`
- RED/GREEN lifecycle propagation: `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestPublishGitStatus_PropagatesRuntimeHead' -count=1`
- Existing push-state regression: `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'TestGetGitStatus_RemoteAheadTracksUpstreamNotBaseBranch' -count=1`
- Focused packages: `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -count=1`

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/git.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/events_git_status_test.go`
- `docs/plans/fork-pr-task-linking/plan.md`
- `docs/plans/fork-pr-task-linking/task-01-stream-runtime-head-ref.md`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes this exact protocol and should not guess its final
field semantics.

## Inputs

- The identity boundary in
  `docs/decisions/2026-08-09-runtime-branch-remote-identity.md`.
- Existing upstream status logic in
  `apps/backend/internal/agentctl/server/process/workspace_git_status.go`.
- Existing provider URL parsers and credential redaction in
  `apps/backend/internal/agentctl/server/process/git_pr_providers.go`.

## Output contract

Report changed files, permanent RED and GREEN evidence, exact focused package
results, the raw-URL/credential safety assertion, and synchronized task/plan
statuses in the primary session.

## Results

RED/GREEN evidence: `go test -tags fts5 ./internal/agentctl/server/process
-run '^TestGetGitStatus_ReportsConfiguredPushHeadRemote$' -count=1` and `go test
-tags fts5 ./internal/agent/runtime/lifecycle -run
'^TestPublishGitStatus_PropagatesHeadRemote$' -count=1` passed after the initial
failing assertions. The focused package command `go test -tags fts5
./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -count=1`
passed. The emitted shape is normalized and credential-free; the test remote
used a credential-bearing push URL and asserted that the secret was absent.
