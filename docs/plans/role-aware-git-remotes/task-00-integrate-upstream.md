---
id: "00-integrate-upstream"
title: "Integrate upstream and re-audit the package"
status: pending
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 00: Integrate upstream and re-audit the package

## Acceptance

- Fetch and merge the then-current `upstream/main` without rewriting branch history, record the integrated SHA and merge commit, and prove the fetched SHA is an ancestor of `HEAD`.
- Resolve known Git-status overlaps additively so writable action-head evidence survives alongside tracking `RemoteHeadCommit`, `RemoteAhead`, and `RemoteBehind` through single/multi status, runtime decoding, lifecycle publication, and frontend hydration.
- Inspect clean same-file overlaps in lifecycle/status tests, GitHub models/store/PAT code, decision records, and related specs so neither side's schema or monotonic state semantics is silently discarded.
- After the merge is complete, perform a fresh path and ownership audit of `plan.md` and every sibling task file against the integrated tree. Record renamed/missing paths, new API seams, overlapping owners, and dependency changes in this task's Results and report them to the primary.
- Do not release Task 01 or later work until the primary has reconciled any audit drift in the plan package.

## TDD sequence

1. Add or preserve contract coverage requiring writable-head and tracking snapshots in the same fixture across backend and frontend hops; identify it as merge-protection coverage if it already passes.
2. Run `cd apps && pnpm install --frozen-lockfile` before any frontend check in this worktree.
3. Fetch `upstream/main`, record its SHA, merge non-destructively, and resolve conflicts while retaining both evidence families.
4. Run focused checks and the ancestry assertion.
5. Re-enumerate every path and ownership set in the plan package against the post-merge tree, then report drift to the primary before downstream release.

## Verification

```bash
git fetch upstream main
git merge-base --is-ancestor upstream/main HEAD
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/github ./internal/orchestrator ./internal/task/statussummary -count=1)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts hooks/domains/session/use-session-git-summary.test.ts)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-00-integrate-upstream.md`
- `apps/backend/internal/agentctl/types/streams/git.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events_git_status_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/store.go`
- `apps/web/lib/types/git-events.ts`
- `apps/web/lib/ws/handlers/git-status.ts`
- `apps/web/lib/ws/handlers/git-status.test.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-multi-repo.test.ts`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `docs/decisions/INDEX.md`
- `docs/specs/INDEX.md`
- `docs/specs/ui/ci-pr-automation.md`

The merge may legitimately bring additional upstream files into the commit. This list identifies known conflict and semantic-overlap surfaces, not permission to omit the complete post-merge audit.

## Dependencies

None. This is the hard prerequisite and path-audit gate for the package.

The latest planning observation is `upstream/main` at `e014f8072`. The intervening upstream change only reverts an available-to-install UI experiment and removes its associated docs/E2E files; it does not overlap the remote-role implementation paths. Fetch again at execution and do not treat this observation as a pin.

## Parallelism

Sequential. No implementation task runs concurrently with the merge or its post-merge audit.

## Output contract

Update only this task file's `## Results`. Report the upstream SHA, merge commit, conflicts and clean overlaps reviewed, focused command results, complete post-merge path/ownership drift, and downstream release recommendation to the primary. Do not edit `plan.md` or another task file, and do not push.

## Results

Pending.
