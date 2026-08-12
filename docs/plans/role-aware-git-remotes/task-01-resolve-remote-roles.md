---
id: "01-resolve-remote-roles"
title: "Resolve provider-neutral remote roles"
status: pending
wave: 1
depends_on: ["00-integrate-upstream"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 01: Resolve provider-neutral remote roles

## Acceptance

- One concrete agentctl process module resolves writable action head, tracking upstream, and an optional comparison ref from Git configuration plus a credential-free repository/ref identity.
- The identity retains normalized provider, host, complete repository path or provider ID, and literal ref. GitHub owner/repository splitting stays in its adapter; nested GitLab paths and Azure IDs are not truncated.
- Remote-name selection is executor-local. Callers receive typed resolved, unresolved, or ambiguous evidence and do not parse provider URLs or assign special meaning to `origin`.
- Push URL precedence, distinct push/tracking refs, first-push remote-ref absence, multiple matching remotes, and credential stripping are explicit invariants.
- The resolver exposes the seam consumed by Tasks 05, 06, and 07 without selecting or transporting backend comparison context, which belongs to Task 05.
- The provider-neutral identity types are owned here in `apps/backend/internal/common/gitremote` so agentctl, orchestrator, provider persistence, and comparison transport share one dependency-safe vocabulary.

## TDD sequence

1. Add failing temporary-repository tests for custom names, fetch/push URL divergence, nested paths, literal branch case, absent destination refs, unrelated `origin`, multiple normalized matches, and credential-bearing URLs.
2. Define the smallest shared identity and role result types required by those tests in `internal/common/gitremote`, including repository identity, ref identity, and role-generation evidence.
3. Move provisional `GitHeadRemote` parsing behind the resolver and remove duplicate parsing within owned files.
4. Re-run focused tests after the final refactor and inspect test diagnostics for credential leakage.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'TestResolveGitRemoteRoles' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/types/... -count=1)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-01-resolve-remote-roles.md`
- `apps/backend/internal/common/gitremote/identity.go` (new)
- `apps/backend/internal/common/gitremote/identity_test.go` (new)
- `apps/backend/internal/agentctl/server/process/workspace_git_remote.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_remote_test.go` (new)
- `apps/backend/internal/agentctl/types/streams/git.go`
- `apps/backend/internal/agentctl/types/types.go`

## Dependencies

Task 00 must complete the merge, and the primary must release this task after reconciling its fresh path/ownership audit.

## Parallelism

Sequential. Tasks 02 through 04 establish exact provider identity, then Task 05 transports comparison context before consumer work starts.

## Output contract

Update only this task file's `## Results`. Report the final resolver interface and invariants, files changed, RED/GREEN evidence, credential-safety evidence, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Pending.
