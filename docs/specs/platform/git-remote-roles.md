---
status: draft
created: 2026-08-12
owner: kandev
---

# Role-Aware Git Remotes

## Why

Users legitimately attach one repository, push a branch to another, track a third ref, and compare the work against a canonical change-request base. Kandev currently collapses those roles into `origin` or the current branch's upstream in several paths, which can inflate sidebar diff counts, link the wrong pull request, or route a remote mutation somewhere other than the repository described by the UI.

Kandev needs one provider-neutral model that works for ordinary repositories, remote contributions, GitHub forks, nested GitLab projects, Azure Repos, custom remote names, and triangular Git configuration.

Decision: [ADR-2026-08-12-role-based-git-remotes](../../decisions/2026-08-12-role-based-git-remotes.md).

## What

- Kandev treats the attached repository, writable action head, comparison target, and tracking upstream as independent roles. Roles may alias but are never inferred from the names `origin` or `upstream`.
- The writable action head is Git's exact resolved push destination. Kandev does not substitute the tracking upstream when no safe action-head identity can be resolved.
- The tracking upstream is the local branch's explicit `@{upstream}`. Pull is unavailable when it is not configured; the comparison target is never a Pull fallback.
- Runtime remote identities are credential-free and provider-neutral: provider, normalized host, full repository path or provider ID, and literal branch/ref. Provider adapters translate complete source and base identities into GitHub, GitLab, or Azure-specific request fields and fail closed when either side cannot be translated exactly.
- The backend supplies agentctl an additive comparison context containing the selected provider-neutral repository/ref and a nullable stored base SHA qualified to that same identity. It refreshes the complete context when durable selection changes and explicitly clears it when the selection is removed.
- Git status reports writable-action evidence separately from one coherent tracking-upstream snapshot and comparison-target evidence. Remote-ref observations distinguish `unknown`, known `absent`, and `present`; nullable counts never turn missing evidence into zero.
- The task sidebar's additions/deletions summarize the cumulative diff from the comparison target's merge base, including when the matching local remote has a custom name.
- With one or more linked PRs/MRs, Kandev selects the canonical comparison base only from the unique linked change whose exact source repository/ref matches the worktree and writable action head. No or multiple exact matches make the comparison unresolved; Kandev never chooses a linked row by order, number, or branch name alone.
- When no linked change competes, a validated remote-contribution target takes precedence, followed by the attached repository and selected base branch. Kandev does not guess a provider fork's parent.
- Push and Force Push use the writable action head. Pull uses only the explicit tracking upstream. Rebase and Merge use the comparison target. Create PR/MR receives both the exact writable source and exact comparison target.
- Every remote mutation is authorized against the expected remote-role generation and exact identity observed by the caller, then re-resolved under the serialized Git-operation lock. Stale or mismatched evidence fails before Git runs.
- Provider association remains anchored to the attached repository and workspace credentials. Runtime Git configuration may narrow an exact source ref but cannot expand provider access or credential scope.
- Exact GitHub discovery compares host/owner/repository case-insensitively and branch names case-sensitively, paginates until uniqueness is established, deduplicates canonical PR identity, and fails closed on ambiguity or incomplete traversal.
- Searching watches preserve their exact writable head across restart and terminal reset. Changing the local branch clears stale head fields; resolving a PR never replaces the durable attached-repository anchor used by a later search.
- Provider/local contribution drift compares the provider's current source identity and head with the writable action observation. Writable-action counts drive Push, tracking counts drive Pull, and comparison counts drive review scope; one role never borrows another role's evidence.
- Desktop and mobile consume the same remote-role state and expose the same safe Push/Pull availability.
- Kandev does not introduce an `isFork` field or branch behavior on provider fork topology.

## Data model

`RemoteRepositoryIdentity` is a credential-free value with:

- `provider`: normalized provider kind when recognized; empty for a generic Git host.
- `host`: normalized host identity required by that provider.
- `repository_path`: full provider repository path, preserving nested namespaces.
- `provider_repository_id`: optional stable provider identifier.

`RemoteRefIdentity` combines a remote repository identity with one literal branch/ref. Repository comparison follows provider case rules; ref comparison is always case-sensitive.

`ComparisonContext` is additive backend-to-agentctl input with:

- `context_generation`: opaque backend generation for the complete comparison selection.
- `target`: nullable `RemoteRefIdentity`.
- `stored_base_commit`: nullable commit SHA valid only for the same `target` identity/ref.
- `update`: `replace` or `clear`; a replacement is complete and a clear removes both target and stored SHA.

The backend sends `ComparisonContext` at session launch/resume and refreshes it after linked-change association/removal, remote-contribution refresh, or selected-base change. Omission of the additive field during compatibility rollout retains the last context; it never means clear. Agentctl returns an opaque `remote_roles_generation` that covers the accepted comparison context plus current branch and Git role configuration.

`RemoteRefObservation` has:

- `identity`: nullable `RemoteRefIdentity`; required for `absent` and `present`.
- `observation_state`: `unknown`, `absent`, or `present`.
- `remote_head_commit`: nullable SHA; required only for `present` and forbidden for `absent`.
- `ahead` and `behind`: nullable non-negative counts computed against the same identity/head observation.

`unknown` means identity or ref state could not be established authoritatively, including invalid configuration, ambiguity, transport failure, or an observation that has not run. `absent` means the exact resolved remote ref was authoritatively checked and does not exist. `present` means the exact ref and observed head SHA are known. A null count means unknown; `0` is published only when zero was actually computed. `absent` does not manufacture zero counts, and first-Push eligibility requires separate local contribution evidence.

The runtime status contract exposes:

- `remote_roles_generation`: the opaque generation required by remote mutations.
- `action_head`: the writable action head's atomic `RemoteRefObservation`.
- `tracking_upstream`: the explicit tracking upstream's atomic `RemoteRefObservation`.
- `comparison`: accepted context generation, target identity, resolution state, resolved executor-local ref evidence, identity-qualified base commit, and nullable base-relative counts.
- `remote_branch`, `remote_head_commit`, `remote_ahead`, and `remote_behind`: compatibility projection of the single `tracking_upstream` observation during migration.
- `comparison_target`, `base_commit`, `ahead`, and `behind`: compatibility projection of `comparison` during migration.

The structured observations are authoritative. A complete status replaces each included observation atomically. A partial status update that omits one structured observation retains its prior value; an explicit `unknown` or clear replaces it and invalidates prior safety evidence. Compatibility scalar fields are derived from their structured source and are never merged independently or used to fill null structured counts.

`ProviderChangeIdentity` contains exact `source` and `base` `RemoteRefIdentity` values plus provider change kind and identifier. GitHub, GitLab, and Azure adapters populate the same shared shape at their provider seam. The attached repository remains a separate authorization anchor and cannot stand in for a missing source or base.

GitHub `github_pr_watches.repository_id` remains the attached repository anchor. Searching rows retain the exact head identity separately. Mutable canonical base owner/repository fields address numbered PR status only and are never reused as the authorization anchor for a later search.

## Comparison selection

For one repository/worktree, comparison selection is ordered and fail-closed:

1. Read every linked change associated with the attachment.
2. If linked changes exist, filter by exact provider-neutral source repository/ref matching the worktree's current writable action head and branch context. Exactly one match supplies the canonical base. Zero matches, multiple matches, or incomplete identities produce `unresolved`; selection does not continue to a weaker candidate.
3. If no linked change exists, use the validated remote-contribution target when present.
4. Otherwise use the attached repository plus the selected base ref.

Agentctl resolves executor-local remote names from normalized configured URLs. If multiple matching remote refs resolve to different commits, the comparison is ambiguous. The resolver does not choose by name or enumeration order. A stored base commit is a fallback only when `ComparisonContext` qualifies it to the exact selected repository/ref; changing or clearing the target clears that authority.

## State and operation rules

| Consumer | Role used | Rule |
| --- | --- | --- |
| Sidebar and cumulative diff | Comparison target | Diff from its merge base; use only an identity-qualified stored base fallback |
| Push and Force Push | Writable action head | Mutate exactly that remote/ref and use only its observation for safety |
| Pull | Tracking upstream | Require and mutate the exact explicit upstream; no other-role fallback |
| Rebase and Merge | Comparison target | Fetch and operate on the resolved target ref |
| Create PR/MR | Writable action head plus comparison target | Publish the exact source, then create against the explicit provider base |
| PR/MR discovery | Attached repository plus writable action head | Authorize through the attachment and match the exact source ref |
| Provider/local drift | Provider source plus writable action head, with tracking upstream for Pull | Compare provider source history to action-head state; offer Pull only when tracking identifies that same source and a fast-forward is proven |

## Mutation authorization

Every Push, Force Push, Pull, Rebase, Merge, and Create PR/MR request includes `expected_remote_roles_generation` and the exact `expected_target` `RemoteRefIdentity` shown to the caller. Push/Force Push also include the expected action-head `observation_state` and remote-head SHA when present; Pull includes the expected tracking observation; Rebase/Merge include the accepted comparison context generation; Create PR/MR includes exact expected source and base identities.

Agentctl acquires the existing serialized Git-operation lock, refreshes the role resolver, and compares the current generation, identity, ref, state, and applicable head SHA with the request before invoking Git. Any mismatch returns a stale-role error that requires a fresh status; it does not silently retarget. Git's non-fast-forward protection or force-with-lease remains required for movement after the refresh.

The backend validates the same source/base identities against the attached repository, linked change or validated contribution binding, workspace provider scope, and credential policy. GitHub, GitLab, and Azure creation adapters receive explicit identities and fail closed on incomplete, cross-host, mismatched, generic-host, or unsupported provider data.

## Failure modes

| Condition | Observable behavior |
| --- | --- |
| Comparison target has no matching local remote | Use only an identity-qualified stored base fallback, mark target evidence unresolved, and do not substitute `origin` |
| Multiple linked changes do not yield one exact source match | Mark comparison unresolved and use only the qualified fallback; never choose an arbitrary linked base |
| Multiple matching comparison refs disagree | Treat comparison evidence as ambiguous; do not publish misleading fresh counts |
| Writable action identity cannot be resolved because configuration is invalid or conflicting | Disable Push, Force Push, cross-repository source discovery, and creation; show the existing operation error path |
| Writable action identity is known but its observation is `unknown` | Permit identity-scoped read-only discovery, but disable Push, Force Push, and creation until authoritative action evidence is refreshed |
| Writable ref is known `absent` | Permit a first Push only when local contribution evidence exists and expected identity/generation still match |
| Tracking upstream is missing or its observation is `unknown` | Pull is unavailable until an explicit usable tracking snapshot is refreshed; Push continues to use only action-head evidence |
| Tracking upstream differs from writable action head | Pull still targets the exact tracking ref, but that snapshot cannot prove a provider-source fast-forward or authorize Push |
| Provider source host differs from the attached provider host | Reject association or creation; provider changes cannot cross hosts |
| Provider adapter lacks complete source or base identity | Omit/disable the provider operation; never combine a branch from one identity with another repository |
| Expected generation, identity, state, or head changes before mutation | Reject as stale before Git runs and require a fresh observation |
| Exact PR traversal fails or cannot establish uniqueness | Keep the watch searching and retry later; never choose a partial/result-order candidate |
| Searching watch has a nonempty but unresolved `repository_id` | Fail closed instead of treating mutable canonical base fields as the attachment |

## Persistence guarantees

Attached repository identity, linked change base/source identity, comparison selection, its identity-qualified stored base, and searching-watch head identity survive backend restart. Executor-local remote names, remote-role generations, and observation snapshots do not become durable semantic identity; they are recomputed in the executor. Existing legacy searching rows with no head identity use the attached repository as the exact expected head for read-only same-repository discovery. Legacy unqualified base SHAs are accepted only when the backend can prove the unchanged attached-repository/selected-ref context; otherwise they are cleared from the comparison context.

## Scenarios

- **GIVEN** `origin` points to a contributor fork and a custom-named remote points to the linked PR's canonical base, **WHEN** status computes the sidebar diff, **THEN** additions/deletions use the canonical base merge-base and exclude commits already present there.
- **GIVEN** two linked changes share a base but only one source repository/ref matches the worktree's writable action head, **WHEN** comparison context is refreshed, **THEN** that exact linked change supplies the base.
- **GIVEN** two linked changes both match incompletely or neither matches the current action head, **WHEN** comparison context is refreshed, **THEN** comparison becomes unresolved and Kandev does not choose by association order.
- **GIVEN** a custom remote name resolves to the attached repository and selected base branch, **WHEN** no PR/MR is linked, **THEN** comparison uses that remote without requiring it to be named `origin`.
- **GIVEN** no remote matches the comparison identity, **WHEN** status runs, **THEN** it retains only a stored anchor qualified to that exact target and never compares against an unrelated `origin/main`.
- **GIVEN** the current branch pushes to a fork but explicitly tracks the canonical repository, **WHEN** status and controls update, **THEN** Push uses action-head divergence and Pull uses tracking-upstream divergence.
- **GIVEN** no tracking upstream is configured, **WHEN** the user opens remote actions, **THEN** Pull is unavailable even if a comparison target exists.
- **GIVEN** Push and tracking upstream use different remote branch names, **WHEN** the user invokes Push, **THEN** Kandev publishes to the configured writable ref and does not derive the destination from the local or tracking branch name.
- **GIVEN** the provider's current source branch advanced and still contains local HEAD, **WHEN** the action head targets that source but tracking points elsewhere, **THEN** Kandev reports provider-ahead history but does not offer Pull through the unrelated tracking ref.
- **GIVEN** a linked change has complete provider-neutral source and base identities, **WHEN** the user invokes Create PR/MR again, **THEN** Kandev reuses the association or invokes the matching GitHub, GitLab, or Azure adapter without inferring either side from `origin`.
- **GIVEN** the UI observed one action head and Git configuration changes before Push, **WHEN** Push reaches agentctl with the old generation and identity, **THEN** agentctl rejects it before invoking Git.
- **GIVEN** three sibling forks have open PRs with the same branch name and the exact writable head is returned after the first page, **WHEN** discovery completes, **THEN** Kandev links only that exact PR.
- **GIVEN** a second eligible exact-head PR appears on a later page, **WHEN** discovery completes, **THEN** the watch remains unlinked as ambiguous.
- **GIVEN** GitHub returns repository identity with different letter casing but the same branch casing, **WHEN** exact discovery runs, **THEN** identity matches; a branch-case difference does not match.
- **GIVEN** two watches share an attached repository and local branch but have different writable heads, **WHEN** batched discovery runs, **THEN** each watch consumes only its exact-head result.
- **GIVEN** immediate detection exhausts retries before a PR appears, **WHEN** background polling resumes after restart, **THEN** the persisted searching watch retains and uses the observed writable head.
- **GIVEN** a watch changes to a different local branch, **WHEN** the branch mutation commits, **THEN** stale head fields are cleared atomically; terminal reset of the same branch retains its exact head.
- **GIVEN** a resolved watch stores a canonical base different from its attachment, **WHEN** it later returns to searching, **THEN** discovery resolves authorization through `repository_id`, not the old canonical base.
- **GIVEN** desktop and mobile show the same contribution, **WHEN** remote-role state changes, **THEN** both surfaces expose identical counts and remote-action safety decisions.

## Out of scope

- Inferring a provider fork parent when no uniquely matched linked change, validated contribution binding, or explicit comparison selection supplies it.
- Adding a fork badge, `isFork`, parent-repository field, or remote-name-specific product behavior.
- Changing provider permissions, minting broader credentials, or exposing credential-bearing URLs.
- Automatically fetching, rebasing, merging, resetting, or otherwise reconciling diverged histories.
- Redesigning the branch picker or adding a remote-role configuration UI in this repair.

## Implementation plan

See [Role-aware Git remotes plan](../../plans/role-aware-git-remotes/plan.md).
