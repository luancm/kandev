---
status: building
created: 2026-07-19
owner: kandev
---

# Workspace Git Status

## Why

Users opening or focusing Changes and Review need a current workspace snapshot without a large generated or untracked tree monopolizing agentctl. Repeated requests for the same repository must not amplify expensive Git and filesystem work, and the initial session-hydration path must remain within its two-second live-status budget by falling back when necessary.

Decision: [ADR-2026-08-12-role-based-git-remotes](../../decisions/2026-08-12-role-based-git-remotes.md).

## What

- Cached reads return the latest workspace-tracker snapshot. When no cached snapshot exists, the tracker performs a live observation.
- Fresh reads observe the live worktree and do not themselves replace the polling cache.
- Overlapping live observations for the same repository share one underlying observation. Different repositories in a multi-repository task may still be observed in parallel.
- Every non-cancelled caller receives the same completed snapshot or error from a shared observation. A caller whose own context is cancelled returns promptly without cancelling or otherwise poisoning the result for other callers.
- Tracker shutdown or the bounded shared-observation deadline cancels the underlying work. Cancelled work does not publish or cache a partial snapshot.
- After Git output is parsed, changed-file and synthetic untracked-diff enrichment performs work proportional to the number of changed entries plus the bounded content processed.
- Existing diff limits remain in force: 10 MiB maximum source file size, 256 KiB maximum emitted diff per file, and a 2 MiB enrichment threshold per status snapshot. Because the threshold is checked before enriching each file, the final accepted file may preserve the existing overshoot of up to the 256 KiB per-file cap. Existing skip reasons remain unchanged.
- Large changed sets retain every path and its status metadata. Once the total diff budget is exhausted, files that are not enriched retain `budget_exceeded` as their diff skip reason.
- Multi-repository responses retain repository identity and partial-success behavior.
- Verification tooling preserves shared managed Go and lint caches for reuse while keeping invocation scratch and command output outside repository worktrees. The root-level `.verify-cache` and `.tmp` paths are ignored as safeguards against legacy or misconfigured verification runs.
- Remote-role observations follow [Role-Aware Git Remotes](git-remote-roles.md): action-head and tracking observations are independent atomic snapshots with `unknown`/`absent`/`present` state and nullable counts; comparison evidence is never synthesized from either snapshot.

### Comparison context lifecycle

The backend, not agentctl, selects provider comparison intent. It sends an additive comparison context containing the provider-neutral comparison repository identity, literal ref, and nullable stored base SHA qualified to that exact identity/ref. A complete context replacement is sent at launch and resume and whenever linked-change association, remote-contribution binding, or selected base changes. Removing or invalidating the selection sends an explicit clear so agentctl cannot reuse an old target or stored SHA; omission during rolling compatibility retains the prior context rather than clearing it.

When multiple changes are linked to one attachment, the backend selects the unique change whose exact source repository/ref matches the worktree's writable action head and branch context. If no linked row or more than one linked row matches exactly, comparison is unresolved and status may use only an identity-qualified stored fallback. It never chooses a canonical base by row order, change number, branch name alone, or mutable base fields.

### Base-commit staleness and refresh

The commits panel (`git log <base>..HEAD`), cumulative diff, and task/sidebar additions and deletions anchor to the accepted comparison context. When no linked change competes, the comparison target is a validated remote-contribution target, then the attached repository plus selected `base_branch`. Agentctl maps that normalized repository identity/ref to executor-local remotes by URL identity; remote names have no semantic priority.

- A stored base SHA is usable only as the `stored_base_commit` of the current comparison context. The repository identity and literal ref are part of its authority. A target refresh to another repository or ref replaces or clears the SHA; an unqualified SHA from a prior context cannot become a fallback for the new target.
- For a resolved comparison target, a stored base commit is stale when it is a strict ancestor of `merge-base(HEAD, <resolved comparison target>)`. When that merge-base advances past the qualified stored SHA, commit enumeration and cumulative diff use the fresh merge-base. A stored base that equals or is a descendant of the current merge-base is not stale and remains the anchor.
- A live non-default comparison ref is authoritative. If the selected target is a live non-default branch such as `develop`, Kandev uses that ref's own merge-base even when it is older than the default integration branch's merge-base. It never hides genuine `develop`-relative commits by re-anchoring to `main`/`master`.
- A default integration candidate may participate in stale correction only when it belongs to the same normalized comparison repository and either the selected target is that default ref or the selected non-default ref can no longer be resolved as live. A remote name such as `origin` is not evidence of repository identity or default-ref authority.
- When a non-default ref is no longer live and an identity-matching default integration candidate proves that the qualified stored base is stale, Kandev may use the fresh integration merge-base for this read. This does not retarget the persisted `base_branch` or transfer the stored SHA to the default ref.
- Base resolution prefers a remote-tracking ref whose configured URL matches the comparison repository over a bare local ref of the same name. A local ref that no longer tracks any live matching remote cannot anchor the range when an identity-matching remote ref is available.
- Staleness detection is a read-time correction. It does not rewrite the persisted `base_commit_sha`; persistence continues to follow existing capture and "Compare against" reset paths, while the backend qualifies that value when constructing comparison context.
- When no configured remote matches the comparison identity, matching remotes disagree, or no usable merge-base exists, status reports comparison resolution as unknown/ambiguous and may use the qualified stored base. If no qualified stored base is available, it uses the existing safe branch-tip fallback and leaves comparison-derived counts unknown. Kandev never substitutes a repository merely because its remote is named `origin`.

## API surface

The existing routes and request parameters remain stable. Their status results gain the additive structured remote-role evidence described in [Role-Aware Git Remotes](git-remote-roles.md), while compatibility fields remain projections during migration.

- `GET /api/v1/git/status?repo=<subpath>&fresh=<bool>` returns `GitStatusResult`.
- `GET /api/v1/git/status/multi?fresh=<bool>` returns `MultiRepoGitStatusResult` containing `PerRepoGitStatus` entries.
- The `fresh` query parameter continues to select a live observation rather than a cached tracker snapshot.
- Comparison context is trusted backend-to-agentctl input, not a caller-selectable query or HTTP body field.

## Failure modes

| Scenario | Observable behavior |
|---|---|
| Primary branch or porcelain observation fails | The live observation fails and the prior cached snapshot remains available. |
| Secondary diff enrichment fails | The established same-HEAD carry-forward behavior is preserved. |
| One caller cancels while a shared observation is running | That caller returns its context cancellation promptly; other callers remain eligible to receive the shared result. |
| The tracker stops or the shared deadline expires | Underlying work is cancelled and no partial result is published or cached. |
| One repository fails during a multi-repository request | Successful repository entries remain available and the failure is reported on its repository entry. |
| Multiple linked changes do not yield one exact source match | Comparison is unresolved; no linked base is chosen by order and only an identity-qualified stored fallback may be used. |
| Stored base commit is a strict ancestor of `merge-base(HEAD, <resolved comparison target>)` | Commit enumeration, cumulative diff, and sidebar additions/deletions use the fresh merge-base rather than the stale stored SHA. |
| Comparison target is a live non-default branch whose merge-base is older than the integration merge-base | The target's own merge-base is preserved; the range is not re-anchored to the integration line. |
| A non-default target disappeared and an identity-matching default integration ref proves the stored anchor stale | The read uses the fresh integration merge-base without changing persisted comparison selection. |
| Resolved base branch exists only as a stale local ref but a matching comparison remote ref is present | The matching remote ref anchors the range; the stale local ref does not. |
| Comparison context changed but carries no newly qualified stored SHA | The prior SHA is not reused; unresolved status uses the safe branch-tip fallback and leaves comparison counts unknown. |
| No remote matches the comparison identity or no usable merge-base exists | Status uses only an identity-qualified stored fallback, otherwise the safe branch tip, and does not inspect an unrelated `origin/*`. |

## Scenarios

- **GIVEN** a stale cached snapshot after a commit, **WHEN** a caller requests `fresh=true`, **THEN** the response reflects the live clean tree and a later cached read still returns the prior cached snapshot.
- **GIVEN** six simultaneous fresh requests for one repository, **WHEN** their observations overlap, **THEN** exactly one underlying status observation runs and all non-cancelled callers receive the same capture timestamp and result.
- **GIVEN** simultaneous fresh requests for two repositories, **WHEN** multi-repository status runs, **THEN** one observation per repository may run in parallel and each response remains identified with its repository.
- **GIVEN** one waiter cancels during a shared observation, **WHEN** other waiters remain, **THEN** the cancelled waiter returns promptly and the remaining waiters receive the completed result.
- **GIVEN** tracker shutdown or the shared-observation deadline while enrichment is running, **WHEN** cancellation reaches the observation, **THEN** filesystem iteration stops and no partial snapshot is cached.
- **GIVEN** approximately 15,000 untracked text files, **WHEN** fresh status is computed, **THEN** every path is present, emitted diff content obeys the existing limits, files not enriched after total-budget exhaustion have `budget_exceeded`, and post-porcelain enrichment remains linear in the number of entries.
- **GIVEN** one invalid repository in a multi-repository request, **WHEN** other repositories succeed, **THEN** the response retains the successful entries and reports the failure only on the invalid repository.
- **GIVEN** verification needs writable scratch space, **WHEN** it selects a location, **THEN** the location is outside every Git worktree and existing shared caches remain reusable; if a legacy run creates root-level `.verify-cache` or `.tmp`, Git status ignores it.
- **GIVEN** a session whose qualified stored base is a strict ancestor of `merge-base(HEAD, <resolved integration candidate>)` after its non-default parent ref disappeared, **WHEN** the commits panel is requested, **THEN** the count matches `git rev-list --first-parent --count $(git merge-base HEAD <resolved integration candidate>)..HEAD` and excludes commits already landed on the integration branch.
- **GIVEN** a stored base branch that no longer has its original remote ref but another custom-named remote matches the comparison repository and base branch, **WHEN** the commits panel resolves its base, **THEN** it anchors to that matching remote's merge-base rather than the stale local ref.
- **GIVEN** a session whose qualified stored base equals or is a descendant of the current merge-base against the same comparison target, **WHEN** the commits panel is requested, **THEN** the stored base is used unchanged and the count is identical to today's behavior.
- **GIVEN** a session comparing against a live non-default branch whose merge-base is a strict ancestor of the default integration merge-base, **WHEN** the commits panel resolves its base, **THEN** it keeps the non-default branch's merge-base and does not re-anchor to the integration line.
- **GIVEN** the comparison target changes from `release` to `main`, **WHEN** the backend refreshes comparison context without a new stored SHA, **THEN** agentctl clears the old release-qualified anchor rather than reusing it for `main`.
- **GIVEN** two linked changes exist but neither exact source matches the worktree's action head, **WHEN** status computes task summary counts, **THEN** comparison is unresolved and neither linked base wins by association order.
- **GIVEN** `origin` points to a contributor fork while a custom-named remote matches the linked PR's canonical base, **WHEN** status computes the task summary, **THEN** additions and deletions use the canonical base merge-base and exclude commits already present there.
- **GIVEN** a worktree with no remote matching its comparison identity and a HEAD sharing no history with any valid candidate branch, **WHEN** the commits panel resolves its base, **THEN** it uses only the identity-qualified stored fallback or safe branch tip without using an unrelated `origin` or returning an error.

## Out of scope

- Changing the existing Git-status route names or `fresh` request semantics.
- Raising or removing existing diff-content limits.
- Changing multi-repository fan-out behavior.
- Making fresh reads owners of the polling cache.
- Replacing Git subprocesses with a native Git implementation.
- Rewriting the persisted `base_commit_sha` as part of staleness detection. The read-time correction changes only which base the commits/diff compute against; persistence continues to follow existing capture and "Compare against" reset paths.
- Auto-retargeting the session's `base_branch` when a stacked parent merges. Detecting a stale base and choosing an identity-matching live integration ref for the read is in scope; changing the stored base branch is not.

## Implementation plan

See [Workspace Git Status Scalability plan](../../plans/workspace-git-status-scalability/plan.md). Remote-role comparison remediation is tracked by the [Role-aware Git remotes plan](../../plans/role-aware-git-remotes/plan.md).
