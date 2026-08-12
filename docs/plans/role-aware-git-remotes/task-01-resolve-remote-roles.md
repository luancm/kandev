---
id: "01-resolve-remote-roles"
title: "Resolve provider-neutral remote roles"
status: complete
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

Implemented the provider-neutral remote-role resolver and shared identity vocabulary. `WorkspaceTracker.ResolveGitRemoteRoles` resolves Git's configured writable push destination first, the explicit tracking upstream separately, and an optional comparison identity by scanning every configured remote without assigning semantics to `origin`, `upstream`, or any other name; `ResolveGitRemoteRolesForBranch` provides the same seam for an already-observed status branch, and `ObserveGitRemoteRef` performs an explicit exact-ref probe that distinguishes unknown, absent, and present evidence.

Added `internal/common/gitremote` identity, repository comparison, observation-state, role, and generation types. Repository identity preserves provider, normalized host (including non-default self-hosted HTTP(S) ports), complete repository path or provider ID; provider paths use provider case rules, generic paths remain literal, and refs remain case-sensitive. Agentctl now honors pushurl precedence, retains distinct push/tracking branch refs including configured refspec renames, rejects unsupported/local-only or conflicting URL evidence, strips credentials by construction, preserves nested GitLab/Azure identity paths, and projects the shared identity into the additive credential-free `GitHeadRemote` fields without changing `RemoteBranch` semantics.

Files changed: `apps/backend/internal/common/gitremote/identity.go`, `apps/backend/internal/common/gitremote/identity_test.go`, `apps/backend/internal/agentctl/server/process/workspace_git_remote.go`, `apps/backend/internal/agentctl/server/process/workspace_git_remote_test.go`, and `apps/backend/internal/agentctl/types/streams/git.go`.

RED evidence: the new identity and resolver tests initially failed to compile because the shared types and resolver were absent; the comparison test then failed unresolved until the test supplied the configured custom GitLab host, confirming provider configuration was part of resolution rather than an implicit remote-name rule.

GREEN evidence: `(cd apps/backend && go test ./internal/common/gitremote ./internal/agentctl/server/process -run 'Test(Remote|ResolveGitRemoteRoles|ObserveGitRemoteRef|ParseRemoteRepositoryIdentity)' -count=1)` passed; `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -count=1)` passed; `(cd apps/backend && go test ./internal/common/gitremote -count=1)` passed; `git diff --check` passed. Tests cover custom remote names, push/tracking divergence, renamed destination refs, nested and self-hosted GitLab paths, Azure HTTP/SSH paths, case-sensitive refs, multiple matching remotes, conflicting push URLs, first-push absent destination refs, local-only rejection, and credential-bearing URLs. No raw URL or credential is stored in the normalized identity or emitted by the projection.
